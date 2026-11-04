package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// An account's allowance keeps the windows that can stop it, each with
// when it resets.
func TestAllowanceOf(t *testing.T) {
	now := time.Now()
	week := now.Add(3 * 24 * time.Hour)
	a := allowanceOf([]QuotaWindow{
		{Name: "5 hours", Used: 80, ResetSecs: 2 * 3600, Span: 5 * time.Hour},
		{Name: "7 days", Used: 30, ResetsAt: &week, Span: 7 * 24 * time.Hour},
		{Name: "On-demand", Used: 100, ResetsAt: &week, Aside: true},
	}, now)
	if len(a) != 2 || !a[0].Resets.Equal(now.Add(2*time.Hour)) || !a[1].Resets.Equal(week) {
		t.Fatalf("%+v", a)
	}
	used, renews := a.For("m", now)
	if used != 80 || len(renews) != 2 || !renews[0].Equal(week) {
		t.Fatalf("the fullest decides, the week renews first: %v %v", used, renews)
	}
	if a = allowanceOf([]QuotaWindow{{Used: 10}}, now); !a[0].Resets.IsZero() {
		t.Fatalf("not known: %+v", a)
	}
}

// An account's pace is the share of its week left per hour until it
// renews, by the tightest week that counts the model, whichever is told
// first, and when that one renews; the five hours don't count, a week not
// started or not known runs from now, a reset passed is a full week again,
// a window of no span is a budget, the hours are never fewer than one, and
// the five hours alone go by what they have left per hour until they
// renew — a week not started with their share used where the reading may
// leave the week out (#576).
func TestAllowancePace(t *testing.T) {
	now := time.Now()
	a := Allowance{
		{Used: 95, Resets: now.Add(2 * 24 * time.Hour), Span: 7 * 24 * time.Hour, Model: "opus"},
		{Used: 90, Resets: now.Add(10 * time.Minute), Span: 5 * time.Hour}, // a rate cap, not a budget
		{Used: 40, Resets: now.Add(5 * 24 * time.Hour), Span: 7 * 24 * time.Hour},
	}
	near := func(got, want float64) bool { return got > want*0.99 && got < want*1.01 }
	if p, due := a.Pace("claude-sonnet-5-5", now); !near(p, 60.0/120) || !due.Equal(a[2].Resets) {
		t.Fatalf("Sonnet by the general week: %v %v", p, due)
	}
	if p, due := a.Pace("claude-opus-5-5", now); !near(p, 5.0/48) || !due.Equal(a[0].Resets) {
		t.Fatalf("Opus by its own, the tighter: %v %v", p, due)
	}
	// Opus's own week the looser: the general decides Opus too
	a[0].Used = 10
	if p, _ := a.Pace("claude-opus-5-5", now); !near(p, 60.0/120) {
		t.Fatalf("Opus by the general week, the tighter: %v", p)
	}
	if p, due := (Allowance{{Used: 30, Span: 7 * 24 * time.Hour}}).Pace("m", now); !near(p, 70.0/168) || !due.IsZero() {
		t.Fatalf("not started: its whole span from now, due not known: %v %v", p, due)
	}
	if p, _ := (Allowance{{Used: 90, Resets: now.Add(-time.Minute), Span: 7 * 24 * time.Hour}}).Pace("m", now); !near(p, 100.0/168) {
		t.Fatalf("reset passed: a full week: %v", p)
	}
	if p, _ := (Allowance{{Used: 50, Resets: now.Add(10 * 24 * time.Hour), Span: 30 * 24 * time.Hour}}).Pace("m", now); !near(p, 50.0/240) {
		t.Fatalf("a month is a budget too: %v", p)
	}
	if p, _ := (Allowance{{Used: 20, Resets: now.Add(time.Hour), Span: 5 * time.Hour}}).Pace("m", now); !near(p, 80.0/1) {
		t.Fatalf("five hours alone: what they lose at their reset, per hour: %v", p)
	}
	if p, _ := (Allowance{{Used: 20, Resets: now.Add(time.Hour), Span: 5 * time.Hour, partial: true}}).Pace("m", now); !near(p, 80.0/168) {
		t.Fatalf("five hours alone, the reading partial: a week not started with their share used: %v", p)
	}
	// a plugin may tell no span: the window is a budget for as long as its
	// reset says, else a week
	if p, due := (Allowance{{Used: 20, Resets: now.Add(time.Hour), Span: 5 * time.Hour}, {Used: 60, Resets: now.Add(2 * 24 * time.Hour)}}).Pace("m", now); !near(p, 40.0/48) || due.IsZero() {
		t.Fatalf("no span, a reset: a budget until it: %v %v", p, due)
	}
	if p, _ := (Allowance{{Used: 60}}).Pace("m", now); !near(p, 40.0/168) {
		t.Fatalf("no span, no reset: a week: %v", p)
	}
	// ...however near: how long is left isn't how long it runs
	if p, _ := (Allowance{{Used: 20, Resets: now.Add(90 * time.Minute)}}).Pace("m", now); !near(p, 80.0/1.5) {
		t.Fatalf("no span, ninety minutes: a budget still: %v", p)
	}
	if p, _ := (Allowance{{Used: 50, Resets: now.Add(12 * time.Hour), Span: 24 * time.Hour}}).Pace("m", now); !near(p, 50.0/12) {
		t.Fatalf("a day is the shortest budget: %v", p)
	}
	if p, _ := (Allowance{{Used: 90, Resets: now.Add(30 * time.Second), Span: 7 * 24 * time.Hour}}).Pace("m", now); !near(p, 10) {
		t.Fatalf("thirty seconds to go count as an hour: %v", p)
	}
	if p, _ := (Allowance{{Used: -5, Span: 7 * 24 * time.Hour}}).Pace("m", now); !near(p, 100.0/168) {
		t.Fatalf("used below nothing: nothing: %v", p)
	}
	if p, _ := (Allowance{}).Pace("m", now); !near(p, FreshPace) {
		t.Fatalf("nothing told: fresh: %v", p)
	}
	// a provider and a group keep the routing when saved
	if q := normalize(Provider{ID: "x", Key: "k", Routing: Pace}); q.Routing != Pace {
		t.Fatalf("a provider's pace routing: %q", q.Routing)
	}
}

// Class is which quota spending an account's allowance uses up, by how soon
// it is made good again: the longest window that counts the model decides,
// the month a five-hour window sits in; points and credits that say nothing
// of when they come back, and a window of a day, are spent first, then a
// plan's five hours, its week and its month; a balance with no window is
// money, spent last. A window that says no span goes by when it renews. Its
// window about to renew with a fifth of it unspent, it is about to lose what
// it has: the span's last tenth, or a day for one that says no span.
func TestAllowanceClass(t *testing.T) {
	now := time.Now()
	m := "deepseek-v4.1-flash"
	// OpenCode Go's: five hours, seven days and a month that says no span
	open := Allowance{
		{Used: 0, Span: 5 * time.Hour},
		{Used: 44, Span: 7 * 24 * time.Hour, Resets: now.Add(5 * 24 * time.Hour)},
		{Used: 95, Resets: now.Add(13 * 24 * time.Hour)},
	}
	if c, loses := open.Class(m, now); c != QuotaMonth || loses {
		t.Fatalf("opencode-go's month: %v %v", c, loses)
	}
	// Claude's and Codex's: five hours and a week
	if c, _ := (Allowance{{Span: 5 * time.Hour}, {Span: 7 * 24 * time.Hour, Resets: now.Add(3 * 24 * time.Hour)}}).Class(m, now); c != QuotaWeek {
		t.Fatalf("a week beside its five hours: %v", c)
	}
	// Claude Enterprise's: five hours alone
	if c, _ := (Allowance{{Span: 5 * time.Hour, Used: 10}}).Class(m, now); c != QuotaFive {
		t.Fatalf("five hours alone: %v", c)
	}
	// a day, and a month
	if c, _ := (Allowance{{Span: 24 * time.Hour, Used: 0}}).Class(m, now); c != QuotaDay {
		t.Fatalf("a day: %v", c)
	}
	if c, _ := (Allowance{{Span: 30 * 24 * time.Hour, Used: 5}}).Class(m, now); c != QuotaMonth {
		t.Fatalf("thirty days: %v", c)
	}
	// points and credits that say nothing of when: Qoder's add-on credits,
	// CodeArts' points with no reset
	if c, _ := (Allowance{{Used: 66, Unit: "credits", Of: 600}}).Class(m, now); c != QuotaDay {
		t.Fatalf("add-on credits: %v", c)
	}
	// CodeArts' points, whose reset says when and not how long
	if c, _ := (Allowance{{Used: 100, Unit: "credits", Of: 9600, Resets: now.Add(19 * 24 * time.Hour)}}).Class(m, now); c != QuotaMonth {
		t.Fatalf("points renewing in 19 days: %v", c)
	}
	if c, _ := (Allowance{{Used: 40, Resets: now.Add(6 * 24 * time.Hour)}}).Class(m, now); c != QuotaWeek {
		t.Fatalf("no span, renewing in six days: %v", c)
	}
	if c, _ := (Allowance{{Used: 40, Resets: now.Add(5 * time.Hour)}}).Class(m, now); c != QuotaFive {
		t.Fatalf("no span, renewing in five hours: %v", c)
	}
	// MiniMax's credits, which expire and hold a balance too: the window
	// says, not the money
	left := 142.32
	if c, _ := (Allowance{{Used: 94, Of: 2600, Unit: "credits", RunsOut: now.Add(29 * 24 * time.Hour), Balance: &left}}).Class(m, now); c != QuotaDay {
		t.Fatalf("credits that expire: %v", c)
	}
	// only a balance: money, spent last
	if c, loses := (Allowance{{Balance: &left, Unit: "credits"}}).Class(m, now); c != QuotaWallet || loses {
		t.Fatalf("a wallet: %v %v", c, loses)
	}
	if c, _ := (Allowance{}).Class(m, now); c != QuotaDay {
		t.Fatalf("nothing told: %v", c)
	}
	// a window for another model doesn't count
	if c, _ := (Allowance{{Model: "opus", Span: 30 * 24 * time.Hour}, {Span: 5 * time.Hour}}).Class("claude-sonnet-5", now); c != QuotaFive {
		t.Fatalf("another model's month: %v", c)
	}
	// a week in its last hours with much of it left is about to be lost
	if c, loses := (Allowance{{Used: 40, Span: 7 * 24 * time.Hour, Resets: now.Add(10 * time.Hour)}}).Class(m, now); c != QuotaWeek || !loses {
		t.Fatalf("a week ending 60%% unspent: %v %v", c, loses)
	}
	if _, loses := (Allowance{{Used: 90, Span: 7 * 24 * time.Hour, Resets: now.Add(10 * time.Hour)}}).Class(m, now); loses {
		t.Fatalf("a week ending 10%% unspent isn't lost, it is spent")
	}
	if _, loses := (Allowance{{Used: 40, Span: 7 * 24 * time.Hour, Resets: now.Add(3 * 24 * time.Hour)}}).Class(m, now); loses {
		t.Fatalf("a week with three days to go isn't about to be lost")
	}
	// the five hours' last half hour: they come back within the hour, so a
	// plan's five-hour cap isn't spent first for
	if _, loses := (Allowance{{Used: 30, Span: 5 * time.Hour, Resets: now.Add(20 * time.Minute)}}).Class(m, now); loses {
		t.Fatalf("five hours renew in twenty minutes")
	}
	// a day renews tomorrow: spending what is left is the next day's job
	if _, loses := (Allowance{{Used: 10, Span: 24 * time.Hour, Resets: now.Add(time.Hour)}}).Class(m, now); loses {
		t.Fatalf("a day about to renew isn't spent first")
	}
	// one that says no span: a day before its reset
	if _, loses := (Allowance{{Used: 40, Resets: now.Add(12 * time.Hour)}}).Class(m, now); !loses {
		t.Fatalf("a month renewing tomorrow 60%% unspent is lost")
	}
	if _, loses := (Allowance{{Used: 90, Resets: now.Add(12 * time.Hour)}}).Class(m, now); loses {
		t.Fatalf("a month renewing tomorrow 10%% unspent isn't")
	}
	if _, loses := (Allowance{{Used: 40, Resets: now.Add(4 * 24 * time.Hour)}}).Class(m, now); loses {
		t.Fatalf("one renewing in four days isn't about to be lost")
	}
	// an empty window, its reset passed: nothing used, so more is at stake
	if _, loses := (Allowance{{Used: 100, Span: 7 * 24 * time.Hour, Resets: now.Add(-time.Minute)}}).Class(m, now); loses {
		t.Fatalf("a window whose reset passed says nothing of when it runs again")
	}
}

// A window counts only the models it is for, and one whose reset has
// passed is empty again.
func TestAllowanceFor(t *testing.T) {
	now := time.Now()
	a := Allowance{
		{Used: 20, Resets: now.Add(2 * time.Hour), Span: 5 * time.Hour},
		{Used: 40, Resets: now.Add(4 * 24 * time.Hour), Span: 7 * 24 * time.Hour},
		{Used: 100, Resets: now.Add(2 * 24 * time.Hour), Span: 7 * 24 * time.Hour, Model: "opus"},
	}
	if used, renews := a.For("claude-sonnet-5", now); used != 40 || len(renews) != 2 {
		t.Fatalf("sonnet: %v %v", used, renews)
	}
	if used, renews := a.For("claude-opus-5-5", now); used != 100 || len(renews) != 3 || !renews[0].Equal(a[1].Resets) {
		t.Fatalf("opus: %v %v", used, renews)
	}
	if got := a.Full("claude-opus-5-5", 98, now); !got.Equal(a[2].Resets) {
		t.Fatalf("opus full until %v", got)
	}
	if got := a.Full("claude-sonnet-5", 98, now); !got.IsZero() {
		t.Fatalf("sonnet isn't full: %v", got)
	}
	a[0].Used, a[0].Resets = 100, now.Add(-time.Minute)
	if used, renews := a.For("claude-sonnet-5", now); used != 40 || !renews[1].IsZero() {
		t.Fatalf("renewed five hours: %v %v", used, renews)
	}
}

// The first ask for an agent's allowances waits for them, so the first
// requests after magpie starts are routed by them too.
func TestAllowancesFirstWaits(t *testing.T) {
	if m := Allowances("nobody"); m == nil {
		t.Fatal("the first ask didn't wait")
	}
}

// An account that says it is out while its allowance is being read has it
// read again once that reading is back, not a minute later: the reading
// was asked before the refusal, and what it tells is kept only until the
// next ask, which reads again (as StaleKeyAllowance does a key's).
func TestAllowancesStaleMidRead(t *testing.T) {
	signIn(t)
	week := time.Now().Add(72 * time.Hour)
	release := make(chan struct{})
	var once sync.Once
	var readings atomic.Int32
	LoginUsageVia(func(_ context.Context, agent string) map[string]SubscriptionQuota {
		used := 100.0
		if readings.Add(1) == 1 {
			<-release
			used = 97
		}
		return map[string]SubscriptionQuota{"me@example.com": {Windows: []QuotaWindow{{Span: 7 * 24 * time.Hour, Used: used, ResetsAt: &week}}}}
	})
	reset := func() {
		usedCache.Lock()
		usedCache.m, usedCache.at, usedCache.loading = map[string]map[string]Allowance{}, map[string]time.Time{}, map[string]chan struct{}{}
		usedCache.stale = nil
		usedCache.Unlock()
	}
	reset()
	t.Cleanup(func() {
		once.Do(func() { close(release) })
		// every reading this started lands before the stand-in goes
		for {
			usedCache.Lock()
			done := usedCache.loading["codex"]
			usedCache.Unlock()
			if done == nil {
				break
			}
			<-done
		}
		LoginUsageVia(nil)
		reset()
	})
	// read over a minute ago, at 90%
	usedCache.Lock()
	usedCache.m["codex"] = map[string]Allowance{"me@example.com": {{Used: 90, Resets: week, Span: 7 * 24 * time.Hour}}}
	usedCache.at["codex"] = time.Now().Add(-2 * time.Minute)
	usedCache.Unlock()
	share := func() float64 {
		u, _ := Allowances("codex")["me@example.com"].For("gpt-5.5", time.Now())
		return u
	}
	share() // reads again, behind the request
	usedCache.Lock()
	reading := usedCache.loading["codex"]
	usedCache.Unlock()
	for readings.Load() < 1 {
		time.Sleep(5 * time.Millisecond)
	}
	StaleAllowance("codex", "me@example.com") // refused: out, as the request found
	once.Do(func() { close(release) })
	<-reading
	first := share()
	got, deadline := first, time.Now().Add(2*time.Second)
	for got != 100 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		got = share()
	}
	if got != 100 {
		t.Fatalf("first ask %v%%, 2s later %v%%, readings %d", first, got, readings.Load())
	}
	if n := readings.Load(); n != 2 {
		t.Fatalf("readings %d", n)
	}
}

// Copilot's allowances renew on the day GitHub says, and its code
// completions don't stop an account from answering.
func TestCopilotUsageResets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"copilot_plan":"individual","quota_reset_date":"2026-10-01","quota_reset_date_utc":"2026-10-01T00:00:00.000Z",
			"quota_snapshots":{"completions":{"has_quota":true,"entitlement":2000,"quota_remaining":0},
			"premium_interactions":{"has_quota":true,"entitlement":300,"quota_remaining":75}}}`))
	}))
	defer srv.Close()
	old := CopilotUserURL
	CopilotUserURL = srv.URL
	defer func() { CopilotUserURL = old }()
	q := copilotSubscriptionUsage(t.Context(), "tok", "")
	if q.Error != "" || len(q.Windows) != 2 || q.Windows[1].ResetsAt == nil || q.Windows[1].ResetsAt.Month() != time.October {
		t.Fatalf("%+v", q)
	}
	now := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	if used, renews := allowanceOf(q.Windows, now).For("gpt-5", now); used != 75 || len(renews) != 1 {
		t.Fatalf("completions counted: %v %v", used, renews)
	}
}
