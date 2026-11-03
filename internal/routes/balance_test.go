package routes

import (
	"slices"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// Balanced spreads a band by what magpie sent each lately, against its
// weight: seats alike — the same accounts, renewing together — are taken in
// turn rather than the first being drained until it is nearly out, and one
// weighing 2 carries twice what one weighing 1 does between their turns.
func TestBalancedSpreadsAlikeSeats(t *testing.T) {
	s := []Seat{
		{Key: "a", Pace: 0.5},
		{Key: "b", Pace: 0.5},
		{Key: "c", Pace: 0.5},
	}
	if got := keys(s, Apply(Balanced, "p", s)); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("alike: %v", got)
	}
	// the one that answered keeps behind those that haven't
	s[0].Tokens = 100
	if got := keys(s, Apply(Balanced, "p", s)); !slices.Equal(got, []string{"b", "c", "a"}) {
		t.Fatalf("after a served: %v", got)
	}
	// sent 100, weighing 2, it has charge of 50 against b's 60: it goes first
	s[0].Weight, s[1].Tokens, s[2].Tokens = 2, 60, 200
	if got := keys(s, Apply(Balanced, "p", s)); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("weighing 2: %v, want [a b c]", got)
	}
	// weighing 1 again, what was sent decides
	s[0].Weight = 1
	if got := keys(s, Apply(Balanced, "p", s)); !slices.Equal(got, []string{"b", "a", "c"}) {
		t.Fatalf("weighing 1: %v, want [b a c]", got)
	}
}

// A seat that has failed since it last answered goes behind those that have
// answered since, and a seat whose vendor's seats in the pool are failing
// goes behind another vendor's: a vendor that refuses magpie refuses its
// accounts alike, and one it happened to ask is enough to find out.
func TestBalancedFailingSeatAndVendor(t *testing.T) {
	// its own failure, its vendor's seats weighing the same: the one that
	// failed goes behind though it was sent less
	s := []Seat{
		{Key: "x1", Vendor: "x", Pace: 0.5, Tokens: 100},
		{Key: "x2", Vendor: "x", Pace: 0.5, Failures: 2},
	}
	if got := keys(s, Apply(Balanced, "p", s)); !slices.Equal(got, []string{"x1", "x2"}) {
		t.Fatalf("the seat that failed: %v, want [x1 x2]", got)
	}
	// its vendor's: x1 hasn't failed, but x's other seat has
	s = []Seat{
		{Key: "x1", Vendor: "x", Pace: 0.5},
		{Key: "x2", Vendor: "x", Pace: 0.5, Failures: 3},
		{Key: "y1", Vendor: "y", Pace: 0.5, Tokens: 500}, // the most sent lately: first without it
	}
	if got := keys(s, Apply(Balanced, "p", s)); !slices.Equal(got, []string{"y1", "x1", "x2"}) {
		t.Fatalf("a failing vendor: %v, want [y1 x1 x2]", got)
	}
	// no seat of either vendor failing: what was sent decides, and x1 goes first
	s[1].Failures = 0
	if got := keys(s, Apply(Balanced, "p", s)); !slices.Equal(got, []string{"x1", "x2", "y1"}) {
		t.Fatalf("vendors alike: %v, want [x1 x2 y1]", got)
	}
}

// A seat answering more than four times more slowly than the fastest of the
// pool goes behind the rest of its band; one within four times is weighed by
// what it was sent, so a small difference in speed doesn't drain the fast
// one.
func TestBalancedSlowSeatGoesBehind(t *testing.T) {
	s := []Seat{
		{Key: "slow", Pace: 0.5, TTFT: 3000, TPS: 5}, // 3 s to the first token, 5 a second
		{Key: "fast", Pace: 0.5, TTFT: 500, TPS: 50, Tokens: 1000},
	}
	if got := keys(s, Apply(Balanced, "p", s)); !slices.Equal(got, []string{"fast", "slow"}) {
		t.Fatalf("a crawling seat: %v, want [fast slow]", got)
	}
	s[0].TTFT, s[0].TPS = 600, 50 // twice the first token, as fast a writer
	if got := keys(s, Apply(Balanced, "p", s)); !slices.Equal(got, []string{"slow", "fast"}) {
		t.Fatalf("alike speeds: %v, want [slow fast]", got)
	}
	// not measured lately is not slow: it is tried, and its answer measures it
	s[0].TTFT, s[0].TPS = 0, 0
	if got := keys(s, Apply(Balanced, "p", s)); !slices.Equal(got, []string{"slow", "fast"}) {
		t.Fatalf("not measured: %v, want [slow fast]", got)
	}
}

// Balanced's tiers are Smart's: those with room go before those running low,
// and those before the ones all but used up, whatever was sent each.
func TestBalancedTiers(t *testing.T) {
	s := []Seat{
		{Key: "spent", Used: 99, Pace: 9},
		{Key: "low", Used: 92, Pace: 9},
		{Key: "fine", Used: 20, Pace: 0.05, Tokens: 10000},
	}
	if got := keys(s, Apply(Balanced, "p", s)); !slices.Equal(got, []string{"fine", "low", "spent"}) {
		t.Fatalf("tiers: %v, want [fine low spent]", got)
	}
}

// A key that reads no windows has no pace, so it is in the last band of those
// with room: it is spent after subscriptions spending quota they would lose
// at their reset, and not before, as #4535 asked.
func TestBalancedKeyGoesBehindSubscriptions(t *testing.T) {
	s := []Seat{
		{Key: "key"},
		{Key: "sub", Known: true, Used: 20, Pace: 0.1, Tokens: 1000},
	}
	if got := keys(s, Apply(Balanced, "p", s)); !slices.Equal(got, []string{"sub", "key"}) {
		t.Fatalf("a key: %v, want [sub key]", got)
	}
}

// An account not known yet that tells what it has left as it answers goes
// first once, as in Smart: kept behind the known ones it would never answer,
// and never be known.
func TestBalancedLearnsFirst(t *testing.T) {
	s := []Seat{
		{Key: "known", Known: true, Pace: 0.6},
		{Key: "learns", Learns: true, Tokens: 900},
	}
	if got := keys(s, Apply(Balanced, "p", s)); !slices.Equal(got, []string{"learns", "known"}) {
		t.Fatalf("learning: %v, want [learns known]", got)
	}
}

// Balanced is one of the built-in routings, as plugins/rotation registers
// it, and weighs what its order reads: what was sent each, and their speed.
func TestBalancedIsBuiltin(t *testing.T) {
	s, ok := Builtin[provider.Balanced]
	if !ok || s.Order == nil || !s.ByUse || !s.BySpeed || s.Exact {
		t.Fatalf("balanced isn't registered as it should be: %v %+v", ok, s)
	}
}
