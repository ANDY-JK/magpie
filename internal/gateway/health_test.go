package gateway

import (
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// two candidates of different models, as a group's plan holds them
func healthCandidates() []candidate {
	return []candidate{
		{p: provider.Provider{ID: "a", Name: "A", Key: "k1"}, model: "good", rest: "a"},
		{p: provider.Provider{ID: "b", Name: "B", Key: "k2"}, model: "flaky", rest: "b"},
	}
}

func healthOrder(cs []candidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.model
	}
	return out
}

// An intermittent failure moves a model's score down and, below the
// threshold, behind its steadier peers — even though it answers again
// between them (the old consecutive-failure ladder forgot it on the first
// answer). Quota failures never touch the score.
func TestUnstableModelIsTriedAfterTheSteadyOnes(t *testing.T) {
	cs := healthCandidates()
	pl := planned{order: []Weighed{{Model: "good"}, {Model: "flaky"}}}
	good, flaky := cs[0], cs[1]

	// one failure: still tried first (0.6), its position unchanged
	noteUnstable(flaky)
	if unstable(flaky) {
		t.Fatal("one failure demoted the model")
	}
	// an answer between failures: the score recovers a little, to about
	// 0.74, but the failure is not forgotten
	noteStable(flaky)
	// two more failures take it below the threshold even with the answer
	noteUnstable(flaky)
	noteUnstable(flaky)
	if !unstable(flaky) {
		t.Fatalf("intermittent failures did not demote the model: score %v", healthOf(flaky.restID(), time.Now()))
	}
	out, pl2 := demoteUnstable(cs, pl)
	if got := healthOrder(out); got[0] != "good" || got[1] != "flaky" {
		t.Fatalf("order %v, want the steady one first", got)
	}
	if pl2.order[0].Model != "good" || pl2.order[1].Model != "flaky" {
		t.Fatalf("pl.order not kept with the candidates: %v", pl2.order)
	}
	// a steady model is untouched
	if unstable(good) {
		t.Fatal("the steady model was demoted")
	}
}

// A quota failure is not instability: the model is out of allowance, not
// broken, and its health stays where it was.
func TestQuotaFailureLeavesHealthAlone(t *testing.T) {
	fresh(t)
	cs := healthCandidates()
	s := &Server{}
	flaky := cs[1]
	before := healthOf(flaky.restID(), time.Now())
	s.restAfterMarked(flaky, 429, nil, []byte(`{"error":{"message":"rate limit exceeded: quota used up","type":"rate_limit_exceeded"}}`), false)
	if after := healthOf(flaky.restID(), time.Now()); after < before {
		t.Fatalf("a quota failure counted against health: %v → %v", before, after)
	}
	// while an unknown failure does count
	s.restAfterMarked(flaky, 502, nil, []byte(`{"error":{"message":"bad gateway"}}`), false)
	if after := healthOf(flaky.restID(), time.Now()); after >= before {
		t.Fatalf("an unknown failure did not count: %v → %v", before, after)
	}
}

// The demotion is a stable partition: healthy and unstable each keep their
// relative order, so the user's routing order still decides within them.
func TestDemotionKeepsEachSidesOrder(t *testing.T) {
	cs := []candidate{
		{p: provider.Provider{ID: "a", Key: "k"}, model: "m1", rest: "a"},
		{p: provider.Provider{ID: "b", Key: "k"}, model: "m2", rest: "b"},
		{p: provider.Provider{ID: "c", Key: "k"}, model: "m3", rest: "c"},
		{p: provider.Provider{ID: "d", Key: "k"}, model: "m4", rest: "d"},
	}
	pl := planned{order: []Weighed{{Model: "m1"}, {Model: "m2"}, {Model: "m3"}, {Model: "m4"}}}
	noteUnstable(cs[0])
	noteUnstable(cs[0])
	noteUnstable(cs[2])
	noteUnstable(cs[2])
	out, _ := demoteUnstable(cs, pl)
	want := []string{"m2", "m4", "m1", "m3"}
	if got := healthOrder(out); !equalStrings(got, want) {
		t.Fatalf("order %v, want %v", got, want)
	}
}

// A model that can answer now is tried before one that is resting, even
// when it is the unstable one: a rest is a wait, not a preference, and the
// demotion must not put a model that can answer behind one that can't.
func TestUnstableModelIsTriedBeforeARestingOne(t *testing.T) {
	cs := []candidate{
		{p: provider.Provider{ID: "a", Key: "k"}, model: "flaky", rest: "a"},
		{p: provider.Provider{ID: "b", Key: "k"}, model: "out", rest: "b"},
	}
	rest := Rest{Why: failQuota, Until: time.Now().Add(time.Hour)}
	pl := planned{order: []Weighed{{Model: "flaky"}, {Model: "out", Rest: &rest}}}
	noteUnstable(cs[0])
	noteUnstable(cs[0])
	if !unstable(cs[0]) {
		t.Fatal("fixture: the model is not unstable")
	}
	out, pl2 := demoteUnstable(cs, pl)
	if got := healthOrder(out); got[0] != "flaky" {
		t.Fatalf("order %v, want the one that can answer first", got)
	}
	if pl2.order[0].Model != "flaky" {
		t.Fatalf("pl.order not kept with the candidates: %v", pl2.order)
	}
}

// The demotion never leaves a subscription unused: an unstable model with
// allowance left is still asked when the steady ones can't answer. A rest
// is a wait, and what is left of a large allowance must be spent rather
// than held back by a health score — the user's own concern about it.
func TestUnstableModelIsStillAskedWhenTheSteadyOnesCannotAnswer(t *testing.T) {
	soon, later := time.Now().Add(time.Minute), time.Now().Add(time.Hour)
	r1, r2 := Rest{Why: failQuota, Until: soon}, Rest{Why: failQuota, Until: later}
	cs := []candidate{
		{p: provider.Provider{ID: "a", Key: "k"}, model: "steady-a", rest: "a"},
		{p: provider.Provider{ID: "b", Key: "k"}, model: "steady-b", rest: "b"},
		{p: provider.Provider{ID: "c", Key: "k"}, model: "big", rest: "c"},
	}
	pl := planned{order: []Weighed{{Model: "steady-a", Rest: &r1}, {Model: "steady-b", Rest: &r2}, {Model: "big"}}}
	noteUnstable(cs[2])
	noteUnstable(cs[2])
	if !unstable(cs[2]) {
		t.Fatal("fixture: the model is not unstable")
	}
	out, _ := demoteUnstable(cs, pl)
	if len(out) != len(cs) {
		t.Fatalf("the demotion dropped a candidate: %d of %d", len(out), len(cs))
	}
	out = probeOne(out)
	if len(out) == 0 || out[0].model != "big" {
		t.Fatalf("order %v, want the model with allowance left asked first", healthOrder(out))
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The routing page says a model is unstable: the weighed row carries the
// health, absent while it is fine.
func TestWeighedCarriesHealth(t *testing.T) {
	cs := healthCandidates()
	c := cs[1]
	noteUnstable(c)
	noteUnstable(c)
	w := weighed(c, c.p, weighing{}, false, provider.Chat)
	if w.Health == 0 || w.Health >= 0.5 {
		t.Fatalf("health %v, want below 0.5", w.Health)
	}
	good := weighed(cs[0], cs[0].p, weighing{}, false, provider.Chat)
	if good.Health != 0 {
		t.Fatalf("a steady model's row says health %v", good.Health)
	}
}
