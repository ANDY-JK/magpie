package usage

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A log scan waiting on the writer's lock is a request held up by a busy
// disk: LastSeen reads the log's tail on the agent-applied path, and a write
// an open holds open must not queue it. Inject a delay inside the write
// (through openLog's existing seam, reached after Append takes the write
// lock) and time a LastSeen whose in-process note is empty, so it must read
// the tail. Every record still lands: the write is held, not dropped.
func TestLastSeenTailScanNotBlockedBySlowAppend(t *testing.T) {
	pageHome(t)
	resetAppendsFailed()
	t.Cleanup(resetAppendsFailed)

	const agent = "last-seen-tail-scan-not-blocked"

	// A real record in the log, so the tail scan has a line to parse and a
	// last-seen time to find without an in-process note.
	at := time.Now().Add(-time.Minute).UTC().Truncate(time.Millisecond)
	Append(Record{Time: at, Agent: agent, Provider: "relay", Model: "m", Input: 1, Status: 200})

	var entered, release atomic.Bool
	// Always let the delayed write finish, even when the assertion below
	// fails on the old lock and Fatal runs this cleanup.
	t.Cleanup(func() { openFail = nil; release.Store(true) })
	openFail = func() error {
		if entered.CompareAndSwap(false, true) {
			for !release.Load() {
				time.Sleep(time.Millisecond)
			}
		}
		return nil // the open still goes on: this write is slow, not dropped
	}

	// One Append holds the write lock with the injected delay. A second
	// Append behind it proves calls queue on the write (accounting stays
	// ordered), while LastSeen must answer without waiting for that lock.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		Append(Record{Agent: agent, Provider: "relay", Model: "m", Input: 2, Status: 204})
	}()
	for !entered.Load() {
		time.Sleep(time.Millisecond)
	}
	go func() {
		defer wg.Done()
		Append(Record{Agent: agent, Provider: "relay", Model: "m", Input: 3, Status: 200})
	}()

	done := make(chan time.Time, 1)
	go func() { done <- LastSeen(agent) }()
	select {
	case got := <-done:
		if got.Before(at) {
			t.Fatalf("tail scan returned %v, want the log record at %v", got, at)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("LastSeen's tail scan blocked behind a slow Append")
	}

	release.Store(true)
	wg.Wait()
	openFail = nil
	if n := AppendsFailed(); n != 0 {
		t.Fatalf("%d records dropped while the write was held", n)
	}
	var inputs []int
	for _, r := range Load(time.Time{}) {
		if r.Agent == agent {
			inputs = append(inputs, r.Input)
		}
	}
	if len(inputs) != 3 || inputs[0] != 1 || inputs[1] != 2 || inputs[2] != 3 {
		t.Fatalf("accounting after the slow write: %v, want [1 2 3]", inputs)
	}
}
