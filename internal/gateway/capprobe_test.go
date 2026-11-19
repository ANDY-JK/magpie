package gateway

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// The gates a probe's turn is due by: a model never asked, letters gone
// stale, and a failure past its backoff on another day than its last. A
// fresh record, a failure not yet due, and one due the same day it failed
// wait.
func TestCapProbeDueByItsGates(t *testing.T) {
	now := time.Now()
	if !capProbeDueNow(capProbeRecord{}, false, now) {
		t.Fatal("never asked: due")
	}
	if capProbeDueNow(capProbeRecord{At: now.Add(-time.Hour), OK: true}, true, now) {
		t.Fatal("letters a day old: not due")
	}
	if !capProbeDueNow(capProbeRecord{At: now.Add(-capProbeStale - time.Minute), OK: true}, true, now) {
		t.Fatal("letters gone stale: due")
	}
	r := capProbeRecord{At: now.Add(-time.Hour), Fails: 1, Next: now.Add(time.Minute)}
	if capProbeDueNow(r, true, now) {
		t.Fatal("a failure before its backoff: not due")
	}
	r.Next = now.Add(-time.Minute)
	if capProbeDueNow(r, true, now) {
		t.Fatal("a failure due the day it failed: not due")
	}
	r.At, r.Next = now.Add(-48*time.Hour), now.Add(-time.Minute)
	if !capProbeDueNow(r, true, now) {
		t.Fatal("a failure past its backoff, another day: due")
	}
}

// A failure is asked again at an hour, then six, then a day.
func TestCapProbeBackoffGrows(t *testing.T) {
	for fails, want := range map[int]time.Duration{0: time.Hour, 1: time.Hour, 2: 6 * time.Hour, 3: 24 * time.Hour, 7: 24 * time.Hour} {
		if got := capProbeBackoff(fails); got != want {
			t.Errorf("%d fails: %s, want %s", fails, got, want)
		}
	}
}

// What a probe settled spells as the pickers' letters do, the tags it said
// only, and is kept as the provider keeps its letters.
func TestCapProbeSaidAndKept(t *testing.T) {
	if got := capSaid(provider.ModelCaps{Tools: "yes", Vision: "no", Stream: ""}); got != "T✓ V✗" {
		t.Errorf("capSaid: %q, want %q", got, "T✓ V✗")
	}
	caps := capCapsOf(map[string]string{"tools": "yes", "vision": "no"})
	if caps.Tools != "yes" || caps.Vision != "no" || caps.Stream != "" {
		t.Errorf("capCapsOf: %+v", caps)
	}
}

// An offer takes its place only when a probe loop is up, the switch is on
// and no offer for the model waits already; a full queue takes none.
func TestCapProbeOfferTakesItsPlace(t *testing.T) {
	resetCapProbes()
	t.Cleanup(func() { capProbeAsk = capProbeOne; resetCapProbes() })
	if capProbeOffer("p", "m"); len(capProbes.wait) != 0 {
		t.Fatal("no probe loop is up: nothing offered")
	}
	capProbes.looping.Store(true)
	// the switch off takes nothing; on again, one offer whatever the plan
	// saw it how often, and a full queue takes none more
	setAutoCapProbe(t, &[]bool{false}[0])
	capProbeOffer("p", "m")
	if len(capProbes.wait) != 0 {
		t.Fatal("the switch off: nothing offered")
	}
	setAutoCapProbe(t, nil)
	capProbeOffer("p", "m")
	capProbeOffer("p", "m")
	if len(capProbes.wait) != 1 {
		t.Fatalf("one offer, whatever the plan sees it how often: %d", len(capProbes.wait))
	}
	for i := 0; i < capProbeQueueMost; i++ {
		capProbeOffer("p", "m"+time.Now().Format("150405.000000000")+string(rune('a'+i%26)))
	}
	if n := len(capProbes.wait); n > capProbeQueueMost {
		t.Fatalf("the queue holds %d, its most is %d", n, capProbeQueueMost)
	}
}

// setAutoCapProbe saves the switch as v says, nil the default on, and puts
// the default back when the test is done.
func setAutoCapProbe(t *testing.T, v *bool) {
	t.Helper()
	s := settings.Load()
	s.AutoCapProbe = v
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s := settings.Load()
		s.AutoCapProbe = nil
		if err := settings.Save(s); err != nil {
			t.Fatal(err)
		}
	})
}

// A turn asks what its gates open: two at once, the rest waiting; a failure
// keeps its record with its backoff; what settled is kept with the
// provider.
func TestCapProbesDueAsksTwo(t *testing.T) {
	resetCapProbes()
	t.Cleanup(func() { capProbeAsk = capProbeOne; resetCapProbes() })
	capProbes.looping.Store(true)
	var held sync.WaitGroup
	held.Add(2)
	release := make(chan struct{})
	asked := make(chan capProbeKey, 8)
	capProbeAsk = func(ctx context.Context, k capProbeKey) {
		asked <- k
		held.Done()
		<-release
		capProbesDone(k, k.model == "able", capSaid(provider.ModelCaps{Tools: "yes"}))
	}
	setAutoCapProbe(t, nil)
	capProbeOffer("p", "able")
	capProbeOffer("p", "unable")
	capProbeOffer("p", "third")
	var asks sync.WaitGroup
	next := capProbesDue(context.Background(), &asks, time.Now())
	held.Wait()
	capProbes.Lock()
	waiting := slices.Clone(capProbes.wait)
	asking := capProbes.asking
	capProbes.Unlock()
	if asking != 2 || len(waiting) != 1 || waiting[0].model != "third" || !next.IsZero() {
		t.Fatalf("two asked, one waiting for their slots, the next turn their wake: asking %d, waiting %v, next %v", asking, waiting, next)
	}
	close(release)
	asks.Wait()
	// what settled is kept in the record; a failure carries its words and
	// its backoff
	r, ok := capProbeRecordOf("p", "able")
	if !ok || !r.OK || r.Said != "T✓" {
		t.Fatalf("the able one's record: %+v, %v", r, ok)
	}
	r, ok = capProbeRecordOf("p", "unable")
	if !ok || r.OK || r.Fails != 1 || time.Until(r.Next) <= 55*time.Minute {
		t.Fatalf("the unable one's record: %+v, %v", r, ok)
	}
	if _, in := capProbes.queued[capProbeKey{"p", "able"}]; in {
		t.Fatal("the offer was let go of once taken")
	}
	// the third waits in the queue for its slot, and takes it at the next
	// turn
	release2 := make(chan struct{})
	held2 := make(chan struct{}, 1)
	capProbeAsk = func(ctx context.Context, k capProbeKey) {
		held2 <- struct{}{}
		<-release2
		capProbesDone(k, true, "T✓")
	}
	next = capProbesDue(context.Background(), &asks, time.Now())
	if !next.IsZero() {
		t.Fatalf("the third's turn takes the slot: next %v", next)
	}
	<-held2
	close(release2)
	asks.Wait()
	if r, ok := capProbeRecordOf("p", "third"); !ok || !r.OK {
		t.Fatalf("the third one's record: %+v, %v", r, ok)
	}
}

// The ten minutes' room and the day's budget hold the queue where it is,
// the day's said once in the log.
func TestCapProbesDueWaitsForItsWindows(t *testing.T) {
	resetCapProbes()
	t.Cleanup(func() { capProbeAsk = capProbeOne; resetCapProbes() })
	capProbes.looping.Store(true)
	setAutoCapProbe(t, nil)
	now := time.Now()
	capProbeOffer("p", "m")
	asks := &sync.WaitGroup{}
	// the ten minutes spent: nothing asked, the offer kept, the next turn
	// when the oldest of the window's probes turns ten minutes
	capProbes.Lock()
	for i := 0; i < capProbeTenMin; i++ {
		capProbes.tenMin = append(capProbes.tenMin, now.Add(-time.Minute))
	}
	capProbes.Unlock()
	next := capProbesDue(context.Background(), asks, now)
	if next.IsZero() || time.Until(next) > 9*time.Minute+30*time.Second {
		t.Fatalf("the ten minutes' next turn: %v", next)
	}
	if r, ok := capProbeRecordOf("p", "m"); ok {
		t.Fatalf("nothing asked: %v", r)
	}
	// the day's budget spent: the offer waits for tomorrow, said once
	capProbes.Lock()
	capProbes.tenMin = nil
	capProbes.dayN = capProbeADay
	capProbes.Unlock()
	next = capProbesDue(context.Background(), asks, now)
	if next.IsZero() || next.Format("2006-01-02") == now.Format("2006-01-02") {
		t.Fatalf("the day's next turn is tomorrow: %v", next)
	}
	if r, ok := capProbeRecordOf("p", "m"); ok {
		t.Fatalf("nothing asked past the budget: %v", r)
	}
	// and the switch off asks nothing at all
	setAutoCapProbe(t, &[]bool{false}[0])
	capProbes.Lock()
	capProbes.dayN = 0
	capProbes.Unlock()
	if next := capProbesDue(context.Background(), asks, now); !next.IsZero() {
		t.Fatalf("the switch off: next %v", next)
	}
	if r, ok := capProbeRecordOf("p", "m"); ok {
		t.Fatalf("the switch off asks nothing: %v", r)
	}
}

// The records a magpie before this one kept go on: the seven days and a
// failure's backoff read across a restart.
func TestCapProbesOutliveRestart(t *testing.T) {
	resetCapProbes()
	t.Cleanup(func() { capProbeAsk = capProbeOne; resetCapProbes() })
	now := time.Now()
	capProbes.Lock()
	capProbes.day, capProbes.dayN, capProbes.dirty = now.Format("2006-01-02"), 3, true
	capProbes.Unlock()
	capProbesDone(capProbeKey{"p", "able"}, true, "T✓")
	path := capProbePath()
	capProbesLoaded.Lock()
	capProbesLoaded.from = map[string]bool{}
	capProbesLoaded.Unlock()
	loadCapProbes()
	if _, ok := capProbeRecordOf("p", "able"); !ok {
		t.Fatalf("the record is kept in %s", filepath.Base(path))
	}
	capProbes.Lock()
	n := capProbes.dayN
	capProbes.Unlock()
	if n != 3 {
		t.Fatalf("the day's count is kept too: %d", n)
	}
}

// A probe that fails marks nothing but its own record: no note to the
// health a route reads, no letters kept with the provider, and the next
// ask waits the backoff out.
func TestCapProbeFailureLeavesNoMark(t *testing.T) {
	resetCapProbes()
	resetHealth()
	t.Cleanup(func() { capProbeAsk = capProbeOne; resetCapProbes(); resetHealth() })
	capProbeAsk = func(ctx context.Context, k capProbeKey) {
		capProbesDone(k, false, "no answer")
	}
	p := provider.Provider{ID: "capprobe-fail", Name: "Fail", Key: "k", Chat: "http://127.0.0.1:9", Models: []string{"m"}}
	if err := provider.Save(p); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { provider.Delete("capprobe-fail") })
	capProbes.looping.Store(true)
	capProbeOffer("capprobe-fail", "m")
	var asks sync.WaitGroup
	capProbesDue(context.Background(), &asks, time.Now())
	asks.Wait()
	r, ok := capProbeRecordOf("capprobe-fail", "m")
	if !ok || r.OK || r.Fails != 1 || r.Next.IsZero() {
		t.Fatalf("the failure's record: %+v, %v", r, ok)
	}
	modelHealth.Lock()
	n := len(modelHealth.m)
	modelHealth.Unlock()
	if n != 0 {
		t.Fatalf("a failed probe noted health: %d entries", n)
	}
	saved, err := provider.Find("capprobe-fail")
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Caps) != 0 {
		t.Fatalf("a failed probe kept letters: %+v", saved.Caps)
	}
}

// capProbeAsk is the ask itself, as the plan's offer becomes: what a Test
// capabilities run asks, kept with the provider (SetCaps) — a call it
// makes, a picture it reads and a stream it takes, and the record and log
// line each way. The endpoint here is served from bytes, never the
// network.
func TestCapProbeOneAsksAndKeeps(t *testing.T) {
	resetCapProbes()
	t.Cleanup(resetCapProbes)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		switch {
		case bytes.Contains(b, []byte(`"stream":true`)):
			// the stream probe: events, then the end
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("data: {\"delta\":\"hi\"}\n\ndata: [DONE]\n\n"))
		case bytes.Contains(b, []byte("image_url")):
			// the vision probe: answered
			w.Write([]byte("blue"))
		default:
			// the test itself, and the tools probe: a call made
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"magpie_probe","arguments":"{}"}}]}}]}`))
		}
	}))
	t.Cleanup(up.Close)
	p := provider.Provider{ID: "capprobe", Name: "Cap", Key: "k", Chat: up.URL, Models: []string{"m"}}
	if err := provider.Save(p); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { provider.Delete("capprobe") })
	capProbeOne(context.Background(), capProbeKey{"capprobe", "m"})
	r, ok := capProbeRecordOf("capprobe", "m")
	if !ok || !r.OK || r.Said != "T✓ V✓ S✓" {
		t.Fatalf("what the ask settled: %+v, %v", r, ok)
	}
	saved, err := provider.Find("capprobe")
	if err != nil {
		t.Fatal(err)
	}
	if caps := saved.Caps["m"]; caps.Tools != "yes" || caps.Vision != "yes" || caps.Stream != "yes" {
		t.Fatalf("what the provider keeps: %+v", caps)
	}
}

// A model no longer there to ask — its provider deleted or turned off
// while it waited — gives the slot it took back: the turns go on, and a
// slot never given back would stop them for good once two had piled up.
func TestCapProbeDroppedGivesTheSlotBack(t *testing.T) {
	resetCapProbes()
	t.Cleanup(resetCapProbes)
	off := provider.Provider{ID: "capprobe-off", Name: "Off", Key: "k", Chat: "http://127.0.0.1:9", Models: []string{"m"}, Off: true}
	if err := provider.Save(off); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { provider.Delete("capprobe-off") })
	capProbes.looping.Store(true)
	setAutoCapProbe(t, nil)
	// one offered whose provider is gone, one whose provider is off: both
	// are let go of without a record, each slot back
	capProbeOffer("capprobe-gone", "m")
	capProbeOffer("capprobe-off", "m")
	var asks sync.WaitGroup
	if next := capProbesDue(context.Background(), &asks, time.Now()); !next.IsZero() {
		t.Fatalf("next %v, want their wake to say nothing", next)
	}
	asks.Wait()
	capProbes.Lock()
	asking := capProbes.asking
	capProbes.Unlock()
	if asking != 0 {
		t.Fatalf("%d slot(s) held after both drops, want none back late", asking)
	}
	if _, ok := capProbeRecordOf("capprobe-gone", "m"); ok {
		t.Fatal("a drop left a record")
	}
	if _, ok := capProbeRecordOf("capprobe-off", "m"); ok {
		t.Fatal("the off one left a record")
	}
	// the slots are back: a turn asks the two it can, whatever the drops
	// took with them
	held := make(chan struct{}, 2)
	old := capProbeAsk
	capProbeAsk = func(ctx context.Context, k capProbeKey) {
		held <- struct{}{}
		capProbesDone(k, true, "T✓")
	}
	t.Cleanup(func() { capProbeAsk = old })
	capProbeOffer("capprobe-gone", "m1")
	capProbeOffer("capprobe-gone", "m2")
	capProbeOffer("capprobe-gone", "m3")
	capProbesDue(context.Background(), &asks, time.Now())
	asks.Wait()
	if n := len(held); n != 2 {
		t.Fatalf("%d probe(s) asked of two free slots, want both taken", n)
	}
}
