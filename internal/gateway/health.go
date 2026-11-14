package gateway

import (
	"math"
	"slices"
	"sync"
	"time"

	"github.com/yetone/magpie/internal/settings"
)

// Model stability: a model that is sometimes good and sometimes not is
// tried after the ones that are steady, so an intermittent 500 or a
// first-token timeout costs one fallback rather than one per request.
//
// The score is per model, not per key or account: the question is whether
// THIS vendor's model is reliable, and the same model on another account
// is the same model. Quota, credit, rate-limit, auth and verification
// failures never touch it — a model out of allowance is not unstable, it
// is out (their own rests handle them).
//
// The score is the model's observed success rate, not a ladder of fixed
// multipliers: every answer and every intermittent failure is one
// observation, each decayed by age so a bad afternoon is forgotten and a
// model fixed overnight is tried first again. The rate is smoothed by
// healthPrior, which is what a model with no history is taken for — good —
// so a single blip on a fresh model moves nothing, while a model whose
// record is long enough to be believable is judged by what it actually
// did. Below healthDemote the model is moved behind the others of its
// plan, keeping the relative order of both sides — the user's routing
// order still decides within the healthy ones.
const (
	// healthHalfLife is how long an observation takes to lose half its
	// weight: what a model did an hour ago counts for less than now.
	healthHalfLife = 30 * time.Minute
	// healthPrior is the weight of the "no history" assumption that a
	// model is steady. One failure against it reads 1/2, two 1/3; a model
	// with a real record is judged by that record instead.
	healthPrior = 1.0
	// healthDemote: a model below this is tried after its healthy peers.
	healthDemote = 0.5
	// healthShare is the share of the plan's best score below which a
	// model is taken for unstable whatever the absolute score: a model
	// failing half its calls settles at about healthDemote and would
	// otherwise sit exactly on the line forever, while beside a model
	// that answers every time it is plainly the worse one.
	healthShare = 0.75
	// healthEnough is how many observations the relative rule needs
	// before it speaks: two or three calls are not a record, and the
	// absolute rule already leaves a single blip in place.
	healthEnough = 4.0
	// healthForget is how long an untouched score is kept.
	healthForget = 24 * time.Hour
)

// healthPoint is a model's record: successes and failures, each decayed
// toward nothing by age (weight halves every healthHalfLife).
type healthPoint struct {
	wins float64
	loss float64
	at   time.Time
}

var modelHealth = struct {
	sync.Mutex
	m map[string]healthPoint
}{m: map[string]healthPoint{}}

// decayed brings h's counts up to now.
func (h healthPoint) decayed(now time.Time) healthPoint {
	if h.at.IsZero() {
		return h
	}
	dt := now.Sub(h.at)
	if dt <= 0 {
		return h
	}
	w := math.Exp2(-dt.Seconds() / healthHalfLife.Seconds())
	h.wins *= w
	h.loss *= w
	h.at = now
	return h
}

// healthOf is id's score now: its observed success rate, smoothed by the
// prior. With no observation at all it is 1.
func healthOf(id string, now time.Time) float64 {
	modelHealth.Lock()
	defer modelHealth.Unlock()
	h, ok := modelHealth.m[id]
	if !ok {
		return 1
	}
	h = h.decayed(now)
	modelHealth.m[id] = h
	return (h.wins + healthPrior) / (h.wins + h.loss + healthPrior)
}

// noteUnstable counts one intermittent failure of c's model.
func noteUnstable(c candidate) {
	note(c, false)
}

// noteStable counts one answer of c's model.
func noteStable(c candidate) {
	note(c, true)
}

// note records one observation of c's model, success or failure.
func note(c candidate, ok bool) {
	if c.model == "" {
		return
	}
	now := time.Now()
	id := c.restID()
	modelHealth.Lock()
	defer modelHealth.Unlock()
	h := modelHealth.m[id].decayed(now)
	if ok {
		h.wins++
	} else {
		h.loss++
	}
	h.at = now
	if len(modelHealth.m) > 4096 {
		for k, v := range modelHealth.m {
			if now.Sub(v.at) > healthForget {
				delete(modelHealth.m, k)
			}
		}
	}
	modelHealth.m[id] = h
}

// resetHealth forgets every score: a test starts as a fresh process
// does, the package's tests sharing provider ids otherwise (a failure in
// one demoting the next one's model).
func resetHealth() {
	modelHealth.Lock()
	modelHealth.m = map[string]healthPoint{}
	modelHealth.Unlock()
}

// unstable says c's model is below healthDemote right now, in absolute
// terms.
func unstable(c candidate) bool {
	return healthOf(c.restID(), time.Now()) < healthDemote
}

// healthMark is what a plan's candidates are judged against: the best
// score among them. A model far below the best one is unstable however
// high its own score is, which is what catches a model that fails about
// half its calls — that settles at healthDemote exactly and never crosses
// it, while beside a steady peer it is plainly the one to try last.
type healthMark struct {
	at    time.Time
	best  float64
	known map[string]healthPoint // each of the plan's models, read once
}

// markOf is the mark for cs now: every candidate's record read under one
// lock, and the highest score among them. Taking the snapshot once keeps
// a plan of many members from taking the lock per member per comparison.
func markOf(cs []candidate, now time.Time) healthMark {
	m := healthMark{at: now, known: map[string]healthPoint{}}
	modelHealth.Lock()
	for _, c := range cs {
		if c.model == "" {
			continue
		}
		id := c.restID()
		if _, seen := m.known[id]; seen {
			continue
		}
		h, ok := modelHealth.m[id]
		if !ok {
			m.known[id] = healthPoint{}
			if 1 > m.best {
				m.best = 1
			}
			continue
		}
		h = h.decayed(now)
		modelHealth.m[id] = h
		m.known[id] = h
		if s := scoreOf(h); s > m.best {
			m.best = s
		}
	}
	modelHealth.Unlock()
	return m
}

// scoreOf is a record's observed success rate, smoothed by the prior.
func scoreOf(h healthPoint) float64 {
	return (h.wins + healthPrior) / (h.wins + h.loss + healthPrior)
}

// demoted says whether c is tried after its peers under m: below the
// absolute line, or far enough below the plan's best one.
func demoted(c candidate, m healthMark) bool {
	if c.model == "" {
		return false
	}
	h, seen := m.known[c.restID()]
	if !seen {
		return false
	}
	s := scoreOf(h)
	if s < healthDemote {
		return true
	}
	if m.best <= 0 {
		return false
	}
	// only a model with a record of its own is judged against the best:
	// a fresh one beside a model with a long, perfect history is not
	// unstable, it is simply unknown
	if h.wins+h.loss < healthEnough {
		return false
	}
	return s < m.best*healthShare
}

// demoteUnstable moves the plans's unstable models behind the rest,
// keeping each side's order and the pairs of cands and pl.order aligned.
// A resting candidate is last of all, whatever its health: it can't answer
// now, and the rest may be a quota that health does not judge — but the
// demotion must not put a model that can answer behind one that can't, so
// an unstable one still comes before the resting ones.
func demoteUnstable(cs []candidate, pl planned) ([]candidate, planned) {
	if len(cs) < 2 || len(pl.order) != len(cs) {
		return cs, pl
	}
	if settings.Load().NoHealthDemotion {
		return cs, pl // the user asked for routing's own order throughout
	}
	m := markOf(cs, time.Now())
	rank := func(i int) int {
		if pl.order[i].Rest != nil {
			return 2
		}
		if demoted(cs[i], m) {
			return 1
		}
		return 0
	}
	idx := make([]int, len(cs))
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int { return rank(a) - rank(b) })
	moved := false
	for j, i := range idx {
		if i != j {
			moved = true
		}
	}
	if !moved {
		return cs, pl
	}
	out := make([]candidate, len(cs))
	order := make([]Weighed, len(cs))
	for j, i := range idx {
		out[j], order[j] = cs[i], pl.order[i]
	}
	pl.order = order
	return out, pl
}
