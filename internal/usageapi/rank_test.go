package usageapi

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/sessions"
	"github.com/yetone/magpie/internal/usage"
)

// rankCase is one provider of a ranking test: what its calls were like, and
// how many days of the period they spread over.
type rankCase struct {
	id       string
	name     string
	calls    int
	failed   int
	retried  int // of the calls that answered, how many took a try that failed
	timed    int // of the calls that answered, how many a session file timed
	ttft     int64
	out      int // each timed call's answer tokens
	ms       int64
	days     int
	accounts []string
	hidden   bool
	deleted  bool
}

// rankHome gives the test a home of its own and holds the clock, so the
// usage log and the providers it reads are the ones it wrote itself. Every
// folder the code reads is moved: testenv.SetHome leaves XDG_CONFIG_HOME,
// which a package-wide TestMain sets once, so the whole package would share
// one usage.jsonl.
func rankHome(t *testing.T) time.Time {
	t.Helper()
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "APPDATA", "LOCALAPPDATA"} {
		t.Setenv(key, home)
	}
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
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

// rankLog writes one provider's calls into the test's usage log, spread over
// its days up to now: day 0 is now's, day n is n days before it, so a
// provider of rankCase.days days answers on that many different days.
func rankLog(t *testing.T, c rankCase) {
	t.Helper()
	now := usage.Clock()
	share := func(i, of int) int { return i * of / max(1, c.calls) }
	for i := 0; i < c.calls; i++ {
		at := now.Add(-time.Duration(share(i, c.days)) * 24 * time.Hour)
		r := usage.Record{Time: at, Agent: "codex", Provider: c.id, Model: "m-" + c.id, Input: 100, Output: 50, Status: 200}
		if i < c.failed {
			r.Status, r.Error, r.ErrType = 429, "rate limit", "rate_limit_error"
		} else if i < c.failed+c.retried {
			r.Tries = 2 // answered, but a try failed first
		}
		if i >= c.failed && i < c.failed+c.timed {
			r.TTFT, r.Output, r.Millis = c.ttft, c.out, c.ms
		}
		if len(c.accounts) > 0 {
			r.ProviderAccount = c.accounts[i%len(c.accounts)]
		}
		usage.Append(r)
	}
}

func saveProvider(t *testing.T, c rankCase) {
	t.Helper()
	p := provider.Provider{ID: c.id, Name: c.name, Key: "k-" + c.id, Chat: "http://127.0.0.1:1/v1", Models: []string{"m-" + c.id}, Hidden: c.hidden}
	if err := provider.Save(p); err != nil {
		t.Fatal(err)
	}
	if c.deleted {
		if err := provider.Delete(c.id); err != nil {
			t.Fatal(err)
		}
	}
}

func rankFixtures(t *testing.T, cases ...rankCase) []provider.Provider {
	t.Helper()
	for _, c := range cases {
		saveProvider(t, c)
		rankLog(t, c)
	}
	return provider.All()
}

func rankEntry(t *testing.T, r Ranking, id string) RankEntry {
	t.Helper()
	for _, e := range r.Providers {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("%s is not ranked: %+v", id, r)
	return RankEntry{}
}

func rankPartMap(parts []RankPart) map[string]RankPart {
	out := map[string]RankPart{}
	for _, p := range parts {
		out[p.Key] = p
	}
	return out
}

func rankCauseKeys(r RankEntry) []string {
	out := []string{}
	for _, c := range r.Causes {
		out = append(out, c.Key)
	}
	return out
}

// The ranking reads only the gateway's own calls: a row from an agent's
// session file, one a client reported, a request the gateway turned away
// itself, a probe, and one of an unknown provider are none of its business,
// and none of them can put a provider in the ranking.
func TestRankIgnoresOtherSources(t *testing.T) {
	now := rankHome(t)
	for i := 0; i < RankMinCalls; i++ {
		at := now.Add(-time.Duration(i) * time.Minute)
		usage.Append(usage.Record{Time: at, Agent: "codex", Provider: "gone", Model: "m", Input: 10, Output: 1})
		usage.Append(usage.Record{Time: at, Agent: "codex", Provider: "reported", Model: "m", Input: 10, Output: 1, Source: usage.SourceReported})
		usage.Append(usage.Record{Time: at, Agent: "codex", Provider: "rejected", Model: "m", Input: 10, Output: 1, Status: 401, Rejected: true})
		usage.Append(usage.Record{Time: at, Agent: "magpie", Provider: "probe", Model: "m", Input: 10, Output: 1, Source: usage.SourceProbe})
		usage.Append(usage.Record{Time: at, Agent: "codex", Provider: usage.UnknownProvider, Model: "m", Input: 10, Output: 1})
	}
	// an agent's own session file is a call someone else made
	session := make([]sessions.Call, 0, RankMinCalls)
	for i := 0; i < RankMinCalls; i++ {
		session = append(session, sessions.Call{
			Time:   now.Add(-48 * time.Hour).Add(time.Duration(i) * time.Minute),
			Agent:  "claude",
			Model:  "session-model",
			Tokens: sessions.Tokens{Input: 7, Output: 3},
		})
	}
	usage.LogCalls = func(time.Time) []sessions.Call { return session }
	r := Rank(usage.All, provider.All(), time.Local)
	if len(r.Providers) != 1 || r.Providers[0].ID != "gone" {
		t.Fatalf("ranked: %+v", r.Providers)
	}
	if e := r.Providers[0]; e.Calls != RankMinCalls || e.Name != "gone" {
		t.Fatalf("the gateway's provider: %+v", e)
	}
	if len(r.Thin) != 0 || len(r.Gone) != 0 {
		t.Fatalf("thin %+v gone %+v", r.Thin, r.Gone)
	}
}

// A provider with fewer than RankMinCalls of its own is left out of the
// ranking, but one whose every row was a try handed on to another is named
// even so: its refusals are what made those requests retry.
func TestRankThinUnlessHandedOn(t *testing.T) {
	now := rankHome(t)
	for i := 0; i < RankMinCalls-1; i++ {
		usage.Append(usage.Record{Time: now.Add(-time.Duration(i) * time.Minute), Agent: "codex", Provider: "thin", Model: "m", Input: 10, Output: 1})
	}
	for i := 0; i < RankMinCalls; i++ {
		usage.Append(usage.Record{Time: now.Add(-time.Duration(i) * time.Minute), Agent: "codex", Provider: "failing", Model: "m", Input: 10, Output: 1, Status: 500, Error: "upstream", HandedOn: true})
	}
	r := Rank(usage.All, provider.All(), time.Local)
	if len(r.Providers) != 1 || r.Providers[0].ID != "failing" {
		t.Fatalf("ranked: %+v", r.Providers)
	}
	if len(r.Thin) != 1 || r.Thin[0].ID != "thin" {
		t.Fatalf("thin: %+v", r.Thin)
	}
	e := r.Providers[0]
	if e.Calls != 0 { // a handed-on try is no request of its own
		t.Fatalf("its calls: %+v", e.Totals)
	}
	if len(rankCauseKeys(e)) != 1 || rankCauseKeys(e)[0] != "upstream" {
		t.Fatalf("its causes: %+v", e.Causes)
	}
}

// The four parts, and what fills them: a provider every one of whose calls
// failed scores nothing served, and one whose every request took a try that
// failed first scores nothing stood. A reply no session file timed leaves
// the two pace parts untold, folded into the two that can be told.
func TestRankParts(t *testing.T) {
	rankHome(t)
	all := rankFixtures(t,
		rankCase{id: "good", name: "Good", calls: 20, timed: 20, ttft: 300, out: 1000, ms: 5000, days: 1},
		rankCase{id: "bad", name: "Bad", calls: 20, failed: 20, days: 1},
		rankCase{id: "retry", name: "Retry", calls: 20, retried: 20, days: 1},
		rankCase{id: "untimed", name: "Untimed", calls: 20, days: 1},
	)
	r := Rank(usage.All, all, time.Local)
	parts := func(id string) map[string]RankPart { return rankPartMap(rankEntry(t, r, id).Parts) }
	good, bad, retry, untimed := parts("good"), parts("bad"), parts("retry"), parts("untimed")
	if good["served"].Points != 25 || good["stood"].Points != 25 {
		t.Fatalf("a clean provider: %+v", good)
	}
	// the fastest provider of the period holds the pace: the good one's own
	// speed is the period's, so it gets its full fast part
	if good["fast"].Points != 25 || good["soon"].Points != 25 {
		t.Fatalf("the period's fastest: %+v", good)
	}
	// nothing timed: served and stood are each out of 50, and a provider
	// every one of whose calls failed keeps only what it stood
	if bad["served"].Points != 0 || bad["served"].Most != 50 {
		t.Fatalf("every call failed: %+v", bad)
	}
	if bad["stood"].Points != 50 || bad["stood"].Most != 50 {
		t.Fatalf("every call failed, its stood part: %+v", bad)
	}
	if retry["served"].Points != 50 || retry["stood"].Points != 0 {
		t.Fatalf("every request retried: %+v", retry)
	}
	if _, ok := untimed["fast"]; ok {
		t.Fatalf("an untimed provider has a pace part: %+v", untimed)
	}
	if untimed["served"].Most != 50 || untimed["served"].Points != 50 || untimed["stood"].Points != 50 {
		t.Fatalf("the folded parts: %+v", untimed)
	}
	if _, ok := bad["fast"]; ok {
		t.Fatalf("a provider with nothing timed has a pace part: %+v", bad)
	}
}

// A provider's models are ranked on their own, so one bad model of an
// otherwise good provider is told apart from a provider whose every model
// is bad. A model's own pace is not held against the period's fastest
// provider: the fastest provider's models would score nothing for being the
// same, so a model's pace parts are its served and stood again.
func TestRankModelsApart(t *testing.T) {
	now := rankHome(t)
	saveProvider(t, rankCase{id: "mixed", name: "Mixed"})
	for i := 0; i < 12; i++ {
		answered := usage.Record{Time: now.Add(-time.Duration(i) * time.Minute), Agent: "codex", Provider: "mixed", Model: "good-model", Input: 100, Output: 50, Status: 200, TTFT: 300, Millis: 5000}
		usage.Append(answered)
		bad := usage.Record{Time: now.Add(-time.Duration(i) * time.Minute), Agent: "codex", Provider: "mixed", Model: "bad-model", Input: 100, Output: 50, Status: 200, TTFT: 300, Millis: 5000}
		if i < 6 {
			bad.Status, bad.Error, bad.ErrType = 429, "rate limit", "rate_limit_error"
			bad.TTFT, bad.Millis = 0, 0
		}
		usage.Append(bad)
	}
	usage.Append(usage.Record{Time: now, Agent: "codex", Provider: "mixed", Model: "once", Input: 100, Output: 50, Status: 200})
	r := Rank(usage.All, provider.All(), time.Local)
	e := rankEntry(t, r, "mixed")
	if len(e.Models) != 2 {
		t.Fatalf("models: %+v", e.Models)
	}
	if e.Models[0].Model != "good-model" || e.Models[0].Score <= e.Models[1].Score {
		t.Fatalf("the best first: %+v", e.Models)
	}
	for _, m := range e.Models {
		keys := []string{}
		for _, p := range m.Parts {
			keys = append(keys, p.Key)
		}
		if !slices.Equal(keys, []string{"served", "stood", "fast", "soon"}) {
			t.Fatalf("%s's part keys: %v", m.Model, keys)
		}
	}
	// a model's pace is its own served and stood again: the bad model fails
	// half its calls and never retries, and its fast part is that served
	good, bad := rankPartMap(e.Models[0].Parts), rankPartMap(e.Models[1].Parts)
	if good["fast"].Points != good["served"].Points || good["soon"].Points != good["stood"].Points {
		t.Fatalf("the good model's pace: %+v", good)
	}
	if bad["fast"].Points != 0 || bad["fast"].Points != bad["served"].Points {
		t.Fatalf("the bad model's pace: %+v", bad)
	}
	// a model with one call says nothing
	for _, m := range e.Models {
		if m.Model == "once" {
			t.Fatalf("a one-call model is ranked: %+v", m)
		}
	}
	if e.Models[0].Grade == "" || e.Models[1].Grade == "" {
		t.Fatalf("grades: %+v", e.Models)
	}
}

// A provider is named for removal only after long use that is still bad:
// enough days and requests, and over rankDropShare of them failing both over
// the whole period and in its last rankRecentDays days. A provider that was
// bad a month ago and has answered since is not named.
func TestRankDropNeedsLongAndStillBad(t *testing.T) {
	now := rankHome(t)
	all := rankFixtures(t,
		// long, many, and still failing now
		rankCase{id: "drop", name: "Drop", calls: rankDropCalls, failed: rankDropCalls / 2, days: rankDropDays + 1},
		// long and many, but its failures are all a month old
		rankCase{id: "healed", name: "Healed", calls: rankDropCalls, failed: 0, days: rankDropDays + 1},
		// long and many, a bad patch early and clean since
		rankCase{id: "patch", name: "Patch", calls: rankDropCalls, failed: 0, days: rankDropDays + 1},
		// bad, but not long enough
		rankCase{id: "young", name: "Young", calls: rankDropCalls, failed: rankDropCalls / 2, days: 2},
	)
	// the healed and patch ones: their failures before the recent window
	for _, id := range []string{"healed", "patch"} {
		for i := 0; i < rankDropCalls/2; i++ {
			usage.Append(usage.Record{Time: now.Add(-time.Duration(rankRecentDays+3) * 24 * time.Hour), Agent: "codex", Provider: id, Model: "m-" + id, Input: 100, Output: 1, Status: 500, Error: "upstream"})
		}
	}
	r := Rank(usage.All, all, time.Local)
	drops := map[string]bool{}
	for _, id := range r.Drops {
		drops[id] = true
	}
	if !drops["drop"] {
		t.Fatalf("the still-failing one is not named: %+v", r.Drops)
	}
	for _, id := range []string{"healed", "patch", "young"} {
		if drops[id] {
			t.Fatalf("%s is named for removal: %+v", id, r.Drops)
		}
	}
	if e := rankEntry(t, r, "drop"); !e.Drop || e.RecentCalls == 0 || e.Days < rankDropDays {
		t.Fatalf("the drop's detail: %+v", e)
	}
}

// A provider still set up and not hidden can be removed from the tab; one
// the user took off the Providers page is hidden, and a deleted one is kept
// in the ranking but never offered again. A deleted provider with too few
// calls to rank yet still says it is gone.
func TestRankRemovableAndGone(t *testing.T) {
	rankHome(t)
	all := rankFixtures(t,
		rankCase{id: "live", name: "Live", calls: RankMinCalls, days: 1},
		rankCase{id: "quiet", name: "Quiet", calls: RankMinCalls, days: 1, hidden: true},
		rankCase{id: "gone", name: "Gone", calls: RankMinCalls, days: 1, deleted: true},
		rankCase{id: "faint", name: "Faint", calls: 1, days: 1, deleted: true},
	)
	r := Rank(usage.All, all, time.Local)
	if e := rankEntry(t, r, "live"); !e.Removable || e.Removed != nil {
		t.Fatalf("a provider still set up: %+v", e)
	}
	if e := rankEntry(t, r, "quiet"); e.Removable {
		t.Fatalf("a hidden provider is offered for removal: %+v", e)
	}
	gone := rankEntry(t, r, "gone")
	if gone.Removable || gone.Removed == nil {
		t.Fatalf("a deleted provider: %+v", gone)
	}
	if len(r.Gone) != 1 || r.Gone[0].ID != "faint" {
		t.Fatalf("gone: %+v", r.Gone)
	}
}

// The ranking is read in the reader's zone: a provider whose calls cross
// midnight UTC still answers on one day of a UTC+8 reader, and the recent
// window is the last rankRecentDays days of that reader's period.
func TestRankReadsInZone(t *testing.T) {
	rankHome(t)
	saveProvider(t, rankCase{id: "zoned", name: "Zoned"})
	zone := time.FixedZone("UTC+8", 8*60*60)
	// 2026-11-04 23:30 in the reader's zone is 15:30 UTC, the day before
	at := time.Date(2026, 11, 4, 23, 30, 0, 0, zone)
	for i := 0; i < RankMinCalls; i++ {
		usage.Append(usage.Record{Time: at.Add(time.Duration(i) * time.Minute), Agent: "codex", Provider: "zoned", Model: "m", Input: 100, Output: 50, Status: 200})
	}
	r := Rank(usage.All, provider.All(), zone)
	e := rankEntry(t, r, "zoned")
	if e.Days != 1 {
		t.Fatalf("days in the reader's zone: %+v", e)
	}
	if e.First.IsZero() || e.Last.IsZero() {
		t.Fatalf("its first and last: %+v", e)
	}
}

// The ranking is JSON the tab reads, and its words are the page's own: a
// cause's key names a word the page translates, never the vendor's type.
func TestRankJSONWords(t *testing.T) {
	rankHome(t)
	all := rankFixtures(t, rankCase{id: "mixed", name: "Mixed", calls: RankMinCalls * 2, failed: RankMinCalls, days: 1, accounts: []string{"a@example.com", "b@example.com"}})
	r := Rank(usage.All, all, time.Local)
	e := rankEntry(t, r, "mixed")
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range append(rankCauseKeys(e), "rate_limit") {
		if !isRankWord(key) {
			t.Fatalf("a cause's key %q is not one the page says", key)
		}
	}
	for _, p := range e.Parts {
		if !isRankWord(p.Key) {
			t.Fatalf("a part's key %q is not one the page says", p.Key)
		}
	}
	for _, a := range e.Accounts {
		if a == "" {
			t.Fatalf("an empty account: %s", b)
		}
	}
	if len(e.Accounts) != 2 {
		t.Fatalf("accounts: %+v", e.Accounts)
	}
	if r.Tier.Needs != RankMinCalls || r.Tier.Still != rankRecentDays {
		t.Fatalf("tier: %+v", r.Tier)
	}
}

// isRankWord is whether key is one of the ranking's own words, which the
// page translates: none of them is a vendor's type or a word of its body.
func isRankWord(key string) bool {
	switch key {
	case "served", "stood", "fast", "soon",
		"steady", "failing", "slow", "slow-start", "unpriced",
		"rate_limit", "quota", "credit", "auth", "overflow", "loop",
		"restricted", "verify", "refused", "prompt", "cut", "upstream", "failed":
		return true
	}
	return false
}
