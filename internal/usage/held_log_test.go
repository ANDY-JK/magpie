package usage

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Append keeps the log open between batches (#3725), except on Windows,
// where another file cannot be renamed over a path a handle holds open; it
// still writes to the file Path names after another program replaces or
// removes it.
func TestAppendHeldLogFollowsThePath(t *testing.T) {
	pageHome(t)
	holdClock(t, time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local))
	t.Cleanup(closeHeldLog)

	Append(Record{Agent: "codex", Provider: "relay", Model: "m", Input: 2, Status: 200})
	first := heldLogForTest(t)
	Append(Record{Agent: "codex", Provider: "relay", Model: "m", Input: 3, Status: 200})
	second := heldLogForTest(t)
	if runtime.GOOS == "windows" {
		if first != nil || second != nil {
			t.Fatalf("Windows kept the log open between batches: %p, %p", first, second)
		}
	} else if first == nil || second != first {
		t.Fatalf("the log was opened again for the second call: %p then %p", first, second)
	}

	f := filepath.Join(filepath.Dir(Path()), "replacement.jsonl")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(f, Path()); err != nil {
		t.Fatal(err)
	}
	Append(Record{Agent: "codex", Provider: "relay", Model: "m", Input: 7, Status: 200})
	if s := Summarize(Today); s.Input != 7 || s.Calls != 1 {
		t.Fatalf("after a replacement: %+v", s.Totals)
	}

	if err := os.Remove(Path()); err != nil {
		t.Fatal(err)
	}
	Append(Record{Agent: "codex", Provider: "relay", Model: "m", Input: 11, Status: 200})
	if s := Summarize(Today); s.Input != 11 || s.Calls != 1 {
		t.Fatalf("after a removal: %+v", s.Totals)
	}

	pageHome(t) // another HOME: the log is another file
	Append(Record{Agent: "codex", Provider: "relay", Model: "m", Input: 13, Status: 200})
	if s := Summarize(Today); s.Input != 13 || s.Calls != 1 {
		t.Fatalf("in another HOME: %+v", s.Totals)
	}
}

// heldLogForTest is the file the writer keeps open, once everything Append
// has returned for is written.
func heldLogForTest(t *testing.T) *os.File {
	t.Helper()
	Flush()
	openWriteMu.Lock()
	defer openWriteMu.Unlock()
	return heldLog.f
}
