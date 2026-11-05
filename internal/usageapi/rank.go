package usageapi

// The ranking: how each provider's models served the requests of a period,
// told apart by what went wrong and how long things took, so a provider that
// keeps failing, keeps being refused and keeps waiting can be told from one
// that only looks that way over a few calls. Read from the same log the
// Usage page reads (the ledger's rows, split by provider and by model at
// each provider in usage.Breakdown), so the numbers agree with the page's
// own; a provider deleted since is named as it was then, and only one still
// set up can be removed from here.
//
// The four parts of a provider's score, each out of 25:
//   - served: its requests the agent was not answered an error for, full at
//     100%, none at a quarter of them failing
//   - stood: how often a request was answered only after a try failed first,
//     full at none, none at a tenth of them
//   - fast: its timed replies' speed against the period's fastest provider
//     with enough replies, full at its speed, none at a tenth of it
//   - soon: how long they waited for the first token, the same way
//
// A provider with too few calls to say anything is left out of the ranking
// (Thin), but one whose every call was a try handed on to another provider
// is named even so: its refusals are what made those requests retry. One
// whose replies no session file timed keeps its score out of the two parts
// that can't be told, which are folded into the two that can. Failure kinds
// are only hints, keyed by what the log kept; the page says what it reads
// them with rather than claim an account's rest or a vendor's policy.

import (
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/usage"
)

const (
	// RankMinCalls is how many of a provider's own requests a period must
	// hold before its score is ranked: five is not a sample, and one of them
	// failing would read as a provider that serves nothing. A provider with
	// fewer is Thin unless its only rows were tries handed on, whose causes
	// are still worth naming.
	RankMinCalls = 5

	// RankMinTimed is how many timed replies a provider needs for its speed
	// and wait to say anything, and for its speed to stand as the period's
	// fastest: one fast reply says little, and a relay's single reply read
	// at 1,000 tok/s would outrank every provider that answers for a living.
	RankMinTimed = 5

	// RankFastShare is how much of the fastest provider's speed fills a
	// provider's speed points: a tenth of it gets none, its own speed or
	// more gets all.
	RankFastShare = 10

	// A provider is named for removal when it has been used on at least
	// rankDropDays different days, has served rankDropCalls requests, and
	// over rankDropShare of them failed both over the whole period and in
	// its last rankRecentDays days: used for a long time, and still bad now
	// rather than only during a bad patch weeks ago (the 11-05 ask).
	rankDropDays    = 7
	rankDropCalls   = 100
	rankDropShare   = 0.2
	rankRecentDays  = 7
	rankSteadyCalls = 20 // how many requests a "steady" tag needs
)

// RankTier is what the period holds a provider to, told on the tab so its
// numbers can be judged rather than taken on faith.
type RankTier struct {
	// Needs is how many requests a period needs of a provider for its score
	// to say anything, and Days, Calls, Errors and Still what a drop is held
	// to: days of use, requests, the share of them failing both over the
	// period and in its last Still days.
	Needs  int     `json:"needs"`
	Days   int     `json:"days"`
	Calls  int     `json:"calls"`
	Errors float64 `json:"errors"`
	Still  int     `json:"still"`
}

// RankCause is one kind of failure a provider's rows were answered with,
// and how many of them there were. A row's own failure is judged from the
// vendor's type (rate_limit_error), the words its body said, or one of
// magpie's own (a reply ended for looping, a prompt turned away). A handed-
// on try's words say what its own provider refused, though the request went
// on to another; words none of those read are "failed" alone, which is all
// the record says. These are hints from the log, not live rests.
type RankCause struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// RankPart is one part of a provider's score: the points it got and the
// most there were. Its key names a word the page says in the reader's own
// language, so none of the page's words comes from the log.
type RankPart struct {
	Key    string  `json:"key"` // served, stood, fast, soon
	Points float64 `json:"points"`
	Most   float64 `json:"most"`
}

// RankTag is a word the ranking puts on a provider: what its period says of
// it, beside its score.
type RankTag struct {
	Key   string  `json:"key"`
	Tone  string  `json:"tone"` // good, warn, info
	Value float64 `json:"value,omitempty"`
}

// RankModel is one model's part of its provider's period: its own score,
// what it served, and what it failed with. A provider's models are ranked on
// their own, so a provider whose one model is bad can be told from one whose
// every model is.
type RankModel struct {
	Model string `json:"model"`
	// At is the provider the model's calls are shown under now, which can
	// be another after a merge (usage.Record.renameBy)
	At    string `json:"at"`
	Score int    `json:"score"`
	Grade string `json:"grade"`
	usage.Totals
	Causes []RankCause `json:"causes,omitempty"`
	Parts  []RankPart  `json:"parts,omitempty"`
}

// RankEntry is one provider's place in the ranking.
type RankEntry struct {
	Named
	Rank  int    `json:"rank"`
	Score int    `json:"score"`
	Grade string `json:"grade"` // A to E
	usage.Totals
	// Days is how many different calendar days it answered on, First and
	// Last when it first and last did: how long it has been used
	Days  int       `json:"days,omitempty"`
	First time.Time `json:"first,omitempty"`
	Last  time.Time `json:"last,omitempty"`
	// RecentCalls and RecentErrors are its last rankRecentDays days: whether
	// it is still bad now, not only over the whole period
	RecentCalls  int `json:"recentCalls,omitempty"`
	RecentErrors int `json:"recentErrors,omitempty"`
	// Causes are the kinds of its failures, the most first: what to look at
	// first when a provider reads badly
	Causes []RankCause `json:"causes,omitempty"`
	Parts  []RankPart  `json:"parts,omitempty"`
	Tags   []RankTag   `json:"tags,omitempty"`
	// Models are its models' own places, the best score first, then the
	// most calls. None when the period holds fewer than RankMinCalls of them
	Models []RankModel `json:"models,omitempty"`
	// Accounts are the subscription accounts that answered its calls, by
	// name, for a provider whose one account is the bad one
	Accounts []string `json:"accounts,omitempty"`
	// Removable is whether the page can remove it from here: a provider
	// still set up and not hidden. One with accounts is hidden from the
	// Providers page rather than deleted (provider.Delete), and a deleted
	// one is kept in the ranking but never offered again
	Removable bool `json:"removable,omitempty"`
	// Removed is when the provider was deleted, for one whose calls are
	// still in the log (Named.Deleted)
	Removed *time.Time `json:"removed,omitempty"`
	// Drop is whether it has been used for a long time and is still bad:
	// the tab names these first
	Drop bool `json:"drop,omitempty"`
}

// Ranking is what the ranking tab shows of a period.
type Ranking struct {
	Period usage.Period `json:"period"`
	Since  time.Time    `json:"since"`
	// Tier is what the period holds a provider to
	Tier RankTier `json:"tier"`
	// Drops are the providers whose standing failure puts them in the last
	// place, named for the tab to say what to do with them
	Drops []string `json:"drops,omitempty"`
	// Providers are ranked by score, best first, and those with fewer than
	// Tier.Needs calls are left in Thin (Gone among them are the deleted),
	// except a provider whose every call was a handed-on try: its own
	// failures are still named. Drops are the ones whose standing failure
	// puts them in the last place.
	Providers []RankEntry `json:"providers"`
	Thin      []Named     `json:"thin,omitempty"`
	Gone      []Named     `json:"gone,omitempty"`
}

// Rank is the ranking of a period, read in zone: each provider's score, what
// it failed with, and its models' own. all is the providers set up now, for
// their names and which can be removed.
func Rank(p usage.Period, all []provider.Provider, zone *time.Location) Ranking {
	if !p.Known() {
		p = usage.Month
	}
	if zone == nil {
		zone = time.Local
	}
	now := usage.Clock().In(zone)
	f := usage.Filter{Zone: zone, Through: usage.SourceGateway}
	rows := rankRows(usage.LedgerOfAt(p, f, now).Rows)
	shares := usage.Breakdown(rows, "provider")
	modelShares := usage.Breakdown(rows, "modelAt")
	fast, soon := rankPace(shares)
	tier := RankTier{Needs: RankMinCalls, Days: rankDropDays, Calls: rankDropCalls, Errors: rankDropShare, Still: rankRecentDays}
	out := Ranking{Period: p, Since: p.Since(now), Tier: tier, Providers: []RankEntry{}}
	set := map[string]provider.Provider{}
	for _, pr := range all {
		set[pr.ID] = pr
	}
	which := providerFaces(all)
	by := rankDetails(rows, now, p, zone)
	for _, s := range shares {
		face := which(s.ID)
		d := by[s.ID]
		if s.Calls < RankMinCalls && len(d.causes) == 0 {
			out.Thin = append(out.Thin, face)
			if face.Deleted != nil {
				out.Gone = append(out.Gone, face)
			}
			continue
		}
		e := RankEntry{Named: face, Totals: s.Totals, Accounts: rankAccounts(d.accounts), Causes: rankCauses(d.causes)}
		e.Removed = face.Deleted
		if pr, ok := set[s.ID]; ok && !pr.Hidden && face.Deleted == nil {
			e.Removable = true
		}
		e.Days, e.First, e.Last = d.days, d.first, d.last
		e.RecentCalls, e.RecentErrors = d.recentCalls, d.recentErrors
		e.Parts = rankParts(s.Totals, fast, soon, true)
		for _, part := range e.Parts {
			e.Score += int(math.Round(part.Points))
		}
		e.Grade = rankGrade(e.Score)
		e.Tags = rankTags(s.Totals, e.Causes)
		e.Models = rankModels(modelShares, s.ID, by, fast, soon)
		e.Drop = rankDrop(e)
		out.Providers = append(out.Providers, e)
		if e.Drop {
			out.Drops = append(out.Drops, e.ID)
		}
	}
	rankSort(out.Providers)
	for i := range out.Providers {
		out.Providers[i].Rank = i + 1
	}
	return out
}

// rankRows are the gateway's own calls: what a provider served. A call read
// from an agent's session file names no supplier; one a client reported went
// straight to its vendor; a rejected one never went out; a probe is magpie's
// own test. A try handed on is kept for what it billed, and is a try of the
// provider that failed, not a request of its own (usage.Totals.count).
func rankRows(rows []usage.Row) []usage.Row {
	out := make([]usage.Row, 0, len(rows))
	for _, r := range rows {
		if r.Source != "" || r.IsRejected() || r.IsProbe() || r.Provider == usage.UnknownProvider {
			continue
		}
		out = append(out, r)
	}
	return out
}

// rankDetail is what a provider's rows say besides their totals: the days
// it answered on, its failures by kind, its accounts and whether its recent
// calls were as bad as its whole period.
type rankDetail struct {
	days         int
	first, last  time.Time
	seen         map[string]bool
	recentCalls  int
	recentErrors int
	causes       map[string]int
	accounts     map[string]int
}

// rankDetails counts each provider's and each model's own detail. recent is
// the last rankRecentDays days of the period's end (now for a preset), and a
// row's day is the reader's, so a provider that was bad a month ago and has
// answered since is not "still bad".
func rankDetails(rows []usage.Row, now time.Time, p usage.Period, zone *time.Location) map[string]*rankDetail {
	end := now
	if until := p.Until(now); !until.IsZero() {
		end = until
	}
	recent := end.Add(-rankRecentDays * 24 * time.Hour)
	at := map[string]*rankDetail{}
	for _, r := range rows {
		for _, key := range []string{r.Provider, usage.ModelAtKey(r.Provider, r.Model)} {
			d := at[key]
			if d == nil {
				d = &rankDetail{causes: map[string]int{}, accounts: map[string]int{}}
				at[key] = d
			}
			d.note(r, recent, zone, key == r.Provider)
		}
	}
	for _, d := range at {
		d.days = len(d.seen)
		d.seen = nil
	}
	return at
}

// note counts one row in a provider's or a model's own detail. A try handed
// on is a failure of the provider that failed, but not a request of its own:
// its words tell a cause, its time a day of use, and neither counts as a
// call. Accounts and recent figures are a provider's, not a model's, so the
// model's rows stay out of them.
func (d *rankDetail) note(r usage.Row, recent time.Time, zone *time.Location, whole bool) {
	if d.first.IsZero() || r.Time.Before(d.first) {
		d.first = r.Time
	}
	if d.last.IsZero() || r.Time.After(d.last) {
		d.last = r.Time
	}
	if d.seen == nil {
		d.seen = map[string]bool{}
	}
	d.seen[r.Time.In(zone).Format(time.DateOnly)] = true
	if r.Failed() {
		if kind := rankCause(r.Status, r.ErrType, r.Error); kind != "" {
			d.causes[kind]++
		}
	}
	if !whole || r.HandedOn {
		return
	}
	if a := r.Account(); a != "" {
		d.accounts[provider.AccountShownOf(r.Provider, a)]++
	}
	if !r.Time.Before(recent) {
		d.recentCalls++
		if r.Failed() {
			d.recentErrors++
		}
	}
}

// rankPace is the fastest provider's speed and the shortest wait of the
// period, each of the providers with RankMinTimed timed replies: what a
// provider's own speed and wait are held against. A single fast reply does
// not stand as the period's fastest.
func rankPace(shares []usage.Share) (fast, soon float64) {
	for _, s := range shares {
		if s.Timed < RankMinTimed {
			continue
		}
		if v := s.Speed(); v > fast {
			fast = v
		}
		if w := float64(s.MeanTTFT()); w > 0 && (soon == 0 || w < soon) {
			soon = w
		}
	}
	return fast, soon
}

// rankParts scores a provider's period out of 100, in four parts of 25 each.
func rankParts(t usage.Totals, fast, soon float64, top bool) []RankPart {
	errRate := float64(t.Errors) / float64(max(1, t.Calls))
	served := clamp01(1 - errRate/0.25)
	tried := float64(t.Retried) / float64(max(1, t.Calls))
	stood := clamp01(1 - tried/0.1)
	parts := []RankPart{
		{Key: "served", Points: math.Round(served * 25), Most: 25},
		{Key: "stood", Points: math.Round(stood * 25), Most: 25},
	}
	if t.Timed < RankMinTimed {
		// nothing timed: the two parts that can't be told are folded into
		// the two that can, so a provider whose replies no session file
		// times is not marked down for it (#3464's lesson: a vendor that
		// counts no cache is not one that misses it)
		parts[0].Most, parts[0].Points = 50, math.Round(served*50)
		parts[1].Most, parts[1].Points = 50, math.Round(stood*50)
		return parts
	}
	if !top {
		// a model's pace is not held against the period's fastest, which is
		// a provider's: a model of the fastest provider would score nothing
		// for being the same, and a model of a slower one would have to
		// catch a number no provider of its own stands to reach. A model's
		// own pace is told by the numbers beside its name.
		return append(parts,
			RankPart{Key: "fast", Points: math.Round(served * 25), Most: 25},
			RankPart{Key: "soon", Points: math.Round(stood * 25), Most: 25})
	}
	fastPart := 0.0
	if fast > 0 {
		switch speed := t.Speed(); {
		case speed >= fast:
			fastPart = 1
		case speed >= fast/RankFastShare:
			fastPart = clamp01(speed / fast)
		}
	}
	wait := 0.0
	if soon > 0 {
		switch w := float64(t.MeanTTFT()); {
		case w <= soon:
			wait = 1
		case w <= soon*3:
			wait = clamp01(soon / w * 3)
		}
	}
	return append(parts,
		RankPart{Key: "fast", Points: math.Round(fastPart * 25), Most: 25},
		RankPart{Key: "soon", Points: math.Round(wait * 25), Most: 25})
}

// rankGrade is a provider's score as a letter, for a narrow column: A for
// 85 and over, B for 70, C for 55, D for 40, E below.
func rankGrade(score int) string {
	switch {
	case score >= 85:
		return "A"
	case score >= 70:
		return "B"
	case score >= 55:
		return "C"
	case score >= 40:
		return "D"
	}
	return "E"
}

// rankTags says what a provider's period is worth a word beside its score.
func rankTags(t usage.Totals, causes []RankCause) []RankTag {
	var tags []RankTag
	errRate := float64(t.Errors) / float64(max(1, t.Calls))
	if t.Calls >= rankSteadyCalls {
		switch {
		case errRate <= 0.01:
			tags = append(tags, RankTag{Key: "steady", Tone: "good", Value: errRate})
		case errRate >= rankDropShare:
			tags = append(tags, RankTag{Key: "failing", Tone: "warn", Value: errRate})
		}
	}
	if t.Timed >= RankMinTimed {
		switch v := t.Speed(); {
		case v >= 100:
			tags = append(tags, RankTag{Key: "fast", Tone: "good", Value: v})
		case v > 0 && v < 20:
			tags = append(tags, RankTag{Key: "slow", Tone: "warn", Value: v})
		}
		if w := t.MeanTTFT(); w >= 10000 {
			tags = append(tags, RankTag{Key: "slow-start", Tone: "warn", Value: float64(w)})
		}
	}
	if len(causes) > 0 && causes[0].Key != "failed" {
		tags = append(tags, RankTag{Key: causes[0].Key, Tone: "warn", Value: float64(causes[0].Count)})
	}
	if t.Unpriced > 0 && t.Unpriced == t.Calls {
		tags = append(tags, RankTag{Key: "unpriced", Tone: "info"})
	}
	return tags
}

// rankDrop is whether a provider has been used for a long time and is still
// bad: enough days and requests, and over rankDropShare of them failing both
// over the whole period and in its last rankRecentDays days. A provider that
// was bad a month ago and has answered since is not named.
func rankDrop(e RankEntry) bool {
	if e.Days < rankDropDays || e.Calls < rankDropCalls || e.RecentCalls < rankDropCalls/10 {
		return false
	}
	return float64(e.Errors)/float64(max(1, e.Calls)) >= rankDropShare &&
		float64(e.RecentErrors)/float64(max(1, e.RecentCalls)) >= rankDropShare
}

// rankModels ranks one provider's models, the best score first and then the
// most calls. A model with fewer than RankMinCalls is left out: one call
// says nothing, and a provider with one call per model would be a list of
// ones.
func rankModels(shares []usage.Share, id string, by map[string]*rankDetail, fast, soon float64) []RankModel {
	out := []RankModel{}
	prefix := id + "/"
	for _, s := range shares {
		model, ok := strings.CutPrefix(s.ID, prefix)
		if !ok || s.Calls < RankMinCalls {
			continue
		}
		m := RankModel{Model: model, At: id, Totals: s.Totals}
		if d := by[s.ID]; d != nil {
			m.Causes = rankCauses(d.causes)
		}
		m.Parts = rankParts(s.Totals, fast, soon, false)
		for _, part := range m.Parts {
			m.Score += int(math.Round(part.Points))
		}
		m.Grade = rankGrade(m.Score)
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Calls > out[j].Calls
	})
	return out
}

// rankAccounts names the subscription accounts that answered a provider's
// calls, the most calls first: a provider whose one account is the bad one
// is told by it. None when no record names an account.
func rankAccounts(at map[string]int) []string {
	if len(at) == 0 {
		return nil
	}
	out := make([]string, 0, len(at))
	for a := range at {
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if at[out[i]] != at[out[j]] {
			return at[out[i]] > at[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

// rankSort puts the best score first, then the most calls, then by name: a
// stable order for a page that is read again while the numbers move.
func rankSort(entries []RankEntry) {
	slices.SortStableFunc(entries, func(a, b RankEntry) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		if a.Calls != b.Calls {
			return b.Calls - a.Calls
		}
		return strings.Compare(a.Name, b.Name)
	})
}

// rankCauseWords are the words a failure's record is read by: a vendor's own
// type (the log's ErrType) first, then the words its body said (the log's
// Error), then magpie's own. Only what the log keeps, so a ranking read years
// later says what it said when it was read.
var rankCauseWords = []struct {
	Key   string
	Words []string
}{
	// a vendor's own type, which the log keeps as ErrType (a numeric error
	// code names that code, not a quota word hidden in a vendor's message)
	{"rate_limit", []string{"rate_limit_error", "rate_limit_exceeded", "too_many_requests", "resource_exhausted", "throttled", "slow_down"}},
	{"quota", []string{"usage_limit_reached", "insufficient_quota", "quota_exceeded", "limit_reached"}},
	{"credit", []string{"insufficient_balance", "insufficient_credit", "billing_hard_limit_exceeded", "account_deactivated"}},
	{"auth", []string{"authentication_error", "invalid_api_key", "unauthorized", "permission_denied", "expired_token"}},
	{"overflow", []string{"context_length_exceeded", "request_too_large", "too_many_tokens"}},
	{"loop", []string{usage.LoopErrType}},
	// a vendor's words, which the log keeps as Error
	{"quota", []string{"quota", "usage limit", "usage_limit", "out of budget", "limit reached", "额度", "用量", "套餐", "上限"}},
	{"credit", []string{"insufficient balance", "insufficient credit", "credits exhausted", "余额", "欠费", "请充值"}},
	{"rate_limit", []string{"rate limit", "frequency limit", "too many requests", "per minute", "per second", "rpm", "tpm", "频率", "太频繁", "速率"}},
	{"auth", []string{"invalid api key", "api key not valid", "unauthorized", "authentication", "access token", "登录", "鉴权"}},
	{"restricted", []string{provider.WBRestrictedHint, provider.ZCodeRestrictedHint, "已被限制", "存在异常行为"}},
	{"verify", []string{"validation_required", "verify your account", "验证"}},
	{"refused", []string{"content policy", "safety system", "blocked by", "违规"}},
	{"overflow", []string{"context length", "context_length", "too long", "exceeds the maximum", "token limit", "超出", "过长"}},
	{"prompt", []string{"prompt_turned_away", "unapproved channel", "system prompt"}},
	{"loop", []string{"stuck in a loop"}},
	{"cut", []string{"cut short", "native tool call has no name"}},
}

// rankCause says what kind of failure a record is, "" when it is none. The
// record keeps a vendor's own error type (or numeric code, which is not a
// word to match) and the words its body said; a word of ErrType wins over a
// word of the body, so "rate_limit_error: your quota is fine" is a rate
// limit and not a spent quota.
func rankCause(status int, errType, msg string) string {
	if status < 400 && msg == "" && errType == "" {
		return ""
	}
	t := strings.ToLower(strings.TrimSpace(errType))
	m := strings.ToLower(msg)
	if !rankNumeric(t) {
		for _, c := range rankCauseWords {
			if t != "" && rankWordIn(t, c.Words) {
				return c.Key
			}
		}
	}
	for _, c := range rankCauseWords {
		if rankWordIn(m, c.Words) {
			return c.Key
		}
	}
	if status == 403 && (strings.Contains(m, "restricted") || strings.Contains(m, "限制")) {
		return "restricted"
	}
	if status >= 500 {
		return "upstream"
	}
	return "failed"
}

// rankNumeric is whether s is a number, as a vendor's error code is.
func rankNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// rankWordIn is whether any of words is in s.
func rankWordIn(s string, words []string) bool {
	for _, w := range words {
		if w != "" && strings.Contains(s, strings.ToLower(w)) {
			return true
		}
	}
	return false
}

// rankCauses tells a group's failures apart, the most first.
func rankCauses(counts map[string]int) []RankCause {
	if len(counts) == 0 {
		return nil
	}
	out := []RankCause{}
	for k, n := range counts {
		if n > 0 {
			out = append(out, RankCause{Key: k, Count: n})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// clamp01 is v held between 0 and 1.
func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}
