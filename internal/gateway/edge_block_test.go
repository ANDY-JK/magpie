package gateway

import (
	"net/http"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// zcodeBlocked is the answer magpie met at 10:21 on 11-07 for a ZCode gift
// account: the ZCode plugin's words for zcode.z.ai's edge firewall having
// blocked this computer's address, the same for every account of it.
var zcodeBlocked = []byte(`{"type":"error","error":{"type":"permission_error","message":"405 Method Not Allowed — the provider's network firewall blocked requests from this IP; wait a while, or switch to another network or proxy (ZCode's Start Plan is not asked again from here until 10:40:22, plugins#649)"}}`)

// blockedAccount is one account of zplug, a provider with more than one,
// as a candidate.
func blockedAccount(user string) candidate {
	return candidate{
		p:     provider.Provider{ID: "zplug", Chat: "http://127.0.0.1:1/v1", Account: &provider.Account{Agent: "plugin", User: user}},
		model: "GLM-5.3-Flash",
		rest:  "zplug#" + user,
	}
}

// A firewall's block of this computer's address is the provider's, not the
// account's that happened to be asked: every key and account of it goes
// out from that address. So one block rests the provider as a whole for a
// few minutes, climbs no account's ladder, and leaves nothing resting once
// it lifts. Before, each 405 rested the account through the failOther
// ladder: on 11-07 zcode's accounts took 405s one after another from 10:21
// and two of them rested six minutes, every request asking them all again.
func TestEdgeBlockRestsWholeProvider(t *testing.T) {
	fresh(t)
	s := New()
	a, b := blockedAccount("one"), blockedAccount("two")
	r := s.restAfter(a, http.StatusMethodNotAllowed, http.Header{}, zcodeBlocked)
	if r.By != "blocked" || r.Failures != 0 || r.Key != "zplug*" {
		t.Fatalf("the block's rest: %+v, want by blocked, no failures, resting as the provider", r)
	}
	if left := time.Until(r.Until); left < edgeRest-time.Minute || left > edgeRest+time.Second {
		t.Fatalf("the block rests %v, want about %v", left.Round(time.Second), edgeRest)
	}
	// the provider's other account is held by it too, wherever it is asked
	if _, ok := restOfAny(b); !ok {
		t.Fatal("the provider's other account is asked while the address is blocked")
	}
	if !resting(b) {
		t.Fatal("a blocked provider's account is planned as ready")
	}
	// refused again, for the other account: the same rest, no ladder, and
	// nothing counted against either account
	r2 := s.restAfter(b, http.StatusMethodNotAllowed, http.Header{}, zcodeBlocked)
	if r2.By != "blocked" || r2.Failures != 0 || r2.Until.Sub(r.Until).Abs() > time.Second {
		t.Fatalf("the second account's block: %+v, want the first's rest", r2)
	}
	routed.Lock()
	failures := routed.failures[a.restKey()] + routed.failures[b.restKey()] + routed.failures[a.restID()]
	routed.Unlock()
	if failures != 0 {
		t.Fatalf("a firewall's block counted %d failures against the accounts", failures)
	}
	// the block lifts: both are asked again at once, with no ladder behind
	expire("zplug*")
	for _, c := range []candidate{a, b} {
		if r, ok := restOfAny(c); ok {
			t.Fatalf("%s still rests %v after the block lifted", c.label(), r.Until)
		}
	}
	if r := s.restAfter(a, http.StatusMethodNotAllowed, http.Header{}, zcodeBlocked); r.Failures != 0 || r.By != "blocked" {
		t.Fatalf("blocked again after the block lifted: %+v, want a fresh three minutes", r)
	}
}

// "Try now" on one account of a blocked provider asks the whole provider
// again: the block is the address's, and the user's retry is the way
// around it.
func TestEdgeBlockLiftedByTryNow(t *testing.T) {
	fresh(t)
	s := New()
	a := blockedAccount("one")
	s.restAfter(a, http.StatusMethodNotAllowed, http.Header{}, zcodeBlocked)
	if !s.Unrest(a.restKey()) {
		t.Fatal("Try now on a blocked account lifted nothing")
	}
	if _, ok := restOfAny(blockedAccount("two")); ok {
		t.Fatal("Try now on one account left the provider blocked")
	}
}

// A 405 that isn't a firewall's block is still the account's: it climbs the
// ladder as any other failure does.
func TestPlain405RestsTheAccount(t *testing.T) {
	fresh(t)
	s := New()
	a := blockedAccount("one")
	plain := []byte(`{"error":{"message":"Method Not Allowed"}}`)
	r := s.restAfter(a, http.StatusMethodNotAllowed, http.Header{}, plain)
	if r.By == "blocked" || r.Key != a.restKey() {
		t.Fatalf("a plain 405: %+v, want the account's own rest", r)
	}
	if _, ok := restOfAny(blockedAccount("two")); ok {
		t.Fatal("a plain 405 rested the provider's other account")
	}
}

// zcodeBusy is the ZCode plugin's answer for a request past the plan's
// in-flight cap, as magpie met it at 10:19 on 11-07.
var zcodeBusy = []byte(`{"type":"error","error":{"type":"rate_limit_error","message":"user concurrency limit exceeded (429, code 3008) — ZCode serves one account a few requests at a time, and every agent and session of this computer asks through magpie; this request came behind the others, and the same request a moment later is served"}}`)

// The plan's in-flight cap is the account being busy, not a rate limit:
// the account sits out half a minute, the same every time, and no rung of
// the rate ladder is climbed. Before, it was a rate limit: on 11-07
// zcode@龙_2608 sat out thirty minutes by 01:02 and zcode@唯心_9997 sixteen
// by 01:04, each asked again at the end and refused for being busy again.
func TestZCodeConcurrencyCapRestsBriefly(t *testing.T) {
	fresh(t)
	s := New()
	c := candidate{p: provider.PluginForTest("zcode", "zcode"), model: "GLM-5.3-Flash", rest: "zcode#k"}
	r := s.restAfter(c, http.StatusTooManyRequests, http.Header{}, zcodeBusy)
	if r.By != "cooldown" || r.Failures != 0 || r.Key != c.restKey() || r.Why != failRate {
		t.Fatalf("the plan's cap: %+v, want a cooldown of the account, no failures", r)
	}
	if left := time.Until(r.Until); left < overloadRest-time.Second || left > overloadRest+time.Second {
		t.Fatalf("the cap rests %v, want about %v", left.Round(time.Second), overloadRest)
	}
	// refused again and again: the same half minute, never a ladder
	for i := 0; i < 5; i++ {
		expire(c.restKey())
		r = s.restAfter(c, http.StatusTooManyRequests, http.Header{}, zcodeBusy)
		if r.Failures != 0 || time.Until(r.Until) > overloadRest+time.Second {
			t.Fatalf("refused %d times: %+v, want half a minute every time", i+1, r)
		}
	}
	// another vendor's 429 that says the same words is its own rate limit
	other := candidate{p: provider.Provider{ID: "elsewhere", Chat: "http://127.0.0.1:1/v1"}, model: "m", rest: "elsewhere"}
	if r := s.restAfter(other, http.StatusTooManyRequests, http.Header{}, zcodeBusy); r.By == "cooldown" && r.Failures == 0 {
		t.Fatalf("another vendor's 429 was read as ZCode's cap: %+v", r)
	}
}
