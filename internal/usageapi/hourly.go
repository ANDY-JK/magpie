package usageapi

// A window's calls by hour: what GET /api/usage/hourly answers, the same
// rows the Usage page reads (the ledger's, the user's clears among them),
// summed into hour buckets for a chart of when the day's calls came, how
// many failed, and what they spent. The req/min a chart derives from a
// point is its calls over 60 — the calls bucket is the only count of
// requests there is, nothing is counted again for it.

import (
	"slices"
	"time"

	"github.com/yetone/magpie/internal/usage"
)

// HourGroup is an hour's share of the calls one vendor key, account or
// model made, when the read split the hours by it.
type HourGroup struct {
	ID     string  `json:"id"`
	Calls  int64   `json:"calls"`
	Errors int64   `json:"errors"`
	Tokens int64   `json:"tokens"`
	Cost   float64 `json:"cost"`
}

// HourPoint is one hour: its calls, how many of them failed, their tokens
// and their cost at the effective price. Groups, when the read split them,
// are the most tokens first.
type HourPoint struct {
	T      time.Time   `json:"t"`
	Calls  int64       `json:"calls"`
	Errors int64       `json:"errors"`
	Tokens int64       `json:"tokens"`
	Cost   float64     `json:"cost"`
	Groups []HourGroup `json:"groups,omitempty"`
}

// Hourly is a window by hours: Since when it began, Points one per hour
// from there, a quiet hour an empty point so the chart holds its shape —
// the same buckets the usage page's timeline gives a rolling window, which
// begin at Since and not at a clock hour.
type Hourly struct {
	Since  time.Time   `json:"since"`
	Hours  int         `json:"hours"`
	Points []HourPoint `json:"points"`
}

// HourlyOf is the calls of the last hours, as the ledger keeps them —
// rejected calls and probes none of them, as the page's own chart leaves
// them out — summed by hour from Since, the reader's zone naming the hours
// in the JSON. by, "key", "account" or "model", splits each hour by the
// vendor key the call went out by (Record.ProviderKeyID), the account that
// answered (Record.Account) or the model (Record.Model); anything else is
// no split.
func HourlyOf(hours int, by string, zone *time.Location) Hourly {
	if hours <= 0 {
		hours = 24
	}
	if hours > 168 {
		hours = 168
	}
	if zone == nil {
		zone = time.Local
	}
	now := usage.Clock().In(zone)
	p, _ := usage.LastOf(time.Duration(hours) * time.Hour)
	rows := usage.LedgerOfAt(p, usage.Filter{}, now).Rows
	since := now.Add(-time.Duration(hours) * time.Hour)
	out := Hourly{Since: since, Hours: hours, Points: make([]HourPoint, hours)}
	for i := range out.Points {
		out.Points[i].T = since.Add(time.Duration(i) * time.Hour)
	}
	for _, r := range rows {
		if r.IsRejected() || r.IsProbe() {
			continue
		}
		t := r.Time
		if t.Before(since) {
			continue
		}
		i := int(t.Sub(since).Hours()) // the bucket since lands in, as the timeline counts its hours
		if i < 0 || i >= len(out.Points) {
			continue
		}
		pt := &out.Points[i]
		pt.Calls++
		if r.Failed() {
			pt.Errors++
		}
		tokens := int64(r.Input) + int64(r.Output) + int64(r.CacheRead) + int64(r.CacheWrite)
		pt.Tokens += tokens
		if r.Priced {
			pt.Cost += r.Cost
		}
		// the split the read asked for, none but these three
		var id string
		grouped := false
		switch by {
		case "key":
			id, grouped = r.ProviderKeyID, true
		case "account":
			id, grouped = r.Account(), true
		case "model":
			id, grouped = r.Model, true
		}
		if grouped {
			pt.Groups = putHourGroup(pt.Groups, id, r.Failed(), tokens, r)
		}
	}
	for i := range out.Points {
		slices.SortStableFunc(out.Points[i].Groups, func(a, b HourGroup) int {
			if a.Tokens != b.Tokens {
				return cmpTokens(b.Tokens, a.Tokens) // the more tokens first
			}
			return cmpID(a.ID, b.ID)
		})
	}
	return out
}

// cmpTokens orders two counts.
func cmpTokens(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// putHourGroup counts one row into its hour's group for id, made when the
// first row of it comes.
func putHourGroup(groups []HourGroup, id string, failed bool, tokens int64, r usage.Row) []HourGroup {
	for i := range groups {
		if groups[i].ID == id {
			g := &groups[i]
			g.Calls++
			if failed {
				g.Errors++
			}
			g.Tokens += tokens
			if r.Priced {
				g.Cost += r.Cost
			}
			return groups
		}
	}
	g := HourGroup{ID: id}
	g.Calls++
	if failed {
		g.Errors++
	}
	g.Tokens += tokens
	if r.Priced {
		g.Cost += r.Cost
	}
	return append(groups, g)
}

// cmpID orders two group ids by text, an empty one (no key, no account
// named) first, so the order a read answers is the order another read of
// the same rows answers.
func cmpID(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return -1
	}
	if b == "" {
		return 1
	}
	if a < b {
		return -1
	}
	return 1
}
