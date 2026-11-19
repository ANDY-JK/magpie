package gateway

// The capability letters a pool routed by score found missing, asked for in
// the background: a candidate its provider has no letters for
// (Provider.Caps), or letters older than the probes re-ask for, is offered
// by the plan (weighRouted), and this asks it — the same requests a Test
// capabilities run sends, with the rules it keeps (only an endpoint a test
// reaches directly, translated and sign-in ones skipped, a tiny request,
// its timeout) — and keeps what settles with provider.SetCaps, where the
// pickers' rows and the plan's ability dimension read it.
//
// Nothing here waits on a request: offering takes only a place in the
// queue, the asking runs in this loop, and a probe that fails touches
// neither a route nor a health score (the provider's test path notes
// neither). The guardrails are the probe's own: a model at most once a
// day, and seven days once its letters settled; two asked at once; six
// starting in ten minutes; two hundred in a day — past it the queue waits
// for tomorrow, said once in the log; a failure asked again at an hour,
// then six, then a day. settings.AutoCapProbe turns it all off.
//
// Only the magpie serving the gateway runs the loop (WhileServing), and
// only a loop that runs takes offers — a test's Server never reaches a
// vendor for this.

import (
	"cmp"
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/steady"
)

const (
	// capProbeStale is how old settled letters may get before the model is
	// asked again: seven days.
	capProbeStale = 7 * 24 * time.Hour
	// capProbeAtOnce is how many probes may be under way at the same time.
	capProbeAtOnce = 2
	// capProbeTenMin is how many may start within ten minutes of one
	// another.
	capProbeTenMin = 6
	// capProbeADay is how many may start in a day; past it the queue waits
	// for the next one.
	capProbeADay = 200
	// capProbeQueueMost is the most offered at once: past it an offer is
	// dropped, and the next plan that lacks the model offers it again.
	capProbeQueueMost = 1024
	// capProbeWait is how long one model's letters are waited for: the
	// test and its three capability probes, each within testWait.
	capProbeWait = 2 * time.Minute
)

// capProbeBackoff is how long after the n-th failed attempt (the first an
// hour, then six of them, then a day) the next may go.
func capProbeBackoff(fails int) time.Duration {
	switch {
	case fails <= 1:
		return time.Hour
	case fails == 2:
		return 6 * time.Hour
	}
	return 24 * time.Hour
}

// capProbeKey is the model a probe asks: its provider and its id there.
type capProbeKey struct{ id, model string }

// capProbeRecord is what one model's last probe made, kept in caps.json
// beside providers.json so the seven days and the backoff survive a
// restart: when it was asked, what came of it, and — a failure — how many
// it has failed in a row and when the next may go.
type capProbeRecord struct {
	At    time.Time `json:"at,omitzero"`
	OK    bool      `json:"ok"`
	Said  string    `json:"said,omitempty"`
	Fails int       `json:"fails,omitempty"`
	Next  time.Time `json:"next,omitzero"`
}

// savedCapProbes is caps.json's shape: the records, and the day's probe
// count, so a restart doesn't hand the day a fresh budget.
type savedCapProbes struct {
	Records map[string]capProbeRecord `json:"records,omitempty"`
	Day     string                    `json:"day,omitempty"`
	DayN    int                       `json:"dayN,omitempty"`
}

// capProbes is the offers waiting, what the probes made of the models
// asked, and the budget's counts. The queue holds the offered models in
// the order they were offered; a turn takes the first one whose gates are
// open. looping says a probe loop is up: it is the only taker of offers.
var capProbes = struct {
	sync.Mutex
	queued  map[capProbeKey]bool // offered, waiting or being asked
	wait    []capProbeKey
	recs    map[capProbeKey]capProbeRecord
	asking  int         // probes under way
	tenMin  []time.Time // when each of the window's probes started
	day     string
	dayN    int
	daySaid bool // the day's budget said spent once
	dirty   bool // what this process made and hasn't written
	looping atomic.Bool
}{queued: map[capProbeKey]bool{}, recs: map[capProbeKey]capProbeRecord{}}

// capProbePath is the file the records are kept in.
func capProbePath() string { return filepath.Join(filepath.Dir(provider.Path()), "caps.json") }

// capProbesLoaded: the paths whose file this process read already, read
// once each (scoresLoaded's way).
var capProbesLoaded = struct {
	sync.Mutex
	from map[string]bool
}{from: map[string]bool{}}

// loadCapProbes reads what the probes of a magpie before this one made,
// once for each file, at New. A file that can't be read or parsed adds
// nothing: the probes begin again, and the next attempt writes it anew.
func loadCapProbes() {
	path := capProbePath()
	capProbesLoaded.Lock()
	defer capProbesLoaded.Unlock()
	if capProbesLoaded.from[path] {
		return
	}
	capProbesLoaded.from[path] = true
	b, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) { // a first run has no file: nothing said
			log.Printf("caps.json: unreadable, the capability probes begin again: %v", err)
		}
		return
	}
	var saved savedCapProbes
	if err := json.Unmarshal(b, &saved); err != nil {
		log.Printf("caps.json: unparsable, the capability probes begin again: %v", err)
		return
	}
	capProbes.Lock()
	defer capProbes.Unlock()
	for key, r := range saved.Records {
		id, model, ok := strings.Cut(key, "\x00")
		if !ok || id == "" || model == "" {
			continue
		}
		if k := (capProbeKey{id, model}); !capProbes.queued[k] {
			if _, had := capProbes.recs[k]; !had {
				capProbes.recs[k] = r
			}
		}
	}
	capProbes.day, capProbes.dayN = saved.Day, saved.DayN
}

// saveCapProbes writes the records and the day's count, what this process
// made and hasn't written yet. Called on the probe loop's own goroutine,
// never on a request's path.
func saveCapProbes() {
	capProbes.Lock()
	if !capProbes.dirty {
		capProbes.Unlock()
		return
	}
	out := savedCapProbes{Records: map[string]capProbeRecord{}, Day: capProbes.day, DayN: capProbes.dayN}
	for k, r := range capProbes.recs {
		out.Records[k.id+"\x00"+k.model] = r
	}
	capProbes.dirty = false
	capProbes.Unlock()
	b, err := json.Marshal(out)
	if err != nil {
		return
	}
	path := capProbePath()
	if os.MkdirAll(filepath.Dir(path), 0o700) != nil {
		return
	}
	target, err := edit.Target(path) // a symlink stays, its target written
	if err != nil {
		return
	}
	tmp := target + ".magpie-tmp"
	if os.WriteFile(tmp, b, 0o600) != nil {
		return
	}
	if steady.Rename(tmp, target) != nil {
		os.Remove(tmp)
		return
	}
	// a record made while the copy above was being taken set dirty again:
	// it waits for the next save, as a flush's mark does (score_store.go)
	capProbes.Lock()
	capProbes.dirty = false
	capProbes.Unlock()
}

// capProbeRecordOf is the last probe's record for model of id, and whether
// there is one.
func capProbeRecordOf(id, model string) (capProbeRecord, bool) {
	capProbes.Lock()
	defer capProbes.Unlock()
	r, ok := capProbes.recs[capProbeKey{id, model}]
	return r, ok
}

// capProbeDueNow says the model may be asked now, given what its last
// probe made: none yet, letters gone stale, or a failure past its backoff
// and not this day's.
func capProbeDueNow(r capProbeRecord, has bool, now time.Time) bool {
	if !has {
		return true
	}
	if r.OK {
		return now.Sub(r.At) >= capProbeStale
	}
	return !now.Before(r.Next) && !capProbeSameDay(r.At, now)
}

// capProbeSameDay says at falls on the day now does: a model is asked at
// most once a day.
func capProbeSameDay(at, now time.Time) bool {
	return at.Format("2006-01-02") == now.Format("2006-01-02")
}

// needsProbing says a pool routed by score saw model of p without the
// letters it would weigh: none these probes settled yet, or settled longer
// ago than they re-ask for (capProbeStale). The candidate is held out or
// scored half for it — the probe asks what the plan missed.
func needsProbing(p provider.Provider, model string, now time.Time) bool {
	r, ok := capProbeRecordOf(p.ID, model)
	return capProbeDueNow(r, ok, now)
}

// capProbeOffer offers model of id to the background probes. Only a place
// in the queue is taken — one for the same model waits already, a full
// queue takes none, and a process with no probe loop or AutoCapProbe off
// takes none at all. The next plan that lacks the model offers it again.
func capProbeOffer(id, model string) {
	if !capProbes.looping.Load() || !settings.Shared().AutoCapProbeOn() {
		return
	}
	k := capProbeKey{id, model}
	capProbes.Lock()
	defer capProbes.Unlock()
	if capProbes.queued[k] || len(capProbes.wait) >= capProbeQueueMost {
		return
	}
	capProbes.queued[k] = true
	capProbes.wait = append(capProbes.wait, k)
	wakeCapProbe()
}

// resetCapProbes empties the queue, the records and the budget's counts: a
// test starts as a fresh process does, the package's tests sharing provider
// ids otherwise (resetScores' way).
func resetCapProbes() {
	capProbes.Lock()
	capProbes.queued = map[capProbeKey]bool{}
	capProbes.wait = nil
	capProbes.recs = map[capProbeKey]capProbeRecord{}
	capProbes.asking = 0
	capProbes.tenMin = nil
	capProbes.day, capProbes.dayN, capProbes.daySaid, capProbes.dirty = "", 0, false, false
	capProbes.Unlock()
	capProbes.looping.Store(false)
}

// capProbesDue takes the offers whose turn has come and asks each on its
// own goroutine, and says when the next turn is: a slot taken by a probe
// under way, the ten minutes' room, the day's budget, the model's own
// gates (capProbeDueNow). An offer whose gates say not now is let go of —
// the next plan that lacks the model offers it again; one whose ten
// minutes or day are spent waits where it is.
func capProbesDue(ctx context.Context, asks *sync.WaitGroup, now time.Time) (next time.Time) {
	capProbes.Lock()
	defer capProbes.Unlock()
	if !settings.Shared().AutoCapProbeOn() {
		return next
	}
	if day := now.Format("2006-01-02"); capProbes.day != day {
		capProbes.day, capProbes.dayN, capProbes.daySaid = day, 0, false
		capProbes.dirty = true
	}
	capProbes.tenMin = slices.DeleteFunc(capProbes.tenMin, func(t time.Time) bool { return now.Sub(t) >= 10*time.Minute })
	for i := 0; i < len(capProbes.wait) && capProbes.asking < capProbeAtOnce; {
		k := capProbes.wait[i]
		if r, has := capProbes.recs[k]; !capProbeDueNow(r, has, now) {
			// not its turn: let it go, the next plan that lacks the model
			// offers it again
			capProbes.wait = slices.Delete(capProbes.wait, i, i+1)
			delete(capProbes.queued, k)
			continue
		}
		if len(capProbes.tenMin) >= capProbeTenMin {
			if at := capProbes.tenMin[0].Add(10 * time.Minute); next.IsZero() || at.Before(next) {
				next = at
			}
			break
		}
		if capProbes.dayN >= capProbeADay {
			if !capProbes.daySaid {
				log.Printf("cap probe: %d asked today, the day's budget is spent; %d model(s) wait for tomorrow", capProbeADay, len(capProbes.wait))
				capProbes.daySaid = true
			}
			if at := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(24 * time.Hour); next.IsZero() || at.Before(next) {
				next = at
			}
			break
		}
		capProbes.wait = slices.Delete(capProbes.wait, i, i+1)
		capProbes.asking++
		capProbes.tenMin = append(capProbes.tenMin, now)
		capProbes.dayN++
		capProbes.dirty = true
		asks.Add(1)
		go func(k capProbeKey) {
			defer asks.Done()
			capProbeAsk(ctx, k)
		}(k)
	}
	return next
}

// capProbeAsk asks one model what it can do (capProbeOne), swapped in
// tests.
var capProbeAsk = capProbeOne

var capProbeWake = make(chan struct{}, 1)

func wakeCapProbe() {
	select {
	case capProbeWake <- struct{}{}:
	default:
	}
}

// keepProbingCaps asks the offered models as their turns come, until ctx
// ends, and keeps what was made. Only the magpie serving the gateway runs
// it (WhileServing), and only while it runs are offers taken.
func keepProbingCaps(ctx context.Context) {
	capProbes.looping.Store(true)
	defer capProbes.looping.Store(false)
	var asks sync.WaitGroup
	defer func() {
		asks.Wait()
		saveCapProbes()
	}()
	t := time.NewTimer(time.Hour)
	defer t.Stop()
	for {
		next := capProbesDue(ctx, &asks, time.Now())
		wait := time.Hour
		if !next.IsZero() {
			wait = max(time.Until(next), 0)
		}
		t.Reset(wait)
		select {
		case <-ctx.Done():
			return
		case <-capProbeWake:
		case <-t.C:
		}
	}
}

func init() {
	WhileServing = append(WhileServing, keepProbingCaps)
}

// capProbeOne asks one model what it can do, as a Test capabilities run
// does (TestModelsWith), and keeps what settles. A probe that fails is no
// route's and no health score's: the provider's test path notes neither.
func capProbeOne(ctx context.Context, k capProbeKey) {
	p, err := provider.Find(k.id)
	switch {
	case err != nil:
		// the provider is gone since it was offered: nothing asked,
		// nothing kept
		log.Printf("cap probe: %s/%s not asked: %v", k.id, k.model, err)
		deleteQueued(k)
		return
	case p.Off:
		// turned off since: on again, the next plan that lacks the model
		// offers it
		log.Printf("cap probe: %s/%s not asked: %s is off", k.id, k.model, k.id)
		deleteQueued(k)
		return
	}
	c, done := context.WithTimeout(ctx, capProbeWait)
	rs := p.TestModelsWith(c, []string{k.model}, true)
	done()
	r := rs[0]
	ok, said := false, ""
	switch {
	case ctx.Err() != nil:
		said = "magpie stopped asking"
	case !r.OK:
		said = cmp.Or(r.Error, "no answer")
	case r.Caps == nil:
		// the endpoint answered and settled no letter (an OpenCode free
		// tier's tools, Gemini's stream): nothing to keep
		said = "it answered, and settled no letter"
	default:
		caps := capCapsOf(r.Caps)
		err = provider.SetCaps(k.id, map[string]provider.ModelCaps{k.model: caps})
		if err == nil {
			ok, said = true, capSaid(caps)
		} else {
			said = err.Error()
		}
	}
	capProbesDone(k, ok, said)
}

// capCapsOf is what a probe's answer settled, as the provider keeps it:
// the letters it said, none for a tag it said nothing of (capability.go's
// ModelCaps, Result.Caps' shape).
func capCapsOf(said map[string]string) provider.ModelCaps {
	return provider.ModelCaps{Tools: said["tools"], Vision: said["vision"], Stream: said["stream"]}
}

// capProbesDone takes in what the probe of k made — the letters it settled
// or the words it failed with — and when the next may go. One line in the
// log, the record kept, the offer let go of, the loop woken for the next
// turn.
func capProbesDone(k capProbeKey, ok bool, said string) {
	now := time.Now()
	r := capProbeRecord{At: now, OK: ok, Said: said}
	if ok {
		log.Printf("cap probe: %s/%s settled %s", k.id, k.model, said)
	} else {
		capProbes.Lock()
		r.Fails = capProbes.recs[k].Fails + 1
		r.Next = now.Add(capProbeBackoff(r.Fails))
		capProbes.Unlock()
		log.Printf("cap probe: %s/%s failed: %s; asked again in %s", k.id, k.model, said, capProbeBackoff(r.Fails))
	}
	capProbes.Lock()
	capProbes.recs[k] = r
	capProbes.asking--
	capProbes.dirty = true
	capProbes.Unlock()
	saveCapProbes()
	deleteQueued(k)
	wakeCapProbe()
}

// deleteQueued lets go of the offer once taken: asked or dropped, the next
// plan that lacks the model offers it again.
func deleteQueued(k capProbeKey) {
	capProbes.Lock()
	delete(capProbes.queued, k)
	capProbes.Unlock()
}

// capSaid is what a probe settled, as the pickers' letters spell it: T
// tools, V vision, S streaming, ticked or crossed, and the tags it settled
// only.
func capSaid(c provider.ModelCaps) string {
	said := ""
	for _, t := range []struct{ l, v string }{{"T", c.Tools}, {"V", c.Vision}, {"S", c.Stream}} {
		if t.v != "yes" && t.v != "no" {
			continue
		}
		mark := "✗"
		if t.v == "yes" {
			mark = "✓"
		}
		if said != "" {
			said += " "
		}
		said += t.l + mark
	}
	return said
}

// CapProbeState is what the background probes have of one provider, as the
// Providers page reads it beside the letters a Test capabilities run
// settled: how many of its models wait in the queue, and each one's last
// probe — when, and what it settled.
type CapProbeState struct {
	Queued int                   `json:"queued"`
	Models map[string]CapProbeAt `json:"models,omitempty"`
}

// CapProbeAt is one model's last probe: when it was asked, whether it
// settled letters, and what they say.
type CapProbeAt struct {
	At   time.Time `json:"at,omitzero"`
	OK   bool      `json:"ok"`
	Said string    `json:"said,omitempty"`
}

// CapProbeOf is the state above for id.
func CapProbeOf(id string) CapProbeState {
	capProbes.Lock()
	defer capProbes.Unlock()
	out := CapProbeState{}
	for _, k := range capProbes.wait {
		if k.id == id {
			out.Queued++
		}
	}
	for k, r := range capProbes.recs {
		if k.id != id {
			continue
		}
		if out.Models == nil {
			out.Models = map[string]CapProbeAt{}
		}
		out.Models[k.model] = CapProbeAt{At: r.At, OK: r.OK, Said: r.Said}
	}
	return out
}
