package gateway

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
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
// health, absent while it is fine. The score comes from the snapshot the
// routing's weighing took under one lock (healthSeats), as a plan's rows
// are filled from it.
func TestWeighedCarriesHealth(t *testing.T) {
	cs := healthCandidates()
	c := cs[1]
	noteUnstable(c)
	noteUnstable(c)
	at := time.Now()
	wg := weighing{health: healthSeats(cs, at)}
	w := weighed(c, c.p, wg, false, provider.Chat)
	if w.Health == 0 || w.Health >= 0.5 {
		t.Fatalf("health %v, want below 0.5", w.Health)
	}
	// the snapshot says exactly what a per-candidate read would, asked of
	// the same moment
	if h := healthOf(c.restID(), at); w.Health != h {
		t.Fatalf("the row's health %v is not the score healthOf answers, %v", w.Health, h)
	}
	good := weighed(cs[0], cs[0].p, wg, false, provider.Chat)
	if good.Health != 0 {
		t.Fatalf("a steady model's row says health %v", good.Health)
	}
}

// A pool routed by score weighs the health the gateway fills into its
// seats: a steady model goes before one that failed twice, though the list
// hands the failing one first and nothing else tells the two apart.
func TestScoreWeighsTheHealthItReads(t *testing.T) {
	fresh(t)
	const model = "m"
	cand := func(id string) candidate {
		return candidate{p: provider.Provider{ID: id, Name: id, Key: "k"}, model: model, rest: id}
	}
	flaky, steady := cand("a"), cand("b")
	noteUnstable(flaky)
	noteUnstable(flaky)
	if !unstable(flaky) {
		t.Fatalf("the failing model isn't unstable: %v", healthOf(flaky.restID(), time.Now()))
	}
	p := provider.Provider{ID: "pool", Routing: provider.Score}
	out, wg := weighRouted(p, []candidate{flaky, steady}, model, provider.Chat, need{})
	if len(out) != 2 || out[0].rest != "b" || out[1].rest != "a" {
		t.Fatalf("order %q then %q, want the steady b first", out[0].rest, out[1].rest)
	}
	if h := wg.health[flaky.restID()]; h <= 0 || h >= 0.5 {
		t.Fatalf("the failing model's health %v, want below 0.5", h)
	}
}

// A pool routed by score tells the score its order was made by: the row
// the trace keeps carries the same value the strategy weighed, and a pool
// routed any other way carries none.
func TestWeighedCarriesTheScoreItWasOrderedBy(t *testing.T) {
	fresh(t)
	const model = "m"
	cand := func(id string) candidate {
		return candidate{p: provider.Provider{ID: id, Name: id, Key: "k"}, model: model, rest: id}
	}
	flaky, steady := cand("a"), cand("b")
	noteUnstable(flaky)
	noteUnstable(flaky)

	scored := provider.Provider{ID: "pool", Routing: provider.Score}
	out, wg := weighRouted(scored, []candidate{flaky, steady}, model, provider.Chat, need{})
	if len(out) != 2 || out[0].rest != "b" {
		t.Fatalf("order %d candidates, want the steady b first", len(out))
	}
	rows := map[string]Weighed{}
	for _, c := range out {
		w := weighed(c, scored, wg, false, provider.Chat)
		if w.Score <= 0 || w.Score > 1 {
			t.Fatalf("%s's score %v, want a share", c.rest, w.Score)
		}
		if w.Score != wg.scores[c.restID()] {
			t.Fatalf("%s's row says %v, the weighing %v", c.rest, w.Score, wg.scores[c.restID()])
		}
		rows[c.rest] = w
	}
	if rows["b"].Score <= rows["a"].Score {
		t.Fatalf("the steady score %v, the failing one %v", rows["b"].Score, rows["a"].Score)
	}

	// a pool routed any other way weighs no score
	plain := provider.Provider{ID: "pool"}
	_, pw := weighRouted(plain, []candidate{flaky, steady}, model, provider.Chat, need{})
	if pw.scores != nil {
		t.Fatalf("a pool routed smart weighed scores: %v", pw.scores)
	}
	if w := weighed(steady, plain, pw, false, provider.Chat); w.Score != 0 {
		t.Fatalf("a pool routed smart says score %v", w.Score)
	}
}

// The relative line's demotion is not undone by one answer: a model whose
// score has climbed back over the line is kept behind until it has
// answered three times in a row.
func TestRelativeDemotionWaitsForThreeAnswers(t *testing.T) {
	fresh(t)
	if !settings.Load().StabilityGuardOn() {
		t.Fatal("the stability guard is off by default")
	}
	steady := candidate{p: provider.Provider{ID: "a", Name: "A", Key: "k1"}, model: "steady", rest: "a"}
	flaky := candidate{p: provider.Provider{ID: "b", Name: "B", Key: "k2"}, model: "flaky", rest: "b"}
	for i := 0; i < 40; i++ {
		noteStable(steady)
	}
	// 5/7: under a perfect peer's 75% line, the last word a failure
	for i := 0; i < 4; i++ {
		noteStable(flaky)
	}
	for i := 0; i < 2; i++ {
		noteUnstable(flaky)
	}
	// the flaky model comes first in the plan, so only a demotion moves it
	cs := []candidate{flaky, steady}
	pl := planned{order: []Weighed{{Model: "flaky"}, {Model: "steady"}}}

	// one answer lifts the score over the line (6/8), but not the demotion:
	// the model has answered once since it failed
	noteStable(flaky)
	out, _ := demoteUnstable(cs, pl)
	if got := healthOrder(out); got[0] != "steady" {
		t.Fatalf("one answer brought it back: %v", got)
	}
	noteStable(flaky)
	out, _ = demoteUnstable(cs, pl)
	if got := healthOrder(out); got[0] != "steady" {
		t.Fatalf("two answers brought it back: %v", got)
	}
	noteStable(flaky)
	out, _ = demoteUnstable(cs, pl)
	if got := healthOrder(out); got[0] != "flaky" {
		t.Fatalf("three answers did not bring it back: %v", got)
	}
}

// The absolute line is untouched by the wait: a model under it stays behind
// its steady peers however many times it has answered.
func TestAbsoluteDemotionKeepsItsPace(t *testing.T) {
	fresh(t)
	steady := candidate{p: provider.Provider{ID: "a", Name: "A", Key: "k1"}, model: "steady", rest: "a"}
	flaky := candidate{p: provider.Provider{ID: "b", Name: "B", Key: "k2"}, model: "flaky", rest: "b"}
	for i := 0; i < 40; i++ {
		noteStable(steady)
	}
	for i := 0; i < 8; i++ {
		noteUnstable(flaky)
	}
	cs := []candidate{flaky, steady}
	pl := planned{order: []Weighed{{Model: "flaky"}, {Model: "steady"}}}
	for i := 0; i < 3; i++ {
		noteStable(flaky)
	}
	out, _ := demoteUnstable(cs, pl)
	if got := healthOrder(out); got[0] != "steady" {
		t.Fatalf("three answers brought a model under the absolute line back: %v", got)
	}
}

// With the stability guard off the relative line alone decides, as before
// it existed: an answer that lifts the score over the line brings the model
// back at once.
func TestStabilityGuardOffRestoresTheRelativeLine(t *testing.T) {
	fresh(t)
	steady := candidate{p: provider.Provider{ID: "a", Name: "A", Key: "k1"}, model: "steady", rest: "a"}
	flaky := candidate{p: provider.Provider{ID: "b", Name: "B", Key: "k2"}, model: "flaky", rest: "b"}
	for i := 0; i < 40; i++ {
		noteStable(steady)
	}
	for i := 0; i < 4; i++ {
		noteStable(flaky)
	}
	for i := 0; i < 2; i++ {
		noteUnstable(flaky)
	}
	cs := []candidate{flaky, steady}
	pl := planned{order: []Weighed{{Model: "flaky"}, {Model: "steady"}}}
	// the answer that takes the score over the line: the guard still holds
	// it behind
	noteStable(flaky)
	out, _ := demoteUnstable(cs, pl)
	if got := healthOrder(out); got[0] != "steady" {
		t.Fatalf("fixture: the guard on did not hold it back: %v", got)
	}

	off := false
	st := settings.Load()
	st.StabilityGuard = &off
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	out, _ = demoteUnstable(cs, pl)
	if got := healthOrder(out); got[0] != "flaky" {
		t.Fatalf("with the guard off the line alone did not bring it back: %v", got)
	}
}

// A provider's row says how its models are answering: ProviderHealth
// finds the entries by the provider they rest by — a key's id#key, an
// account's id@user, its one key's bare id — and never by a prefix, so
// "ab" reads none of "a"'s records and "a" none of "ab"'s. The worst
// score is the lowest among the records; a provider nothing has asked
// reads 1, nothing recorded.
func TestProviderHealthByProvider(t *testing.T) {
	fresh(t)
	noteID("a#k/m1", false)
	noteID("a#k/m1", false)
	noteID("a#k/m1", false)
	noteID("a#k/m1", false) // 1/5: below the line
	noteID("a#k/m2", true)  // a record, and a good one
	noteID("a@u/m3", true)  // an account's record counts for its provider
	noteID("a/m5", true)    // the one key's bare id
	noteID("ab/m4", true)   // another provider's, a prefix away
	h := ProviderHealth([]string{"a"})
	if h.Recorded != 4 {
		t.Fatalf("recorded %d, want the four entries resting by a", h.Recorded)
	}
	if h.Below != 1 {
		t.Fatalf("below %d, want m1's alone", h.Below)
	}
	if h.WorstModel != "m1" {
		t.Fatalf("worst model %q, want m1", h.WorstModel)
	}
	if h.Worst >= healthDemote {
		t.Fatalf("worst %v, want m1's 1/5", h.Worst)
	}
	// "ab" is matched whole: its own ask carries only its own entry
	hb := ProviderHealth([]string{"ab"})
	if hb.Recorded != 1 || hb.WorstModel != "m4" {
		t.Fatalf("ab's health %+v, want its one entry alone", hb)
	}
	// nothing asked yet: the prior, 1, with nothing recorded
	quiet := ProviderHealth([]string{"quiet"})
	if quiet.Worst != 1 || quiet.Recorded != 0 || quiet.Below != 0 || quiet.WorstModel != "" {
		t.Fatalf("an unasked provider read %+v, want worst 1 and nothing recorded", quiet)
	}
}

// NoHealthDemotion leaves the order requests are tried in alone: what the
// records say of a provider reads the same with it on — the scores are
// what answering did, not how the order was set.
//
// A read is pure — the decay is worked out on a copy and the record is
// left as it was — so the two reads below differ only by the real decay
// of the elapsed time between them, a little toward the prior. The gap
// is injected rather than left to the machine: how long two reads really
// take depends on the runner. The allowed drift is derived from the
// elapsed time actually measured, with room for a little more, so a slow
// machine passes while a real change to the score (a demotion, a fresh
// note) fails.
func TestProviderHealthReadsTheSameWithNoDemotion(t *testing.T) {
	fresh(t)
	noteID("a#k/m1", false)
	noteID("a#k/m1", false)
	noteID("a#k/m1", false)
	noteID("a#k/m1", false)
	noteID("a#k/m2", true)
	st := settings.Load()
	was := st.NoHealthDemotion
	t.Cleanup(func() {
		st := settings.Load()
		st.NoHealthDemotion = was
		settings.Save(st)
	})
	first := time.Now()
	h := ProviderHealth([]string{"a"})
	// Force a gap larger than the old 1e-6 gate could tolerate: with m1's
	// loss of 4 the score rises ~6e-5 per second, so 100ms is ~6e-6 of drift
	// on even the fastest machine. The old assertion went red at ~17ms of
	// wall time, i.e. on any moderately loaded runner, hiding real changes.
	time.Sleep(100 * time.Millisecond)
	st.NoHealthDemotion = true
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	after := ProviderHealth([]string{"a"})
	last := time.Now()
	// the two reads are a gap apart, so the records decayed a little
	// between them; the counts and the named model are exact
	if after.Recorded != h.Recorded || after.Below != h.Below || after.WorstModel != h.WorstModel {
		t.Fatalf("NoHealthDemotion changed the reading: %+v → %+v", h, after)
	}
	// m1 is the worst: wins 0 and the four failures above, so
	// Worst = prior/(loss + prior) = 1/(loss+1). The slowest legitimate
	// reading is one where all the elapsed time went into decaying m1's
	// loss from 4; that bounds the drift regardless of runner load. A fixed
	// 1e-6 let wall-clock jitter decide the test, going red at ~17ms.
	w := math.Exp2(-last.Sub(first).Seconds() / healthHalfLife.Seconds())
	maxWorst := 1 / (4*w + 1)
	tolerance := (maxWorst - 1.0/5) + 1e-6
	if d := math.Abs(after.Worst - h.Worst); d > tolerance {
		t.Fatalf("worst %v → %v drifted %v over a %v gap, more than the decay bound %v",
			h.Worst, after.Worst, d, last.Sub(first), tolerance)
	}
}

// ProviderHealth reads the table under the one lock its writers take: a
// note made while it reads coexists with it, and no entry goes missing.
func TestProviderHealthReadsWhileNotesAreMade(t *testing.T) {
	fresh(t)
	noteID("a/m1", true)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			noteID("a/m2", i%2 == 0)
		}
	}()
	for i := 0; i < 100; i++ {
		if h := ProviderHealth([]string{"a"}); h.Recorded < 1 {
			t.Fatalf("a record went missing while notes were being made: %+v", h)
		}
	}
	<-done
}

// The row's health is said with no model's name when there is none to
// say: the key itself is left out rather than an empty word drawn.
func TestProviderHealthJSONOmitsWhatIsNotThere(t *testing.T) {
	b, err := json.Marshal(Health{Worst: 1})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"worst":1,"below":0,"recorded":0}` {
		t.Fatalf("no model named: %s", b)
	}
}

// A table over the limit with every entry fresh is not swept on every
// note: an active table never had an entry to forget, so the sweep is
// throttled to once a minute rather than paid whole (O(n)) on every one
// of the ten thousand observations a busy table takes. The count of
// sweeps is read from the table's own state (modelHealth.swept), so no
// clock is involved, and it is the same at twice the table's size.
func TestSweepOfAnActiveOverLimitTableIsThrottledNotPerNote(t *testing.T) {
	fresh(t)
	fill := func(n int) {
		modelHealth.Lock()
		modelHealth.m = map[string]healthPoint{}
		for i := 0; i < n; i++ {
			modelHealth.m[fmt.Sprintf("p%d/m", i)] = healthPoint{wins: 1, at: time.Now()}
		}
		modelHealth.Unlock()
	}
	observe := func(n int) int {
		before := modelHealth.swept
		for i := 0; i < n; i++ {
			noteID("p0/m", true)
		}
		return modelHealth.swept - before
	}
	// a table just over the limit: one sweep (the first note), then none
	// for the rest of the run's ten thousand notes
	fill(4097)
	if got := observe(10000); got != 1 {
		t.Fatalf("%d sweeps for 10000 notes on a 4097-entry table, want the one throttled in a minute", got)
	}
	if len(modelHealth.m) != 4097 {
		t.Fatalf("an active table lost entries: %d of 4097", len(modelHealth.m))
	}
	// twice the entries, the same count: the sweep does not grow with the
	// table
	modelHealth.Lock()
	modelHealth.lastSweep = time.Time{}
	modelHealth.Unlock()
	fill(8192)
	if got := observe(10000); got != 1 {
		t.Fatalf("%d sweeps for 10000 notes on a 8192-entry table, want the same one as the 4097-entry table", got)
	}
	if len(modelHealth.m) != 8192 {
		t.Fatalf("an active table lost entries: %d of 8192", len(modelHealth.m))
	}
}

// A read of the table changes nothing in it: healthOf, ProviderHealth and
// markOf all worked their decay out on a copy, so two reads a moment apart
// of a record an hour old say the same — and the record is left exactly
// as it was, not rewritten with the decayed counts as a read's side
// effect (the lazy refresh the tests below once had to make room for).
func TestAReadLeavesTheHealthTableAlone(t *testing.T) {
	fresh(t)
	hourAgo := time.Now().Add(-time.Hour)
	modelHealth.Lock()
	modelHealth.m = map[string]healthPoint{
		"a/good":   {wins: 9, loss: 1, at: hourAgo},
		"b/flaky":  {wins: 1, loss: 4, at: hourAgo},
		"ab/other": {wins: 2, at: hourAgo},
	}
	modelHealth.Unlock()
	was := map[string]healthPoint{}
	modelHealth.Lock()
	for k, v := range modelHealth.m {
		was[k] = v
	}
	modelHealth.Unlock()
	now := time.Now()
	healthOf("b/flaky", now)
	ProviderHealth([]string{"a", "b"})
	markOf(healthCandidates(), now)
	healthSeats(healthCandidates(), now)
	modelHealth.Lock()
	unchanged := reflect.DeepEqual(modelHealth.m, was)
	modelHealth.Unlock()
	if !unchanged {
		t.Fatalf("a read rewrote the table: %+v, want it left as %+v", modelHealth.m, was)
	}
	// and two reads a moment apart say the same: no drift toward the
	// prior from a record being refreshed as it is read
	if a, b := healthOf("b/flaky", now), healthOf("b/flaky", now); a != b {
		t.Fatalf("two reads of the same moment said %v and %v", a, b)
	}
}

// The one-lock snapshot the plan's rows read their health from says what
// the per-candidate read would: for every candidate with a record, at
// the same moment, exactly healthOf's answer; a candidate with no record
// is absent from it, as healthOf's prior reads it as 1 and the row stays
// quiet about a model nothing has asked.
func TestHealthSeatsSayWhatHealthOfDoes(t *testing.T) {
	fresh(t)
	cs := []candidate{
		{p: provider.Provider{ID: "a", Key: "k"}, model: "asked", rest: "a"},
		{p: provider.Provider{ID: "b", Key: "k"}, model: "asked-too", rest: "b"},
		{p: provider.Provider{ID: "c", Key: "k"}, model: "never-asked", rest: "c"},
	}
	now := time.Now()
	modelHealth.Lock()
	modelHealth.m = map[string]healthPoint{
		"a/asked":     {wins: 9, loss: 1, at: now.Add(-time.Hour)},
		"b/asked-too": {wins: 1, loss: 4, at: now.Add(-time.Minute)},
	}
	modelHealth.Unlock()
	seats := healthSeats(cs, now)
	for _, c := range cs[:2] {
		want := healthOf(c.restID(), now)
		if got, ok := seats[c.restID()]; !ok || got != want {
			t.Fatalf("%s's seat said %v (found %v), want healthOf's %v", c.restID(), got, ok, want)
		}
	}
	if _, ok := seats[cs[2].restID()]; ok {
		t.Fatalf("a model with no record took a seat: %v", seats[cs[2].restID()])
	}
	if healthOf(cs[2].restID(), now) != 1 {
		t.Fatal("a model with no record does not read as the prior, 1")
	}
}
