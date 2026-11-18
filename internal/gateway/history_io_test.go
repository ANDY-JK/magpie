package gateway

// Tests for the write path's locks (#F-14): the file IO and the gzip moved
// off the reader's history.mu onto a writer-only lock, so a save or a prune
// never holds up a look at the history. The old saveRoute took history.mu
// around MkdirAll/open/write/close and pruneHistory, and a read took it too.

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A save takes only the writer's lock: while history.mu (the reader's) is
// held by another goroutine, saveRoute still finishes and the line lands.
// The old path took history.mu around its IO, so this would hang.
func TestSaveRouteTakesNoReadLock(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	now := historyNoon(t)
	historyWrite.mu.Lock()
	historyWrite.pruned = now // no prune here
	historyWrite.mu.Unlock()
	history.mu.Lock()
	done := make(chan struct{})
	go func() {
		saveRoute(Route{ID: 1, Time: now, Model: "m", Done: true})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		history.mu.Unlock()
		t.Fatal("saveRoute waited for the reader's history.mu")
	}
	history.mu.Unlock()
	if _, err := os.Stat(filepath.Join(HistoryDir(), now.Format(dayForm)+".jsonl")); err != nil {
		t.Fatalf("the route was not written: %v", err)
	}
}

// A read needs only history.mu: while a writer holds historyWrite (a slow
// disk, or a day's gzip), History and DayTallies still return.
func TestHistoryReadNotBlockedByWrite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	now := historyNoon(t)
	saveRoute(Route{ID: 1, Time: now, Model: "m", Done: true}) // a day to read
	historyWrite.mu.Lock()
	done := make(chan struct{})
	go func() {
		History(now.Format(dayForm))
		DayTallies()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		historyWrite.mu.Unlock()
		t.Fatal("a read waited for the writer's historyWrite.mu")
	}
	historyWrite.mu.Unlock()
}

// pruneHistory runs off history.mu too: gzipping a day never takes the
// reader's lock, so a look at the history is not held up by it.
func TestPruneHistoryTakesNoReadLock(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	now := historyNoon(t)
	dir := HistoryDir()
	os.MkdirAll(dir, 0o700)
	day := now.AddDate(0, 0, -1).Format(dayForm)
	line := `{"id":1,"time":"2026-09-29T10:00:00Z","done":true}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, day+".jsonl"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	history.mu.Lock()
	done := make(chan struct{})
	go func() {
		pruneHistory(dir, now)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		history.mu.Unlock()
		t.Fatal("pruneHistory waited for history.mu")
	}
	history.mu.Unlock()
	if _, err := os.Stat(filepath.Join(dir, day+".jsonl.gz")); err != nil {
		t.Fatalf("the day was not gzipped: %v", err)
	}
}

// gzipFile streams: a written day's routes come back whole from its .gz, the
// .gz is a complete gzip stream, and the .jsonl is gone once its routes are in.
func TestGzipFileRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	now := historyNoon(t)
	dir := HistoryDir()
	os.MkdirAll(dir, 0o700)
	day := now.AddDate(0, 0, -1).Format(dayForm)
	path := filepath.Join(dir, day+".jsonl")
	var buf bytes.Buffer
	want := []int64{1, 2, 3}
	for _, id := range want {
		b, _ := json.Marshal(Route{ID: id, Time: now, Model: "m", Done: true})
		buf.Write(b)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	gzipFile(path)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the .jsonl is still there: %v", err)
	}
	b, err := os.ReadFile(path + ".gz")
	if err != nil {
		t.Fatal(err)
	}
	z, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, z); err != nil {
		t.Fatalf("the .gz isn't whole: %v", err)
	}
	_, rs, _ := History(day)
	var got []int64
	for _, r := range rs {
		got = append(got, r.ID)
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("routes %v, want %v", got, want)
	}
}
