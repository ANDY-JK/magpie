package gateway

import (
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// A model that fails about half its calls settles at healthDemote exactly
// with a fixed multiplier, and never crosses it: it would sit on the line
// forever while a peer answers every time. Beside that peer it is plainly
// the worse one and is tried last, which is what the relative rule is for.
func TestAModelFailingHalfItsCallsIsTriedLast(t *testing.T) {
	resetHealth()
	t.Cleanup(resetHealth)
	// the half-failing model comes first in the plan, so only a real
	// demotion moves it: a plan already in the wanted order proves nothing
	good := candidate{p: provider.Provider{ID: "a", Name: "A", Key: "k1"}, model: "good", rest: "a"}
	flaky := candidate{p: provider.Provider{ID: "b", Name: "B", Key: "k2"}, model: "flaky", rest: "b"}
	cs := []candidate{flaky, good}
	pl := planned{order: []Weighed{{Model: "flaky"}, {Model: "good"}}}

	// a long, evenly split record: 20 answers, 20 intermittent failures.
	// Each failure is followed by an answer, so the old consecutive-failure
	// ladder never saw a run at all.
	for i := 0; i < 20; i++ {
		noteStable(flaky)
		noteUnstable(flaky)
	}
	for i := 0; i < 40; i++ {
		noteStable(good)
	}

	score := healthOf(flaky.restID(), time.Now())
	// the point of the relative rule: the absolute one never fires here —
	// a model failing half its calls settles just above the line
	if score < healthDemote {
		t.Fatalf("fixture: score %v, want it settled just above the line", score)
	}
	if unstable(flaky) {
		t.Fatal("fixture: the absolute rule caught it, so this proves nothing")
	}
	// the user's order puts the half-failing model FIRST, so only a real
	// demotion moves it
	out, _ := demoteUnstable(cs, pl)
	if got := healthOrder(out); got[0] != "good" {
		t.Fatalf("order %v, want the steady one moved ahead (score %v)", got, score)
	}
}

// The relative rule does not fire on a model with no record of its own: a
// fresh model beside one with a long, perfect history is unknown, not
// unstable, and keeps its place until it has done something.
func TestAFreshModelBesideAPerfectOneIsNotDemoted(t *testing.T) {
	resetHealth()
	t.Cleanup(resetHealth)
	cs := healthCandidates()
	good, fresh := cs[0], cs[1]
	for i := 0; i < 40; i++ {
		noteStable(good)
	}
	m := markOf(cs, time.Now())
	if h, ok := m.known[fresh.restID()]; ok && h.wins+h.loss >= healthEnough {
		t.Fatal("fixture: the fresh model has a record")
	}
	out, _ := demoteUnstable(cs, planned{order: []Weighed{{Model: "good"}, {Model: "flaky"}}})
	if got := healthOrder(out); got[0] != "good" || got[1] != "flaky" {
		t.Fatalf("order %v, want the user's order kept", got)
	}
}

// A single blip on a model with a short record is not enough for the
// relative rule: two or three calls do not make a track record.
func TestABlipOnAShortRecordIsNotDemoted(t *testing.T) {
	resetHealth()
	t.Cleanup(resetHealth)
	cs := healthCandidates()
	good, flaky := cs[0], cs[1]
	for i := 0; i < 40; i++ {
		noteStable(good)
	}
	noteStable(flaky)
	noteUnstable(flaky)
	noteStable(flaky) // 3 observations: still under healthEnough
	out, _ := demoteUnstable(cs, planned{order: []Weighed{{Model: "good"}, {Model: "flaky"}}})
	if got := healthOrder(out); got[0] != "good" || got[1] != "flaky" {
		t.Fatalf("order %v, want the short record left alone", got)
	}
}
