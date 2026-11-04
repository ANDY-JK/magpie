package gateway

import (
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// Balanced weighs what the gateway knows of each seat and not only what a
// vendor said of its allowance: a key that has failed since it last answered
// goes behind the ones that have answered since (Seat.Failures), and a key
// whose provider's keys are failing goes behind another provider's
// (Seat.Vendor) — so one that keeps failing is asked again only when the
// others can't answer, and a vendor refusing magpie is not tried over and
// over.
func TestBalancedWeighsFailuresAndVendors(t *testing.T) {
	now := time.Now()
	of := map[string]provider.Allowance{}
	keyWindows(t, of)
	// all alike: the same week half left, renewing together
	for _, k := range []string{"k1", "k2", "k3"} {
		of[k] = week(10, now.Add(6*24*time.Hour))
	}
	defer func() {
		routed.Lock()
		for _, k := range []string{"v1#k1", "v1#k2", "v2#k3"} {
			delete(routed.failures, k)
			delete(routed.used, k)
		}
		routed.Unlock()
	}()
	key := func(providerID, k string) candidate {
		return candidate{p: provider.Provider{ID: providerID, Key: k}, model: "m", rest: providerID + "#" + k}
	}
	// one provider, its second key failing: the first goes first, though the
	// second was sent nothing lately, and the Routing page is told it failed
	p := provider.Provider{ID: "v1", Routing: provider.Balanced}
	cs := []candidate{key("v1", "k2"), key("v1", "k1")}
	routed.Lock()
	routed.failures["v1#k2"] = 2
	routed.used["v1#k1"] = tokenUse{n: 500, at: now}
	routed.Unlock()
	got, wg := weigh(p, cs, "m", provider.Chat)
	if order := restsOf(got); order != "v1#k1 v1#k2 " {
		t.Fatalf("a key that failed: %s, want v1#k1 v1#k2", order)
	}
	if w := weighed(cs[0], p, wg, false, provider.Chat); w.Fails != 2 {
		t.Fatalf("the trace doesn't say it failed: %+v", w)
	}
	if w := weighed(cs[1], p, wg, false, provider.Chat); w.Fails != 0 {
		t.Fatalf("the trace says a key that answered failed: %+v", w)
	}
	// a group of two providers: v1's k2 failed, so v2's key goes first,
	// though v1's k1 was sent less
	g := provider.Provider{ID: provider.GroupPrefix + "g", Routing: provider.Balanced}
	group := []candidate{key("v1", "k1"), key("v1", "k2"), key("v2", "k3")}
	routed.Lock()
	routed.used["v1#k1"] = tokenUse{}
	routed.used["v1#k2"] = tokenUse{}
	routed.used["v2#k3"] = tokenUse{n: 500, at: now}
	routed.Unlock()
	if got := restsOf(route(g, group, "m", provider.Chat)); got != "v2#k3 v1#k1 v1#k2 " {
		t.Fatalf("a vendor failing: %s, want v2#k3 v1#k1 v1#k2", got)
	}
	// no key failing: what was sent decides, and v1's k1 goes first
	routed.Lock()
	delete(routed.failures, "v1#k2")
	routed.Unlock()
	if got := restsOf(route(g, group, "m", provider.Chat)); got != "v1#k1 v1#k2 v2#k3 " {
		t.Fatalf("vendors alike: %s, want v1#k1 v1#k2 v2#k3", got)
	}
}

// Balanced spends the quota that comes back soonest first: points and
// credits, or a day's window, before a plan's week and that before its
// month, so a paid plan is kept for when the rest is out; and one whose
// window is about to renew with much of it left goes first of all, since
// what it has is lost at the reset. The gateway reads each candidate's
// allowance into the class the order goes by, and the Routing page shows it
// (Weighed.Class, Loses).
func TestBalancedSpendsWhatComesBackSoonest(t *testing.T) {
	now := time.Now()
	of := map[string]provider.Allowance{}
	keyWindows(t, of)
	defer func() {
		routed.Lock()
		for _, k := range []string{"bv#credits", "bv#week", "bv#month", "bv#pay"} {
			delete(routed.used, k)
			delete(routed.failures, k)
		}
		routed.Unlock()
	}()
	key := func(k string) candidate {
		return candidate{p: provider.Provider{ID: "bv", Key: k}, model: "m", rest: "bv#" + k}
	}
	// pay is a plain key: no window is read for it, so what spending it
	// costs is unknown and may be money — it goes after the plan's own
	// quota, not before it (#4535)
	cs := []candidate{key("month"), key("week"), key("credits"), key("pay")}
	of["credits"] = provider.Allowance{{Used: 50, Unit: "credits", Of: 600, Amount: 300}} // Qoder's add-on credits
	of["week"] = week(20, now.Add(3*24*time.Hour))                                        // Claude's week
	of["month"] = provider.Allowance{{Used: 20, Span: 30 * 24 * time.Hour, Resets: now.Add(20 * 24 * time.Hour)}}
	p := provider.Provider{ID: "bv", Routing: provider.Balanced}
	got, wg := weigh(p, cs, "m", provider.Chat)
	if order := restsOf(got); order != "bv#credits bv#week bv#month bv#pay " {
		t.Fatalf("by class: %s, want bv#credits bv#week bv#month bv#pay", order)
	}
	if w := weighed(cs[2], p, wg, false, provider.Chat); w.Class != int(provider.QuotaDay) || w.Loses {
		t.Fatalf("the trace of the credits seat: class %d, loses %v", w.Class, w.Loses)
	}
	if w := weighed(cs[0], p, wg, false, provider.Chat); w.Class != int(provider.QuotaMonth) {
		t.Fatalf("the trace of the month seat: class %d", w.Class)
	}
	if w := weighed(cs[3], p, wg, false, provider.Chat); w.Class != int(provider.QuotaUnread) {
		t.Fatalf("the trace of a key nothing was read for: class %d", w.Class)
	}
	// the week is in its last ten hours with a fifth of it unspent: first,
	// though the credits are the cheaper quota
	of["week"] = week(20, now.Add(10*time.Hour))
	got, wg = weigh(p, cs, "m", provider.Chat)
	if order := restsOf(got); order != "bv#week bv#credits bv#month bv#pay " {
		t.Fatalf("about to be lost: %s, want bv#week bv#credits bv#month bv#pay", order)
	}
	if w := weighed(cs[1], p, wg, false, provider.Chat); !w.Loses {
		t.Fatalf("the trace doesn't say the week is about to be lost: %+v", w)
	}
}
