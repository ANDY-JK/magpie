package routes

import (
	"math"
	"path/filepath"
	"slices"
	"testing"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// scoreHome gives the test a settings home of its own: Find reads
// settings.json for the score routing's switch.
func scoreHome(t *testing.T) {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
}

// Score is one of Builtin, as plugins/rotation registers it, and asks the
// gateway for the health and speed it weighs.
func TestScoreIsBuiltin(t *testing.T) {
	s, ok := Builtin[provider.Score]
	if !ok || s.Order == nil {
		t.Fatalf("Builtin[%q] = %+v, %v; want a strategy", provider.Score, s, ok)
	}
	if !s.ByHealth || !s.BySpeed || !s.ByUse {
		t.Fatalf("score doesn't ask for what it weighs: %+v", s)
	}
}

// Served while ScoreRouting is on, the default: Find knows it once a plugin
// registers it, as plugins/rotation does with every Builtin.
func TestScoreIsServed(t *testing.T) {
	scoreHome(t)
	remove := Register(provider.Score, Builtin[provider.Score])
	t.Cleanup(remove)
	if s, ok := Find(provider.Score); !ok || s.Order == nil {
		t.Fatalf("score isn't served: %+v, %v", s, ok)
	}
	if !Served(provider.Score) {
		t.Fatal("Served says score isn't served")
	}
}

// Off, the score routing isn't served though a plugin registered it: a
// saved one is tried in its listed order, as a routing no plugin serves is.
// On again, it is.
func TestScoreRoutingOffIsNotServed(t *testing.T) {
	scoreHome(t)
	remove := Register(provider.Score, Builtin[provider.Score])
	t.Cleanup(remove)
	s := settings.Load()
	if !s.ScoreRoutingOn() {
		t.Fatal("score routing is off by default")
	}
	off := false
	s.ScoreRouting = &off
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	if st, ok := Find(provider.Score); ok {
		t.Fatalf("score is served with ScoreRouting off: %+v", st)
	}
	if Served(provider.Score) {
		t.Fatal("Served says score is served with ScoreRouting off")
	}
	s.ScoreRouting = nil // never chose: on, the default
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	if _, ok := Find(provider.Score); !ok {
		t.Fatal("score isn't served again with ScoreRouting on")
	}
}

// Quota is the layer before the score: a seat all but used up goes behind
// one with room however well it has answered or how fast it is — 99 points
// on every other measure don't spend the last of an allowance while another
// has room — and one about to lose what it has at a reset goes first.
func TestScoreQuotaComesBeforeTheScore(t *testing.T) {
	s := []Seat{
		{Key: "spent", Known: true, Used: 99, Health: 1, TTFT: 100, TPS: 60, Pace: provider.FreshPace},
		{Key: "room", Known: true, Used: 85, Health: 0.8, TTFT: 2000, TPS: 10, Pace: provider.FreshPace},
	}
	if got := keys(s, Apply(Score, "p", s)); !slices.Equal(got, []string{"room", "spent"}) {
		t.Fatalf("used up first: %v, want [room spent]", got)
	}
	// the same at the 90% line, and one running low before one all but used
	// up
	s = []Seat{
		{Key: "spent", Known: true, Used: 99, Health: 1, TTFT: 100, TPS: 60, Pace: provider.FreshPace},
		{Key: "low", Known: true, Used: 95, Health: 1, TTFT: 100, TPS: 60, Pace: provider.FreshPace},
		{Key: "room", Known: true, Used: 10, Health: 0.5, TTFT: 3000, TPS: 1, Pace: provider.FreshPace},
	}
	if got := keys(s, Apply(Score, "p", s)); !slices.Equal(got, []string{"room", "low", "spent"}) {
		t.Fatalf("tiers: %v, want [room low spent]", got)
	}
	// what it has is lost at the reset: spent now, whatever it used
	s = []Seat{
		{Key: "full", Known: true, Used: 5, Health: 1, TTFT: 100, TPS: 60, Pace: provider.FreshPace},
		{Key: "loses", Known: true, Used: 96, Loses: true, Health: 0.2, TTFT: 3000, TPS: 1, Pace: provider.FreshPace},
	}
	if got := keys(s, Apply(Score, "p", s)); !slices.Equal(got, []string{"loses", "full"}) {
		t.Fatalf("about to lose: %v, want [loses full]", got)
	}
}

// Reliability is the layer after quota: a seat that has answered since goes
// before one that failed since, the one failing most last, and one resting
// after every one awake.
func TestScoreFailuresAndResting(t *testing.T) {
	room := func(key string) Seat {
		return Seat{Key: key, Known: true, Used: 10, Health: 0.5, TTFT: 1000, TPS: 30, Pace: provider.FreshPace}
	}
	s := []Seat{room("rest"), room("failed"), room("ok")}
	s[0].Resting = true
	s[1].Failures = 2
	if got := keys(s, Apply(Score, "p", s)); !slices.Equal(got, []string{"ok", "failed", "rest"}) {
		t.Fatalf("awake, failing, resting: %v, want [ok failed rest]", got)
	}
	s = []Seat{room("two"), room("one"), room("ok")}
	s[0].Failures, s[1].Failures = 2, 1
	if got := keys(s, Apply(Score, "p", s)); !slices.Equal(got, []string{"ok", "one", "two"}) {
		t.Fatalf("the failing most last: %v, want [ok one two]", got)
	}
	// a resting seat with the best score stays behind every awake one
	s = []Seat{room("resting"), room("awake")}
	s[0].Resting, s[0].Health, s[0].TTFT, s[0].TPS = true, 1, 100, 60
	if got := keys(s, Apply(Score, "p", s)); !slices.Equal(got, []string{"awake", "resting"}) {
		t.Fatalf("resting with the best score: %v, want [awake resting]", got)
	}
}

// Within their layers seats go by the score: health first, then the first
// token and the pace, and what the allowance has left.
func TestScoreGoesByItsScore(t *testing.T) {
	base := Seat{Key: "base", Known: true, Used: 10, Health: 0.6, TTFT: 1000, TPS: 30, Pace: provider.FreshPace / 2}
	// the edited seat goes after the base one, so a tie keeps the base
	// first and the case is seen to decide
	other := func(edit func(*Seat)) []Seat {
		a, b := base, base
		a.Key = "better"
		edit(&a)
		return []Seat{b, a}
	}
	for _, c := range []struct {
		name string
		s    []Seat
	}{
		{"health", other(func(s *Seat) { s.Health = 0.9 })},
		{"first token", other(func(s *Seat) { s.TTFT = 200 })},
		{"throughput", other(func(s *Seat) { s.TPS = 60 })},
		{"quota left", other(func(s *Seat) { s.Used = 5 })},
		{"pace", other(func(s *Seat) { s.Pace = provider.FreshPace })},
	} {
		if got := keys(c.s, Apply(Score, "p", c.s)); got[0] != "better" {
			t.Errorf("%s: %v, want better first", c.name, got)
		}
	}
	// health outweighs both speed dimensions together: a steady seat goes
	// before a faster one that keeps losing
	s := []Seat{
		{Key: "steady", Known: true, Used: 10, Health: 1, TTFT: 3000, TPS: 1, Pace: provider.FreshPace},
		{Key: "fast", Known: true, Used: 10, Health: 0.1, TTFT: 10, TPS: 60, Pace: provider.FreshPace},
	}
	if got := keys(s, Apply(Score, "p", s)); !slices.Equal(got, []string{"steady", "fast"}) {
		t.Fatalf("health against speed: %v, want [steady fast]", got)
	}
}

// A dimension with nothing measured lately counts neutral, not nothing: a
// new seat is neither put first nor last for being new, and one that has
// done badly goes behind it.
func TestScoreNoDataIsNeutral(t *testing.T) {
	s := []Seat{
		{Key: "bad", Known: true, Used: 50, Health: 0.2, TTFT: 2500, TPS: 10, Pace: provider.FreshPace},
		{Key: "new"}, // no record, no allowance read
	}
	if got := keys(s, Apply(Score, "p", s)); !slices.Equal(got, []string{"new", "bad"}) {
		t.Fatalf("neutral: %v, want [new bad]", got)
	}
	// a measured seat that is good goes before the unmeasured one
	s = []Seat{
		{Key: "new"},
		{Key: "good", Known: true, Used: 0, Health: 1, TTFT: 200, TPS: 60, Pace: provider.FreshPace},
	}
	if got := keys(s, Apply(Score, "p", s)); !slices.Equal(got, []string{"good", "new"}) {
		t.Fatalf("measured: %v, want [good new]", got)
	}
}

// The five dimensions score weighs share it whole: a weight left over, or
// one missing, would lift or sink every seat alike.
func TestScoreWeightsSumToOne(t *testing.T) {
	if sum := scoreWeightHealth + scoreWeightTTFT + scoreWeightTPS + scoreWeightQuota + scoreWeightAbility; math.Abs(sum-1) > 1e-9 {
		t.Fatalf("the weights sum to %v, want 1", sum)
	}
}

// Seats alike keep their listed order, so a pool alike throughout keeps the
// vendor's prompt cache warm.
func TestScoreAlikeKeepsTheListedOrder(t *testing.T) {
	s := []Seat{
		{Key: "a", Known: true, Used: 20, Health: 0.6, TTFT: 800, TPS: 20, Pace: provider.FreshPace},
		{Key: "b", Known: true, Used: 20, Health: 0.6, TTFT: 800, TPS: 20, Pace: provider.FreshPace},
		{Key: "c", Known: true, Used: 20, Health: 0.6, TTFT: 800, TPS: 20, Pace: provider.FreshPace},
	}
	if got := keys(s, Apply(Score, "p", s)); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("alike: %v", got)
	}
}
