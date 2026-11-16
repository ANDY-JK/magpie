package usageapi

import (
	"testing"
	"time"

	"github.com/yetone/magpie/internal/usage"
)

// hourlyHome gives the test a home of its own and holds the clock (the
// clock test's lesson: a fixture pinned to a date is tomorrow somewhere,
// and the run's zone must never be the thing the answer hangs on), so the
// usage log the test reads is the one it wrote itself.
func hourlyHome(t *testing.T) time.Time {
	t.Helper()
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "APPDATA", "LOCALAPPDATA"} {
		t.Setenv(key, home)
	}
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	old := usage.Clock
	usage.Clock = func() time.Time { return now }
	t.Cleanup(func() { usage.Clock = old })
	// the user's own session files, or one another test left, are no part
	// of what this test wrote
	oldLogs := usage.LogCalls
	usage.LogCalls = nil
	t.Cleanup(func() { usage.LogCalls = oldLogs })
	return now
}

// The window's hours: the calls of the last 24 by the hour they came in, a
// quiet hour an empty point so the chart holds its shape, one that fell
// before the window in none of them.
func TestHourlyBucketsTheWindowByHour(t *testing.T) {
	now := hourlyHome(t)
	priced := func(r usage.Record) usage.Record {
		r.Model, r.Provider = "m", "prov"
		r.Input, r.Output = 100, 10
		return r
	}
	usage.Append(priced(usage.Record{Time: now.Add(-30 * time.Minute), Status: 200, ProviderKeyID: "k1"}))
	usage.Append(usage.Record{Time: now.Add(-30 * time.Minute), Status: 429, ProviderKeyID: "k2", Model: "m", Provider: "prov"})
	usage.Append(priced(usage.Record{Time: now.Add(-2 * time.Hour), Status: 200}))
	usage.Append(priced(usage.Record{Time: now.Add(-26 * time.Hour), Status: 200})) // before the window
	h := HourlyOf(24, "", time.UTC)
	if h.Hours != 24 || len(h.Points) != 24 {
		t.Fatalf("hours %d of %d points, want 24", h.Hours, len(h.Points))
	}
	if !h.Since.Equal(now.Add(-24 * time.Hour)) {
		t.Fatalf("since %v, want %v", h.Since, now.Add(-24*time.Hour))
	}
	var calls, errors int64
	for i, pt := range h.Points {
		if !pt.T.Equal(h.Since.Add(time.Duration(i) * time.Hour)) {
			t.Fatalf("point %d is at %v, want an hour from since", i, pt.T)
		}
		calls, errors = calls+pt.Calls, errors+pt.Errors
		switch i {
		case 23: // the two calls of half an hour ago, one of them failed
			if pt.Calls != 2 || pt.Errors != 1 {
				t.Fatalf("the last hour read %d calls, %d errors, want 2 and 1", pt.Calls, pt.Errors)
			}
			if pt.Tokens != 110 {
				t.Fatalf("the last hour read %d tokens, want the priced one's 110 (the failed one had none)", pt.Tokens)
			}
			if len(pt.Groups) != 0 {
				t.Fatalf("no split asked, %d groups came", len(pt.Groups))
			}
		case 22: // the call two hours ago
			if pt.Calls != 1 || pt.Errors != 0 {
				t.Fatalf("the hour before read %d calls, %d errors, want 1 and 0", pt.Calls, pt.Errors)
			}
		default:
			if pt.Calls != 0 {
				t.Fatalf("hour %d read %d calls, want none", i, pt.Calls)
			}
		}
	}
	if calls != 3 || errors != 1 {
		t.Fatalf("the window read %d calls, %d errors, want 3 and 1", calls, errors)
	}
	// a shorter window says so, and a long one is held to the week it serves
	if h := HourlyOf(0, "", time.UTC); h.Hours != 24 {
		t.Fatalf("no hours asked read %d, want the 24 of a day", h.Hours)
	}
	if h := HourlyOf(500, "", time.UTC); h.Hours != 168 {
		t.Fatalf("%d hours asked read %d, want the week's 168", 500, h.Hours)
	}
}

// ?by= splits each hour by the vendor key, the account that answered or
// the model, the most tokens first, the calls with none named under the
// empty id.
func TestHourlySplitsByTheKeyAccountOrModel(t *testing.T) {
	now := hourlyHome(t)
	// two calls by k1 (one of them failed), one by k2, one by no key at
	// all; the accounts and models named along
	usage.Append(usage.Record{Time: now.Add(-30 * time.Minute), Status: 200, ProviderKeyID: "k1", ProviderAccount: "a@example.com", Model: "m1", Input: 50})
	usage.Append(usage.Record{Time: now.Add(-40 * time.Minute), Status: 429, Provider: "prov", Model: "m1", ProviderKeyID: "k1", ProviderAccount: "a@example.com"})
	usage.Append(usage.Record{Time: now.Add(-20 * time.Minute), Status: 200, ProviderKeyID: "k2", Host: "host as b@example.com", Model: "m2", Output: 10})
	usage.Append(usage.Record{Time: now.Add(-10 * time.Minute), Status: 200, Model: "m2"})
	h := HourlyOf(24, "key", time.UTC)
	pt := h.Points[23]
	if len(pt.Groups) != 3 {
		t.Fatalf("the hour split into %d keys, want k1, k2 and the keyless one", len(pt.Groups))
	}
	if pt.Groups[0].ID != "k1" || pt.Groups[0].Calls != 2 || pt.Groups[0].Errors != 1 || pt.Groups[0].Tokens != 50 {
		t.Fatalf("k1's share: %+v", pt.Groups[0])
	}
	if pt.Calls != pt.Groups[0].Calls+pt.Groups[1].Calls+pt.Groups[2].Calls {
		t.Fatalf("the hour's calls %d are not its groups' sum: %+v", pt.Calls, pt.Groups)
	}
	// a call the session's host names the account of is that account's,
	// as Record.Account reads it; the calls with none are the empty id
	if a := HourlyOf(24, "account", time.UTC).Points[23]; len(a.Groups) != 3 || a.Groups[0].ID != "a@example.com" || a.Groups[2].ID != "" {
		t.Fatalf("the hour by account: %+v", a.Groups)
	}
	if m := HourlyOf(24, "model", time.UTC).Points[23]; len(m.Groups) != 2 || m.Groups[0].ID != "m1" || m.Groups[1].ID != "m2" {
		t.Fatalf("the hour by model: %+v (m1's calls carried the 50 tokens, m2's the 10)", m.Groups)
	}
	// an unknown split is none
	if n := HourlyOf(24, "agent", time.UTC).Points[23]; len(n.Groups) != 0 {
		t.Fatalf("an unknown split gave %d groups", len(n.Groups))
	}
}

// The hours are the reading's, whatever zone names them: the same calls in
// the same instants of the same buckets, the zone only in how the JSON
// spells the times (the clock test's lesson, run in two zones).
func TestHourlyReadsTheSameInTwoZones(t *testing.T) {
	now := hourlyHome(t)
	usage.Append(usage.Record{Time: now.Add(-30 * time.Minute), Status: 200, Input: 5, ProviderKeyID: "k1"})
	usage.Append(usage.Record{Time: now.Add(-90 * time.Minute), Status: 429, Provider: "prov", Model: "m", ProviderKeyID: "k2"})
	utc := HourlyOf(24, "", time.UTC)
	// fourteen hours ahead of UTC, as far as a zone goes from it
	far := time.FixedZone("w", 14*3600)
	inFar := HourlyOf(24, "", far)
	for i, a := range utc.Points {
		b := inFar.Points[i]
		if a.Calls != b.Calls || a.Errors != b.Errors || a.Tokens != b.Tokens || a.Cost != b.Cost {
			t.Fatalf("hour %d read %+v in UTC and %+v far away", i, a, b)
		}
		if !a.T.Equal(b.T) {
			t.Fatalf("hour %d began %v in UTC and %v far away, the same instant wanted", i, a.T, b.T)
		}
	}
	if utc.Points[0].T.Format("-07:00") == inFar.Points[0].T.Format("-07:00") {
		t.Fatal("the two reads spelled their times in the same offset")
	}
	if utc.Points[23].Calls != 1 || utc.Points[22].Calls != 1 {
		t.Fatalf("UTC read %+v, want one call in each of the last two hours", utc.Points[22:24])
	}
}
