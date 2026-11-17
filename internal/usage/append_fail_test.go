package usage

import (
	"errors"
	"testing"
)

// An Append that can't open the log is not silent: the dropped record is
// counted (AppendsFailed), the calls go on, and a log that heals again
// still shows the total dropped. openFail is the test's seam for the open.
func TestAppendFailureCounted(t *testing.T) {
	pageHome(t)
	resetAppendsFailed()
	t.Cleanup(resetAppendsFailed)
	refused := errors.New("the folder takes no file")

	// the log opens: one record, nothing dropped
	Append(Record{Agent: "codex", Provider: "relay", Model: "m", Input: 2, Status: 200})
	if n := AppendsFailed(); n != 0 {
		t.Fatalf("nothing dropped, %d counted", n)
	}

	// the log can't be opened: two records dropped, both counted, and the
	// calls still go on
	openFail = func() error { return refused }
	Append(Record{Agent: "codex", Provider: "relay", Model: "m", Input: 3, Status: 200})
	Append(Record{Agent: "codex", Provider: "relay", Model: "m", Input: 5, Status: 200})
	if n := AppendsFailed(); n != 2 {
		t.Fatalf("two records dropped, %d counted", n)
	}
	// and what could be read is still there: the ledger keeps what it
	// could open
	if s := Summarize(Today); s.Calls != 1 || s.Input != 2 {
		t.Fatalf("the record that wrote is not in the ledger: %+v", s.Totals)
	}

	// a healed log writes again, and the count of what was dropped stands
	openFail = nil
	Append(Record{Agent: "codex", Provider: "relay", Model: "m", Input: 7, Status: 200})
	if n := AppendsFailed(); n != 2 {
		t.Fatalf("the drops since start, %d", n)
	}
	if s := Summarize(Today); s.Calls != 2 || s.Input != 9 {
		t.Fatalf("the record that healed is not in the ledger: %+v", s.Totals)
	}
}
