package usage

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Every record Append returns for is in the log, however many calls wrote at
// once: the writer batches them but loses none and keeps each one whole.
func TestAppendConcurrentWritesEveryRecord(t *testing.T) {
	pageHome(t)
	resetAppendsFailed()
	t.Cleanup(resetAppendsFailed)

	const n = 128
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			Append(Record{Agent: "writer", Provider: "relay", Model: "m", Input: i, Status: 200})
		}(i)
	}
	wg.Wait()
	Flush()

	recs := Load(time.Time{})
	if len(recs) != n {
		t.Fatalf("%d records written, %d read back", n, len(recs))
	}
	seen := make([]bool, n)
	for _, r := range recs {
		if r.Input < 0 || r.Input >= n || seen[r.Input] {
			t.Fatalf("record %d read twice or out of range", r.Input)
		}
		seen[r.Input] = true
	}
	if drops := AppendsFailed(); drops != 0 {
		t.Fatalf("%d records counted as dropped", drops)
	}
}

// A full queue waits for the writer instead of dropping the record: while the
// writer is held, no Append returns and nothing is counted as dropped; once
// it drains, every record is in the log, in the order the writer wrote them.
func TestAppendQueueFullWaitsForTheWriter(t *testing.T) {
	pageHome(t)
	resetAppendsFailed()
	var entered, release atomic.Bool
	t.Cleanup(func() {
		release.Store(true)
		openFail = nil
		resetAppendsFailed()
	})
	openFail = func() error {
		if entered.CompareAndSwap(false, true) {
			for !release.Load() {
				time.Sleep(time.Millisecond)
			}
		}
		return nil // the write goes on: held, not dropped
	}

	// One Append starts the writer and is held inside its open.
	go Append(Record{Agent: "queue", Provider: "relay", Model: "m", Input: -1, Status: 200})
	for !entered.Load() {
		time.Sleep(time.Millisecond)
	}

	const n = appendQueueLen + 8
	var started, acked atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			started.Add(1)
			Append(Record{Agent: "queue", Provider: "relay", Model: "m", Input: i, Status: 200})
			acked.Add(1)
		}(i)
	}
	for started.Load() < int64(n) {
		time.Sleep(time.Millisecond)
	}
	// The queue is more than full; every caller waits for the writer, and
	// none has given up on its record.
	time.Sleep(200 * time.Millisecond)
	if acked.Load() != 0 {
		t.Fatalf("%d Appends returned while the writer was held", acked.Load())
	}
	release.Store(true)
	wg.Wait()
	if acked.Load() != int64(n) {
		t.Fatalf("%d of %d Appends returned", acked.Load(), n)
	}
	if drops := AppendsFailed(); drops != 0 {
		t.Fatalf("%d records counted as dropped", drops)
	}
	if got := len(Load(time.Time{})); got != n+1 {
		t.Fatalf("%d records read back, want %d", got, n+1)
	}
}

// Flush waits until what Append returned for is on disk, for a test or a
// process about to stop that reads the file itself.
func TestFlushLeavesTheLogReadable(t *testing.T) {
	pageHome(t)
	Append(Record{Agent: "flush", Provider: "relay", Model: "m", Input: 4, Status: 200})
	Flush()
	b, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(b), "\n"); got != 1 || !strings.Contains(string(b), `"in":4`) {
		t.Fatalf("after Flush the log holds %d line(s): %s", got, b)
	}
}

// The 200 share F-11 lacked as a denominator: answered calls counted, those
// with no token count at all counted apart, reported calls and probes out.
func TestAnswered200CountsMissingUsage(t *testing.T) {
	pageHome(t)
	resetAnswered200()
	t.Cleanup(resetAnswered200)
	for _, r := range []Record{
		{Agent: "a", Provider: "p", Model: "m", Status: 200, Input: 5},
		{Agent: "a", Provider: "p", Model: "m", Status: 200}, // the vendor said no usage
		{Agent: "a", Provider: "p", Model: "m", Status: 429},
		{Agent: "a", Provider: "p", Model: "m", Status: 200, Source: SourceReported},
		{Agent: "a", Provider: "p", Model: "m", Status: 200, Source: SourceProbe},
		// a local refusal that never reached a vendor is no answered call
		{Agent: "a", Provider: "magpie", Model: "m", Status: 200, Rejected: true, Error: "a title"},
	} {
		Append(r)
	}
	if total, noUsage := Answered200(); total != 2 || noUsage != 1 {
		t.Fatalf("answered 200s %d, of them without usage %d; want 2 and 1", total, noUsage)
	}
}
