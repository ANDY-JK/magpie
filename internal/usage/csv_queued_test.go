package usage

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

// queued_ms is the ledger's last column, and only there: appended after
// credits, so a reader written for the header before it reads the columns
// it knew unchanged.
func TestCSVQueuedMsIsTheLastColumn(t *testing.T) {
	if last := CSVHeader[len(CSVHeader)-1]; last != "queued_ms" {
		t.Fatalf("the header ends with %q, want queued_ms", last)
	}
	if i := slices.Index(CSVHeader, "queued_ms"); i != len(CSVHeader)-1 {
		t.Fatalf("queued_ms sits at %d of %d, the tail alone", i, len(CSVHeader))
	}
	var b bytes.Buffer
	if err := WriteCSV(&b, nil); err != nil {
		t.Fatal(err)
	}
	got, err := csv.NewReader(&b).Read()
	if err != nil {
		t.Fatal(err)
	}
	if last := got[len(got)-1]; last != "queued_ms" {
		t.Fatalf("the written header ends with %q, want queued_ms", last)
	}
}

// queued_ms is the ms the call waited for one of its key's or account's
// slots. A record from before the field was kept has none in it, and that
// reads 0: the same 0 a call that never waited writes.
func TestCSVTailsQueuedAndOldRecordsReadZero(t *testing.T) {
	waited := Record{Time: time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC), Agent: "claude", Provider: "p", Model: "m", Status: 200, Queued: 1234}
	old := Record{Time: waited.Time, Agent: "claude", Provider: "p", Model: "m", Status: 200}
	// an old record is one whose JSON has no queued_ms in it: reading one
	// back is a wait of 0, as the field's absence is
	line, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(line), "queued_ms") {
		t.Fatalf("an old record carries a wait: %s", line)
	}
	var read Record
	if err := json.Unmarshal(line, &read); err != nil {
		t.Fatal(err)
	}
	if read.Queued != 0 {
		t.Fatalf("an old record read a wait of %d, want 0", read.Queued)
	}
	var b bytes.Buffer
	if err := WriteCSV(&b, []Row{{Record: waited}, {Record: old}}); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&b).ReadAll()
	if err != nil || len(rows) != 3 {
		t.Fatalf("the export read %d rows (%v), want the header and two calls", len(rows), err)
	}
	q := slices.Index(rows[0], "queued_ms")
	if q != len(rows[0])-1 {
		t.Fatalf("queued_ms is column %d of %d, the tail alone", q, len(rows[0]))
	}
	if rows[1][q] != "1234" {
		t.Fatalf("the waiting call's %s, want 1234", rows[1][q])
	}
	if rows[2][q] != "0" {
		t.Fatalf("the old record's %s, want 0", rows[2][q])
	}
	// and every row answers as many cells as the header names
	for i, row := range rows[1:] {
		if len(row) != len(CSVHeader) {
			t.Fatalf("row %d has %d cells, the header's %d", i+1, len(row), len(CSVHeader))
		}
	}
}
