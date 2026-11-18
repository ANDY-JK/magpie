package gateway

// A kept route's line is what a day's file — and its gzip — is made of, and a
// busy route weighs tens of candidates, each a Weighed of tens of fields. The
// line used to carry every field, most of all the ones the Routing page never
// reads from a kept request, so a line averaged 26.5 KB and reached 46 KB at
// 73 candidates (F-15). keptLine drops those unread fields; the rest are the
// GUI replay's own, kept under their JSON names so a day's older lines still
// read.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// aWeighed is one weighed candidate carrying every field the history's
// readers use, and the five keptLine drops set as well: a route whose line
// still names them is one written whole (F-15).
func aWeighed(id, model, who string) Weighed {
	now := time.Date(2026, 9, 30, 11, 0, 0, 0, time.Local)
	bal := 12.5
	return Weighed{
		ID: id, Provider: "codex", Name: "Codex", Icon: "/icons/codex.svg", Preset: "codex",
		Who: who, Kind: "account", Agent: "codex", Plan: "Plus", Model: model,
		Fixed: "high", Fast: true, Routing: "smart", Fallback: true, Known: true, Learns: true,
		Used: 62.4, Amount: 100, Limit: 500, Unit: "credits", Balance: &bal,
		Renews: []time.Time{now.Add(5 * time.Hour), now.Add(24 * time.Hour)}, Pace: 4.2,
		Due: &now, Class: 1, Loses: true, Tokens: 1234567, Fails: 2, Free: true,
		LateTTFT: 820, LateTPS: 45.5, Turn: true, Fit: 2, Speaks: "openai", Unlisted: true,
		Window: 128000, DueBy: "reset", Barred: true, Held: true, HeldBy: "g",
		Capped: 3, NoCredits: true, Credits: true, CreditsLast: true, Rank: 3, Aside: true,
		Reserve: 2, NoURL: "openai", Via: []string{"g1", "g2"}, Health: 0.98,
		Price: &SeatPrice{Input: 0.0000025, Output: 0.00001, CacheRead: 2.5e-7},
		Rest:  &Rest{Why: "failCredit", Status: 429, Until: now.Add(time.Hour), By: "window", Failures: 2, Key: id, Model: model},
		// The five keptLine drops: none is read from a kept candidate by the
		// Routing page (client.js) or the tallies.
		Shared: true, Busy: 55.5, CapBack: &now, CreditsFirst: true, Unread: "the usage read failed",
	}
}

// A done route is written without the five candidate fields the history's
// readers never use, so its line is smaller than the whole Route's, and the
// History and the tallies still read back every field they do use. The old
// code, writing Route whole, names those fields and the size assertion and
// the absence assertion both fail (F-15).
func TestHistoryLineOmitsUnreadCandidateFields(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	now := historyNoon(t)
	day := now.Format(dayForm)

	order := []Weighed{aWeighed("acct-1", "gpt-5.5", "alice"), aWeighed("acct-2", "gpt-5.5", "bob")}
	left := []Weighed{aWeighed("acct-3", "gpt-5.5", "carol")}
	r := Route{ID: 7, Time: now, Agent: "codex", Provider: "codex", Model: "gpt-5.5",
		Order: order, Left: left, Done: true, Status: 200,
		Tries: []Try{{ID: "acct-1", Model: "gpt-5.5", Start: now, Done: true, Status: 200, Millis: 100}},
	}
	saveRoute(r)

	path := filepath.Join(HistoryDir(), day+".jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the day: %v", err)
	}
	line := bytes.TrimSpace(raw)
	if len(line) == 0 || bytes.Contains(line, []byte("\n")) {
		t.Fatalf("want one whole line, got %d bytes", len(raw))
	}

	// The five fields keptLine drops are not named in the line — there from a
	// full-keyed line, absent from the slim one.
	for _, key := range []string{`"shared"`, `"busy"`, `"capBack"`, `"creditsFirst"`, `"unread"`} {
		if bytes.Contains(line, []byte(key)) {
			t.Errorf("a kept line still names %s: %s", key, line)
		}
	}

	// Smaller than the whole Route: the old code wrote exactly that, so it
	// fails this.
	saved := r
	saved.Seq = 0
	whole, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if len(line) >= len(whole) {
		t.Fatalf("kept line is %d bytes, not smaller than the whole route's %d", len(line), len(whole))
	}

	// History still replays the kept fields, in order, one candidate each.
	_, routes, _ := History(day)
	if len(routes) != 1 {
		t.Fatalf("History read %d routes, want 1", len(routes))
	}
	got := routes[0]
	if len(got.Order) != 2 || len(got.Left) != 1 {
		t.Fatalf("candidates: %d order, %d left; want 2 and 1", len(got.Order), len(got.Left))
	}
	first := got.Order[0]
	if first.ID != "acct-1" || first.Provider != "codex" || first.Model != "gpt-5.5" || first.Routing != "smart" {
		t.Errorf("kept identity lost: %+v", first)
	}
	if !first.Known || first.Used != 62.4 || first.Balance == nil || *first.Balance != 12.5 || first.Rank != 3 {
		t.Errorf("kept weighing lost: %+v", first)
	}
	if first.Rest == nil || first.Rest.Why != "failCredit" || first.Rest.Status != 429 || first.Rest.Key != "acct-1" {
		t.Errorf("kept rest lost: %+v", first.Rest)
	}
	if first.Price == nil || first.Price.Input != 0.0000025 || len(first.Renews) != 2 {
		t.Errorf("kept price/renews lost: %+v", first)
	}
	if got.Left[0].ID != "acct-3" {
		t.Errorf("left candidate lost: %+v", got.Left[0])
	}

	// The day's tallies still fold the line: both order candidates counted,
	// the answered one's try counted.
	var tallies = map[string]*Tally{}
	foldTally(tallies, line)
	k1, _ := json.Marshal([]string{"codex", "alice"})
	k2, _ := json.Marshal([]string{"codex", "bob"})
	if a := tallies[string(k1)]; a == nil || a.Tried != 1 || a.OK != 1 || a.Provider != "codex" || a.Model != "gpt-5.5" {
		t.Errorf("the answered candidate's tally: %+v (want %s)", a, k1)
	}
	if a := tallies[string(k2)]; a == nil || a.Provider != "codex" || a.Kind != "account" {
		t.Errorf("the other candidate's tally: %+v (want %s)", a, k2)
	}
}
