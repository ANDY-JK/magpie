package usage

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// logRecordsAt reads every record in path.
func logRecordsAt(t *testing.T, path string) []Record {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	defer f.Close()
	var out []Record
	rd := bufio.NewReaderSize(f, 64<<10)
	for {
		line, err := rd.ReadBytes('\n')
		if len(line) > 0 {
			var r Record
			if json.Unmarshal(line, &r) != nil {
				t.Fatalf("%s: %q", path, line)
			}
			out = append(out, r)
		}
		if err != nil {
			return out
		}
	}
}

// A rotation moves the records older than the windows the readers use to a
// dated file and keeps the rest in the active log: the Usage page's 30 days
// (and Codex's 30-day lead), the week a key's window counts, and the active
// log itself never start from zero. Every record stays readable on disk.
func TestLogRotationKeepsTheWindowsReadersUse(t *testing.T) {
	pageHome(t)
	was := logMaxBytes
	logMaxBytes = 2048
	t.Cleanup(func() {
		logMaxBytes = was
		resetAppendsFailed()
		resetLogRotations()
		closeHeldLog()
	})
	resetAppendsFailed()
	resetLogRotations()

	now := time.Now()
	const oldN, monthN, weekN = 20, 10, 10
	for i := 0; i < oldN; i++ { // older than every window the readers use
		Append(Record{Time: now.AddDate(0, 0, -60), Agent: "old", Provider: "relay", Model: "m", Input: i, Status: 200})
	}
	for i := 0; i < monthN; i++ { // inside the Usage page's 30-day period
		Append(Record{Time: now.AddDate(0, 0, -20), Agent: "month", Provider: "relay", Model: "m", Input: i, Status: 200})
	}
	for i := 0; i < weekN; i++ { // inside a key's week
		Append(Record{Time: now.Add(-time.Hour), Agent: "week", Provider: "relay", Model: "m", Input: i, Status: 200})
	}
	Flush()

	active := Load(time.Time{})
	if len(active) != monthN+weekN {
		t.Fatalf("the active log holds %d records, want the %d inside the window", len(active), monthN+weekN)
	}
	for _, r := range active {
		if r.Agent == "old" {
			t.Fatalf("a record older than the retained window is still active: %+v", r)
		}
	}
	// The readers' windows: the Usage page's month, a key's week, and the
	// raw scan budget does from a window's start (usage.Visit).
	if got := Summarize(Month).Calls; got != monthN+weekN {
		t.Fatalf("Summarize(30d) counts %d calls, want %d", got, monthN+weekN)
	}
	countVisit := func(since time.Time) int {
		n := 0
		Visit(since, func(Record) { n++ })
		return n
	}
	if got := countVisit(now.AddDate(0, 0, -30)); got != monthN+weekN {
		t.Fatalf("Visit(30d) sees %d calls, want %d", got, monthN+weekN)
	}
	if got := countVisit(now.Add(-7 * 24 * time.Hour)); got != weekN {
		t.Fatalf("Visit(7d) sees %d calls, want %d", got, weekN)
	}

	// The old records are not lost: they are in a dated file beside the log.
	files, err := filepath.Glob(filepath.Join(filepath.Dir(Path()), "usage-*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("the log never rotated")
	}
	found := map[int]int{}
	for _, path := range files {
		for _, r := range logRecordsAt(t, path) {
			if r.Agent == "old" {
				found[r.Input]++
			}
		}
	}
	if len(found) != oldN {
		t.Fatalf("%d of %d old records readable after rotation", len(found), oldN)
	}
	for i := 0; i < oldN; i++ {
		if found[i] != 1 {
			t.Fatalf("old record %d read %d time(s)", i, found[i])
		}
	}
	if files, rows := LogRotations(); files == 0 || rows != oldN {
		t.Fatalf("rotation counts: %d file(s), %d row(s); want the %d old records", files, rows, oldN)
	}
	if skips := LogRotationSkips(); skips == 0 {
		t.Fatal("the over-cap active window was not reported as a skip")
	}
	if drops := AppendsFailed(); drops != 0 {
		t.Fatalf("%d records counted as dropped", drops)
	}
}

// A rotation that can't rename keeps the log and goes on writing it: the
// failure is counted and the records are not dropped, old or new.
func TestLogRotationFailureKeepsEveryRecord(t *testing.T) {
	pageHome(t)
	wasMax, wasRename := logMaxBytes, renameLog
	logMaxBytes = 1024
	renameLog = func(string, string) error { return errors.New("no rename here") }
	t.Cleanup(func() {
		logMaxBytes, renameLog = wasMax, wasRename
		resetAppendsFailed()
		resetLogRotations()
		closeHeldLog()
	})
	resetAppendsFailed()
	resetLogRotations()

	now := time.Now()
	const n = 20
	for i := 0; i < n; i++ {
		at := now.AddDate(0, 0, -60)
		if i%2 == 0 {
			at = now.Add(-time.Hour)
		}
		Append(Record{Time: at, Agent: "keep", Provider: "relay", Model: "m", Input: i, Status: 200})
	}
	Flush()
	if failures := LogRotateFailures(); failures == 0 {
		t.Fatal("a rotation that could not rename was not counted")
	}
	if files, rows := LogRotations(); files != 0 || rows != 0 {
		t.Fatalf("a failed rotation counted as done: %d files, %d rows", files, rows)
	}
	if drops := AppendsFailed(); drops != 0 {
		t.Fatalf("%d records counted as dropped", drops)
	}
	if recs := Load(time.Time{}); len(recs) != n {
		t.Fatalf("%d of %d records kept", len(recs), n)
	}
}

// When every record is inside the window the readers use, a rotation has
// nothing to move: it is skipped, the log keeps growing, and no record is
// lost or hidden.
func TestLogRotationSkippedWithinRetention(t *testing.T) {
	pageHome(t)
	was := logMaxBytes
	logMaxBytes = 1024
	t.Cleanup(func() {
		logMaxBytes = was
		resetAppendsFailed()
		resetLogRotations()
		closeHeldLog()
	})
	resetAppendsFailed()
	resetLogRotations()

	const n = 30
	for i := 0; i < n; i++ {
		Append(Record{Time: time.Now().Add(-time.Hour), Agent: "recent", Provider: "relay", Model: "m", Input: i, Status: 200})
	}
	Flush()
	if skips := LogRotationSkips(); skips == 0 {
		t.Fatal("a rotation with nothing old enough was not reported")
	}
	if files, rows := LogRotations(); files != 0 || rows != 0 {
		t.Fatalf("a skipped rotation counted as done: %d files, %d rows", files, rows)
	}
	if recs := Load(time.Time{}); len(recs) != n {
		t.Fatalf("%d of %d records kept", len(recs), n)
	}
	if drops := AppendsFailed(); drops != 0 {
		t.Fatalf("%d records counted as dropped", drops)
	}
}
