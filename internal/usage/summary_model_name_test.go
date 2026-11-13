package usage

import (
	"os"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/settings"
)

// A model's group carries what the user calls it (Group.ModelName) beside
// the id the counts are keyed on (Model): the Usage page shows the name in
// the id's place, and a clear of the model's usage still goes by the id.
func TestSummaryModelsCarryTheirNames(t *testing.T) {
	pageHome(t)
	now := holdClock(t, time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local))
	s := settings.Load()
	s.ModelNames = map[string]string{"relay/m": "Mine"}
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	Append(Record{Agent: "codex", Provider: "relay", Model: "m", Input: 10, Status: 200})
	su := Summarize(Today)
	if len(su.Models) != 1 {
		t.Fatalf("models: %+v", su.Models)
	}
	g := su.Models[0]
	if g.Model != "m" {
		t.Fatalf("the counts still key on the id: %+v", g)
	}
	if g.ModelName != "Mine" {
		t.Fatalf("the name the user gave it: %q", g.ModelName)
	}
	// a model no name was given keeps its group's empty
	s = settings.Load()
	s.ModelNames = map[string]string{"relay/other": "Not this one"}
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	direct := summarize(Today, now, Load(time.Time{}))
	if direct.Models[0].ModelName != "" {
		t.Fatalf("an unnamed model named: %q", direct.Models[0].ModelName)
	}
}

// A rename recounts: the summaries' cache is keyed on the names the model
// groups carry (usagePriceCacheKey), so a name taken back shows on the page
// at once, not when some other change lands. The file's old stamp and size
// are put back, as a save can land within the stamp's granularity — the new
// name spelled as long as the old — and the stat alone then says nothing
// changed: the names in the hash are what recount.
func TestSummaryRecountsWhenAModelIsRenamed(t *testing.T) {
	pageHome(t)
	now := holdClock(t, time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local))
	Append(Record{Agent: "codex", Provider: "relay", Model: "m", Input: 10, Status: 200})
	s := settings.Load()
	s.ModelNames = map[string]string{"relay/m": "Mine"}
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	first := Summarize(Today)
	if len(first.Models) != 1 || first.Models[0].ModelName != "Mine" {
		t.Fatalf("the named summary: %+v", first.Models)
	}
	// renamed again, to a name as long as the first, under the stamp the
	// first answer was keyed on
	old, err := os.Stat(settings.Path())
	if err != nil {
		t.Fatal(err)
	}
	s = settings.Load()
	s.ModelNames = map[string]string{"relay/m": "Ours"}
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(settings.Path(), old.ModTime(), old.ModTime()); err != nil {
		t.Fatal(err)
	}
	direct := summarize(Today, now, Load(time.Time{}))
	if len(direct.Models) != 1 || direct.Models[0].ModelName != "Ours" {
		t.Fatalf("the renamed summary: %+v", direct.Models)
	}
	if next := Summarize(Today); next.Models[0].ModelName != direct.Models[0].ModelName {
		t.Fatalf("cached summary stale after a rename: %q, want %q", next.Models[0].ModelName, direct.Models[0].ModelName)
	}
}
