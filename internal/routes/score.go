package routes

import (
	"sort"

	"github.com/yetone/magpie/internal/provider"
)

// Score goes by a seat's score: what it has done lately and what it has
// left, together, rather than by one measure. It is for a pool that should
// follow its providers as they behave — one that answers steadily and
// quickly with room left first — and the weights are constants here, in
// source alone: the window offers no way to change them.
//
// It orders in layers, a layer always before the score, so a seat
// everything else favors can't be spent while another has room:
//
//  0. quota: a seat about to lose what it has at a reset first (Seat.Loses),
//     then those with room, those running low and those all but used up
//     (seatTier's 90% and 98% lines), as Balanced keeps them. A seat at 99%
//     used that leads on every other measure never goes before one with
//     room at 85%.
//  1. reliability: one resting after a failure goes after every one awake,
//     and one that failed since it last answered after those that have
//     answered since, the one failing most last (Seat.Failures, Seat.Resting).
//  2. the score, the highest first. Seats alike keep their listed order, so
//     a pool alike throughout keeps the vendor's prompt cache warm.
//
// The score is 0–1, a weighted sum of four dimensions:
//
//	0.40 health       how it has answered lately (Seat.Health, 0–1); one
//	                  with no record yet counts health.go's prior 1.0, so a
//	                  new seat starts even
//	0.18 first token  1 at once, down to 0 at 3 seconds or slower
//	0.12 throughput   0 at nothing a second, up to 1 at 60 output tokens
//	0.20 quota left   the share unused, and its pace against a whole week,
//	                  each half (Seat.Used, Seat.Pace)
//
// A fifth dimension, ability (the tags a request needs: tools, vision,
// stream), is absent: weighRouted and plan carry no request body, and
// Provider.Caps holds only what a probe settled, yes or no, with no
// per-request check; for a request that needs nothing it would be 1 for
// every seat, a constant that lifts all alike. It returns when the
// request's needs reach a strategy through weighRouted and plan — a
// probed "no" excluding the seat — with its 0.10 weighed again and these
// four renormalized. Until then the four keep their shares, normalized
// here to sum 1.
//
// A dimension nothing was measured for lately counts 0.5: neither rewarded
// nor punished, so a seat is never put first or last for being new. Health
// alone outweighs both speed dimensions together, since a link that drops
// fails a turn where a slow one only takes longer.
const (
	scoreShares = 0.40 + 0.18 + 0.12 + 0.20

	scoreWeightHealth = 0.40 / scoreShares
	scoreWeightTTFT   = 0.18 / scoreShares
	scoreWeightTPS    = 0.12 / scoreShares
	scoreWeightQuota  = 0.20 / scoreShares
)

const (
	// scoreTTFTFull is the first token, in ms, at which a seat scores
	// nothing for it: 3 seconds.
	scoreTTFTFull = 3000.0
	// scoreTPSFull is the pace that scores full for throughput: 60 output
	// tokens a second.
	scoreTPSFull = 60.0
	// scoreNeutral is a dimension with nothing measured lately.
	scoreNeutral = 0.5
	// scoreHealthPrior is a health with no record yet: the prior the
	// gateway's health score starts a seat from.
	scoreHealthPrior = 1.0
)

// Score is the routing a pool is ordered by its seats' scores, as above.
var Score = Strategy{ByUse: true, BySpeed: true, ByHealth: true, Order: scoreOrder}

// scoreOrder is Score's order: the layers above, then the score.
func scoreOrder(_ string, seats []Seat) []int {
	scores := make([]float64, len(seats))
	for i, s := range seats {
		scores[i] = ScoreOf(s)
	}
	idx := identity(len(seats))
	sort.SliceStable(idx, func(a, b int) bool {
		sa, sb := seats[idx[a]], seats[idx[b]]
		if pa, pb := scorePlace(sa), scorePlace(sb); pa != pb {
			return pa < pb
		}
		if sa.Failures != sb.Failures {
			return sa.Failures < sb.Failures
		}
		return scores[idx[a]] > scores[idx[b]]
	})
	return idx
}

// Where a seat lands before the score: one about to lose what it has, then
// the quota tiers, and one resting after every one awake whatever its quota.
const (
	scorePlaceLoses   = iota
	scorePlaceRoom    // with room, plus seatTier: low, then all but used up
	scorePlaceResting = scorePlaceRoom + 3
)

func scorePlace(s Seat) int {
	if s.Resting {
		return scorePlaceResting
	}
	if s.Loses {
		return scorePlaceLoses
	}
	return scorePlaceRoom + seatTier(s)
}

// ScoreOf is a seat's score, 0–1, as the weights above say: scoreOrder
// orders by it, and the gateway's trace tells the same value it weighed
// (weighing.scores in internal/gateway).
func ScoreOf(s Seat) float64 {
	health := scoreHealthPrior
	if s.Health > 0 {
		health = clampShare(s.Health)
	}
	ttft := scoreNeutral
	if s.TTFT > 0 {
		ttft = 1 - clampShare(s.TTFT/scoreTTFTFull)
	}
	tps := scoreNeutral
	if s.TPS > 0 {
		tps = clampShare(s.TPS / scoreTPSFull)
	}
	return scoreWeightHealth*health + scoreWeightTTFT*ttft + scoreWeightTPS*tps +
		scoreWeightQuota*scoreQuota(s)
}

// scoreQuota is the allowance a seat has left, 0–1: the share unused, and
// its pace against a seat with a whole week ahead of it, each half. A seat
// whose allowance wasn't read counts neutral: what it has left is unknown,
// not nothing, as #4535 asked of a key with no share.
func scoreQuota(s Seat) float64 {
	if !s.Known {
		return scoreNeutral
	}
	left := 1 - clampShare(s.Used/100)
	pace := scoreNeutral
	if s.Pace > 0 {
		pace = clampShare(s.Pace / provider.FreshPace)
	}
	return (left + pace) / 2
}

// clampShare keeps v within 0–1.
func clampShare(v float64) float64 { return min(max(v, 0), 1) }
