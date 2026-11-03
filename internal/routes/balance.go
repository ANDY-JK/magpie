package routes

import "sort"

// Balanced weighs a pool's accounts and keys together rather than by one
// measure: what each seat would lose at its reset, how it has answered
// lately, how its vendor's other seats have, and what magpie has sent it
// against its weight. No seat is used up while the others sit idle, one
// that keeps failing is not asked again while healthy ones can answer, and
// a vendor that is refusing magpie is left to the others once its accounts
// say so.
//
// Its tiers are Smart's: a seat with room goes before one at 90% or more of
// a window, and that before one all but used up (98%), so nothing running
// out is spent while there is room elsewhere; one running low goes by the
// share used, and one all but used up that goes on on its credits first
// (spentBefore). Within a tier:
//
//   - a seat that failed since it last answered goes behind those that
//     have answered since, the one failing most last: it rests meanwhile
//     as in any routing, and when the rest ends it is tried after them,
//     so one that keeps failing is asked again only when the others
//     can't answer.
//   - a seat whose vendor's seats in the pool are failing goes behind
//     those of a vendor whose aren't, by how many of its seats are. A
//     vendor that refuses magpie refuses its accounts alike, so the ones
//     it happened to ask are enough to find out: WorkBuddy's ten accounts
//     were refused one after another before their vendor was still (AI
//     11140). One provider's seats are one vendor, so a pool of one
//     provider goes by the seat's own failures alone.
//   - the seat with the highest pace goes first, as in Use before expiry,
//     those within a tenth of each other in one band: what has the most to
//     lose soonest is spent first, so little is lost at resets and to
//     credits that expire before they do. A key that reads no windows has
//     no pace and keeps its place in the last band, behind subscriptions
//     spending quota they would lose, as #4535 asked.
//   - of a band, one answering more than four times more slowly than the
//     fastest seat of the pool goes behind the rest of it: a channel that
//     is up but crawling gives way to one that works. One not measured
//     lately counts as fast, and is tried and measured.
//   - then what magpie sent each lately, against its weight: a seat
//     weighing 3 carries three times the tokens one weighing 1 does
//     between their turns, and seats alike are taken in turn rather than
//     one being used until it is nearly out — the same accounts renew
//     together, so renewal alone would drain the first before the second
//     (opencode-go's two plans). A wallet with less left goes first among
//     its provider's and unit (byLeft).
//   - the listed order, so a pool alike throughout keeps the vendor's
//     prompt cache warm.
//
// A Claude account not known yet that tells what it has left as it answers
// goes first once, as in Smart, Least used first and Use before expiry:
// kept behind the known ones it would never answer and never be known.
//
// Price is not weighed: Free first and Cheapest first are for what a
// request costs, which is a different question from what spending it here
// would lose.
var Balanced = Strategy{ByUse: true, BySpeed: true, Order: func(_ string, seats []Seat) []int {
	failing := map[string]int{} // a vendor → how many of its seats in the pool are failing
	for _, s := range seats {
		failing[s.Vendor] += s.Failures
	}
	band := paceBands(seats)
	slow := slowSeats(seats)
	left := byLeft(seats, loadLess)
	idx := identity(len(seats))
	sort.SliceStable(idx, func(a, b int) bool {
		sa, sb := seats[idx[a]], seats[idx[b]]
		pa, pb := balancePlace(sa), balancePlace(sb)
		switch {
		case pa != pb:
			return pa < pb
		case pa == placeLow:
			return sa.Used < sb.Used
		case pa == placeSpent:
			return spentBefore(sa, sb)
		}
		if sa.Failures != sb.Failures {
			return sa.Failures < sb.Failures
		}
		if fa, fb := failing[sa.Vendor], failing[sb.Vendor]; fa != fb {
			return fa < fb
		}
		if band[idx[a]] != band[idx[b]] {
			return band[idx[a]] < band[idx[b]]
		}
		if slow[idx[a]] != slow[idx[b]] {
			return !slow[idx[a]]
		}
		if left != nil {
			if c := left(idx[a], idx[b]); c != 0 {
				return c < 0
			}
		}
		return loadLess(sa, sb)
	})
	return idx
}}

// Where a seat lands in Balanced's order: a learning account first, once,
// then those with room, then those running low, then those all but used up.
const (
	placeLearns = iota
	placeFine
	placeLow
	placeSpent
)

func balancePlace(s Seat) int {
	if s.Learns {
		return placeLearns
	}
	if t := seatTier(s); t > 0 {
		return placeFine + t
	}
	return placeFine
}

// loadLess is what Balanced spreads a band by: the tokens magpie sent a
// seat lately, against its weight, the fewest first. A seat weighing 3 may
// carry three times the tokens of one weighing 1 before the other is tried.
func loadLess(a, b Seat) bool { return loadOf(a) < loadOf(b) }

func loadOf(s Seat) float64 { return s.Tokens / float64(max(s.Weight, 1)) }

// slowAfter: a seat whose reply would take this many times the fastest
// seat's is slow, and Balanced puts it behind the rest of its band. Four
// is a channel still up, but one an agent's turn waits on several times
// over.
const slowAfter = 4

// slowSeats marks the seats more than slowAfter times slower than the
// fastest seat of the pool that answered lately, by when a reply of
// RefTokens would be done (Estimate). One not measured lately is not slow:
// it is tried, and its answer measures it.
func slowSeats(seats []Seat) []bool {
	var paces []float64
	for _, s := range seats {
		if s.TTFT > 0 && s.TPS > 0 {
			paces = append(paces, s.TPS)
		}
	}
	mid := 0.0
	if len(paces) > 0 {
		sort.Float64s(paces)
		mid = paces[len(paces)/2]
	}
	est := make([]float64, len(seats))
	fastest := 0.0
	for i, s := range seats {
		if s.TTFT <= 0 {
			continue
		}
		est[i] = Estimate(s.TTFT, s.TPS, mid)
		if fastest == 0 || est[i] < fastest {
			fastest = est[i]
		}
	}
	slow := make([]bool, len(seats))
	if fastest <= 0 {
		return slow
	}
	for i := range seats {
		slow[i] = est[i] > slowAfter*fastest
	}
	return slow
}
