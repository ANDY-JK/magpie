package gateway

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// TestProxyDownDoesNotRest (#381): a member whose proxy isn't listening
// (127.0.0.1:1082 down, "proxyconnect tcp: … connection refused") never
// reached its vendor. The next member answers, and the first isn't set
// aside: it is asked first again as soon as the proxy is back, not after a
// backoff that grew with every request made while it was down.
func TestProxyDownDoesNotRest(t *testing.T) {
	for _, scheme := range []string{"http", "socks5"} {
		t.Run(scheme, func(t *testing.T) {
			fresh(t)
			for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
				t.Setenv(k, "")
			}
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"id":"c","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"from b"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
			}))
			t.Cleanup(up.Close)
			down := deadAddr(t, up.Listener.Addr().String())
			for _, p := range []provider.Provider{
				{ID: "a", Name: "A", Key: "ka", Models: []string{"m"}, Chat: "http://a.test/v1", Proxy: scheme + "://" + down},
				{ID: "b", Name: "B", Key: "kb", Models: []string{"m"}, Chat: up.URL + "/v1"},
			} {
				if err := provider.Save(p); err != nil {
					t.Fatal(err)
				}
			}
			if err := provider.SaveGroup(provider.Group{Name: "Grok", Members: []string{"a/m", "b/m"}, Routing: provider.Ordered}); err != nil {
				t.Fatal(err)
			}
			s := New()
			for i := 0; i < 2; i++ {
				code, body := postAs(t, s, "", `{"model":"group/grok","messages":[{"role":"user","content":"hi"}]}`)
				if code != 200 || !strings.Contains(body, "from b") {
					t.Fatalf("request %d: %d %s", i, code, body)
				}
				r := s.trace.routes[len(s.trace.routes)-1]
				if len(r.Tries) != 2 || r.Tries[0].ID != r.Order[0].ID || r.Order[0].Provider != "a" {
					t.Fatalf("request %d: a wasn't asked first: %+v", i, r.Tries)
				}
				if f := r.Tries[0]; f.Fail != "proxy" || f.Rest != nil || !strings.Contains(f.Error, down) {
					t.Fatalf("request %d: a's try %+v", i, f)
				}
			}
			restingUntil.Lock()
			n := len(restingUntil.m)
			restingUntil.Unlock()
			if n != 0 {
				t.Fatalf("%d set aside over a proxy that was down", n)
			}
		})
	}
}

// deadAddr is a loopback address nothing listens on, for a proxy that is
// down. It is taken after the test's own servers are up and is never one of
// theirs: a port just let go can be handed out again at once (Linux CI gave
// the closed port to the next httptest server, so the "down" proxy answered).
func deadAddr(t *testing.T, taken ...string) string {
	t.Helper()
	for range 20 {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		a := l.Addr().String()
		l.Close()
		if !slices.Contains(taken, a) {
			return a
		}
	}
	t.Fatal("no free loopback port apart from the test's servers")
	return ""
}

// clearProxyEnv keeps a proxy set for the machine out of a test's way.
func clearProxyEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(k, "")
	}
}

// globalProxyTo sets the proxy magpie's settings hold, the global one a
// provider without its own follows.
func globalProxyTo(t *testing.T, addr string) {
	t.Helper()
	if err := settings.Save(settings.Settings{Proxy: "http://" + addr}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = settings.Save(settings.Settings{}) })
}

// TestTransportFailuresStopTheChain (the review's F-4, 11-16): a local
// proxy that takes no connection failed every one of 24 channels in 34
// tries. Three providers in a row that never reached a vendor are enough
// to tell that every one left would fail the same way: the request stops,
// the last try's 502 is the agent's error, and the model's outage is said
// once (F-9). No candidate rests for a failure that never reached it.
func TestTransportFailuresStopTheChain(t *testing.T) {
	fresh(t)
	clearProxyEnv(t)
	down := deadAddr(t)
	ids := []string{"a", "b", "c", "d", "e", "f"}
	members := make([]string, len(ids))
	for i, id := range ids {
		if err := provider.Save(provider.Provider{ID: id, Name: strings.ToUpper(id), Key: "k", Models: []string{"m"}, Chat: "http://" + id + ".test/v1", Proxy: "http://" + down}); err != nil {
			t.Fatal(err)
		}
		members[i] = id + "/m"
	}
	if err := provider.SaveGroup(provider.Group{Name: "G", Members: members, Routing: provider.Ordered}); err != nil {
		t.Fatal(err)
	}
	s := New()
	var said []string
	AllDownAlert = func(m string) { said = append(said, m) }
	t.Cleanup(func() { AllDownAlert = nil })

	code, body := postAs(t, s, "", `{"model":"group/g","messages":[{"role":"user","content":"hi"}]}`)
	if code != http.StatusBadGateway || !strings.Contains(strings.ToLower(body), "proxy") {
		r := s.trace.routes[len(s.trace.routes)-1]
		t.Fatalf("%d %s route=%d %q tries=%+v", code, body, r.Status, r.Error, r.Tries)
	}
	r := s.trace.routes[len(s.trace.routes)-1]
	if len(r.Tries) != transportFailMost {
		t.Fatalf("%d tries of %d members: %+v", len(r.Tries), len(ids), r.Tries)
	}
	for i, tr := range r.Tries {
		if tr.Fail != failProxy {
			t.Fatalf("try %d: %+v", i, tr)
		}
	}
	if len(said) != 1 || said[0] != "group/g" {
		t.Fatalf("the outage said as %v, want [group/g]", said)
	}
	restingUntil.Lock()
	n := len(restingUntil.m)
	restingUntil.Unlock()
	if n != 0 {
		t.Fatalf("%d set aside over a proxy that was down", n)
	}
}

// TestAGlobalProxyDoesNotSkipADirectCandidate: the same, through the global
// proxy this machine is set to (no provider has one of its own): a direct
// candidate still answers after three providers failed on it.
func TestAGlobalProxyDoesNotSkipADirectCandidate(t *testing.T) {
	fresh(t)
	clearProxyEnv(t)
	down := deadAddr(t)
	globalProxyTo(t, down)
	for _, id := range []string{"a", "b", "c"} {
		if err := provider.Save(provider.Provider{ID: id, Name: strings.ToUpper(id), Key: "k", Models: []string{"m"}, Chat: "http://" + id + ".test/v1"}); err != nil {
			t.Fatal(err)
		}
	}
	scriptedOn(t, "d", provider.Chat, &scripted{replies: []reply{{code: 200, body: chatOK}}})
	if err := provider.SaveGroup(provider.Group{Name: "G", Members: []string{"a/m", "b/m", "c/m", "d/m"}, Routing: provider.Ordered}); err != nil {
		t.Fatal(err)
	}
	s := New()
	code, body := postAs(t, s, "", `{"model":"group/g","messages":[{"role":"user","content":"hi"}]}`)
	if code != 200 || !strings.Contains(body, "hello") {
		t.Fatalf("%d %s", code, body)
	}
	r := s.trace.routes[len(s.trace.routes)-1]
	if len(r.Tries) != 4 || r.Tries[3].Status != 200 {
		t.Fatalf("%d tries: %+v", len(r.Tries), r.Tries)
	}
}

// TestOneProvidersProxyDoesNotStopTheChain: a provider's own proxy is its
// own, and its several accounts failing through it are one provider, not
// three: the next provider is still asked and answers.
func TestOneProvidersProxyDoesNotStopTheChain(t *testing.T) {
	fresh(t)
	clearProxyEnv(t)
	down := deadAddr(t)
	if err := provider.Save(provider.Provider{ID: "a", Name: "A", Key: "k1", Keys: []provider.KeyAccount{{Key: "k2"}, {Key: "k3"}}, Models: []string{"m"}, Chat: "http://a.test/v1", Proxy: "http://" + down}); err != nil {
		t.Fatal(err)
	}
	scriptedOn(t, "b", provider.Chat, &scripted{replies: []reply{{code: 200, body: chatOK}}})
	if err := provider.SaveGroup(provider.Group{Name: "G", Members: []string{"a/m", "b/m"}, Routing: provider.Ordered}); err != nil {
		t.Fatal(err)
	}
	s := New()
	code, body := postAs(t, s, "", `{"model":"group/g","messages":[{"role":"user","content":"hi"}]}`)
	if code != 200 || !strings.Contains(body, "hello") {
		t.Fatalf("%d %s", code, body)
	}
	r := s.trace.routes[len(s.trace.routes)-1]
	if len(r.Tries) != 4 || r.Tries[3].Status != 200 {
		t.Fatalf("%d tries: %+v", len(r.Tries), r.Tries)
	}
}

// TestADirectCandidateIsAskedAfterDeadProxies: three providers behind the
// same dead proxy are not every way left — a fourth set to direct still
// answers, so the chain must ask it rather than stop on the proxy's count.
func TestADirectCandidateIsAskedAfterDeadProxies(t *testing.T) {
	fresh(t)
	clearProxyEnv(t)
	down := deadAddr(t)
	for _, id := range []string{"a", "b", "c"} {
		if err := provider.Save(provider.Provider{ID: id, Name: strings.ToUpper(id), Key: "k", Models: []string{"m"}, Chat: "http://" + id + ".test/v1", Proxy: "http://" + down}); err != nil {
			t.Fatal(err)
		}
	}
	scriptedOn(t, "d", provider.Chat, &scripted{replies: []reply{{code: 200, body: chatOK}}})
	if err := provider.SaveGroup(provider.Group{Name: "G", Members: []string{"a/m", "b/m", "c/m", "d/m"}, Routing: provider.Ordered}); err != nil {
		t.Fatal(err)
	}
	s := New()
	code, body := postAs(t, s, "", `{"model":"group/g","messages":[{"role":"user","content":"hi"}]}`)
	if code != 200 || !strings.Contains(body, "hello") {
		t.Fatalf("%d %s", code, body)
	}
	r := s.trace.routes[len(s.trace.routes)-1]
	if len(r.Tries) != 4 || r.Tries[3].Status != 200 {
		t.Fatalf("%d tries: %+v", len(r.Tries), r.Tries)
	}
}

// TestASlowCandidateResetsTheTransportStop: a member that reached its
// vendor and was slow to start is no transport failure; the proxy
// failures after it start the count again, so the last one is still asked
// rather than the chain stopping at d. Without the reset, d is the third
// failure and the same path stops it at 4 tries.
func TestASlowCandidateResetsTheTransportStop(t *testing.T) {
	fresh(t)
	clearProxyEnv(t)
	down := deadAddr(t)
	for _, id := range []string{"a", "c", "d", "e"} {
		if err := provider.Save(provider.Provider{ID: id, Name: strings.ToUpper(id), Key: "k", Models: []string{"m"}, Chat: "http://" + id + ".test/v1", Proxy: "http://" + down}); err != nil {
			t.Fatal(err)
		}
	}
	slowOn(t, "b", &slowStart{wait: 10 * time.Second})
	if err := provider.SaveGroup(provider.Group{Name: "G", Members: []string{"a/m", "b/m", "c/m", "d/m", "e/m"}, Routing: provider.Ordered, FirstToken: 1}); err != nil {
		t.Fatal(err)
	}
	s := New()
	code, body := postAs(t, s, "", `{"model":"group/g","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	if code != http.StatusBadGateway {
		t.Fatalf("%d %s", code, body)
	}
	r := s.trace.routes[len(s.trace.routes)-1]
	if len(r.Tries) != 5 || r.Tries[1].Fail != failSlow || r.Tries[4].Fail != failProxy {
		t.Fatalf("%d tries: %+v", len(r.Tries), r.Tries)
	}
}

// TestA503ResetsTheTransportStop: a vendor's own 503 reached it; the proxy
// failures after it start the count again, so e is asked (5 tries) rather
// than the chain stopping at d (4).
func TestA503ResetsTheTransportStop(t *testing.T) {
	fresh(t)
	clearProxyEnv(t)
	down := deadAddr(t)
	for _, id := range []string{"a", "c", "d", "e"} {
		if err := provider.Save(provider.Provider{ID: id, Name: strings.ToUpper(id), Key: "k", Models: []string{"m"}, Chat: "http://" + id + ".test/v1", Proxy: "http://" + down}); err != nil {
			t.Fatal(err)
		}
	}
	scriptedOn(t, "b", provider.Chat, &scripted{replies: []reply{{code: http.StatusServiceUnavailable, body: `{"error":{"message":"busy"}}`}}})
	if err := provider.SaveGroup(provider.Group{Name: "G", Members: []string{"a/m", "b/m", "c/m", "d/m", "e/m"}, Routing: provider.Ordered}); err != nil {
		t.Fatal(err)
	}
	s := New()
	code, body := postAs(t, s, "", `{"model":"group/g","messages":[{"role":"user","content":"hi"}]}`)
	if code != http.StatusBadGateway {
		t.Fatalf("%d %s", code, body)
	}
	r := s.trace.routes[len(s.trace.routes)-1]
	if len(r.Tries) != 5 || r.Tries[1].Status != http.StatusServiceUnavailable || r.Tries[4].Fail != failProxy {
		t.Fatalf("%d tries: %+v", len(r.Tries), r.Tries)
	}
}

// TestAGlobalProxyStopsAfterThree: the F-4 case, with no provider's own
// proxy: the global proxy magpie's settings name is dead, and after three providers the
// rest are the same dead proxy, so the request stops at 3 (not 6), every
// try failed as proxy, and the outage is said once.
func TestAGlobalProxyStopsAfterThree(t *testing.T) {
	fresh(t)
	clearProxyEnv(t)
	down := deadAddr(t)
	globalProxyTo(t, down)
	ids := []string{"a", "b", "c", "d", "e", "f"}
	members := make([]string, len(ids))
	for i, id := range ids {
		if err := provider.Save(provider.Provider{ID: id, Name: strings.ToUpper(id), Key: "k", Models: []string{"m"}, Chat: "http://" + id + ".test/v1"}); err != nil {
			t.Fatal(err)
		}
		members[i] = id + "/m"
	}
	if err := provider.SaveGroup(provider.Group{Name: "G", Members: members, Routing: provider.Ordered}); err != nil {
		t.Fatal(err)
	}
	s := New()
	var said []string
	AllDownAlert = func(m string) { said = append(said, m) }
	t.Cleanup(func() { AllDownAlert = nil })
	code, body := postAs(t, s, "", `{"model":"group/g","messages":[{"role":"user","content":"hi"}]}`)
	if code != http.StatusBadGateway || !strings.Contains(strings.ToLower(body), "proxy") {
		t.Fatalf("%d %s", code, body)
	}
	r := s.trace.routes[len(s.trace.routes)-1]
	if len(r.Tries) != 3 {
		t.Fatalf("%d tries: %+v", len(r.Tries), r.Tries)
	}
	for i, tr := range r.Tries {
		if tr.Fail != failProxy {
			t.Fatalf("try %d: %+v", i, tr)
		}
	}
	if len(said) != 1 || said[0] != "group/g" {
		t.Fatalf("the outage said as %v, want [group/g]", said)
	}
}

// TestAGlobalProxyFromTheEnvironmentStopsAfterThree: the same stop when the
// global proxy comes from the environment (ALL_PROXY, which netproxy reads
// beside HTTP_PROXY), with no provider's own proxy and none in settings.
func TestAGlobalProxyFromTheEnvironmentStopsAfterThree(t *testing.T) {
	fresh(t)
	clearProxyEnv(t)
	down := deadAddr(t)
	t.Setenv("ALL_PROXY", "http://"+down)
	t.Setenv("all_proxy", "http://"+down)
	ids := []string{"a", "b", "c", "d", "e", "f"}
	members := make([]string, len(ids))
	for i, id := range ids {
		if err := provider.Save(provider.Provider{ID: id, Name: strings.ToUpper(id), Key: "k", Models: []string{"m"}, Chat: "http://" + id + ".test/v1"}); err != nil {
			t.Fatal(err)
		}
		members[i] = id + "/m"
	}
	if err := provider.SaveGroup(provider.Group{Name: "G", Members: members, Routing: provider.Ordered}); err != nil {
		t.Fatal(err)
	}
	s := New()
	var said []string
	AllDownAlert = func(m string) { said = append(said, m) }
	t.Cleanup(func() { AllDownAlert = nil })
	code, body := postAs(t, s, "", `{"model":"group/g","messages":[{"role":"user","content":"hi"}]}`)
	if code != http.StatusBadGateway || !strings.Contains(strings.ToLower(body), "proxy") {
		t.Fatalf("%d %s", code, body)
	}
	r := s.trace.routes[len(s.trace.routes)-1]
	if len(r.Tries) != 3 {
		t.Fatalf("%d tries: %+v", len(r.Tries), r.Tries)
	}
	for i, tr := range r.Tries {
		if tr.Fail != failProxy {
			t.Fatalf("try %d: %+v", i, tr)
		}
	}
	if len(said) != 1 || said[0] != "group/g" {
		t.Fatalf("the outage said as %v, want [group/g]", said)
	}
}

// TestTransportPathOfGlobalProxy: a provider with no proxy of its own takes
// this machine's global proxy; one set to direct goes direct; one with its own
// proxy takes that. transportPath must tell them apart for the stop.
func TestTransportPathOfGlobalProxy(t *testing.T) {
	fresh(t)
	clearProxyEnv(t)
	down := deadAddr(t)
	globalProxyTo(t, down)
	global := candidate{p: provider.Provider{ID: "g", Chat: "http://g.test/v1"}, model: "m"}
	if got := transportPath(global, provider.Chat); !strings.HasPrefix(got, "proxy ") || !strings.Contains(got, down) {
		t.Fatalf("global %q, down %q", got, down)
	}
	direct := candidate{p: provider.Provider{ID: "d", Chat: "http://d.test/v1", Proxy: "direct"}, model: "m"}
	if got := transportPath(direct, provider.Chat); got != "direct" {
		t.Fatalf("direct %q", got)
	}
	own := candidate{p: provider.Provider{ID: "o", Chat: "http://o.test/v1", Proxy: "http://" + down}, model: "m"}
	if got := transportPath(own, provider.Chat); got != "proxy http://"+down {
		t.Fatalf("own %q", got)
	}
}

// TestTransportRunReachedVendorResets: the counter behind the stop resets
// when a try reached its vendor (a slow start, a loop, its own 5xx, an
// answer); the slow and 5xx cases above prove the loop calls it.
func TestTransportRunReachedVendorResets(t *testing.T) {
	r := transportRun{all: true}
	r.failed(failProxy)
	r.failed(failProxy)
	r.failed(failOffline)
	if r.fails != 3 || r.why != failOffline || !r.all {
		t.Fatalf("%+v", r)
	}
	r.reachedVendor()
	if r.fails != 0 || r.why != "" || r.all {
		t.Fatalf("after a vendor was reached: %+v", r)
	}
	r.failed(failProxy)
	if r.fails != 1 || r.why != failProxy || r.all {
		t.Fatalf("the next failure: %+v", r)
	}
}

// TestSameTransportPath: a stop is safe only when every candidate left
// takes the same proxy (or goes direct too); a different or unknown path
// never matches.
func TestSameTransportPath(t *testing.T) {
	dead := "http://127.0.0.1:9"
	a := candidate{p: provider.Provider{ID: "a", Chat: "http://a.test/v1", Proxy: dead}, model: "m"}
	b := candidate{p: provider.Provider{ID: "b", Chat: "http://b.test/v1", Proxy: dead}, model: "m"}
	direct := candidate{p: provider.Provider{ID: "c", Chat: "http://c.test/v1", Proxy: "direct"}, model: "m"}
	unknown := candidate{p: provider.Provider{ID: "u"}, model: "m"}
	path := transportPath(a, provider.Chat)
	if path == "" || strings.HasPrefix(path, "unknown ") {
		t.Fatalf("a's path = %q", path)
	}
	if !sameTransportPath([]candidate{b}, path, provider.Chat) {
		t.Fatal("the same proxy didn't match")
	}
	if sameTransportPath([]candidate{direct}, path, provider.Chat) {
		t.Fatal("a direct candidate matched a proxy's path")
	}
	if sameTransportPath([]candidate{unknown}, path, provider.Chat) {
		t.Fatal("an unknown path matched a proxy's")
	}
	if sameTransportPath([]candidate{unknown}, transportPath(unknown, provider.Chat), provider.Chat) {
		t.Fatal("two unknown paths matched")
	}
}

// TestOrdinaryServerErrorsStillAskTheNext: a vendor's own 5xx reached it,
// so it isn't the proxy's or the network's failure: every member is asked,
// and the one that answers is the agent's reply.
func TestOrdinaryServerErrorsStillAskTheNext(t *testing.T) {
	fresh(t)
	ids := []string{"a", "b", "c"}
	members := make([]string, len(ids)+1)
	for i, id := range ids {
		scriptedOn(t, id, provider.Chat, &scripted{replies: []reply{{code: http.StatusServiceUnavailable, body: `{"error":{"message":"busy"}}`}}})
		members[i] = id + "/m"
	}
	scriptedOn(t, "d", provider.Chat, &scripted{replies: []reply{{code: 200, body: chatOK}}})
	members[len(ids)] = "d/m"
	if err := provider.SaveGroup(provider.Group{Name: "G", Members: members, Routing: provider.Ordered}); err != nil {
		t.Fatal(err)
	}
	s := New()
	code, body := postAs(t, s, "", `{"model":"group/g","messages":[{"role":"user","content":"hi"}]}`)
	if code != 200 || !strings.Contains(body, "hello") {
		t.Fatalf("%d %s", code, body)
	}
	r := s.trace.routes[len(s.trace.routes)-1]
	if len(r.Tries) != len(members) || r.Tries[len(r.Tries)-1].Status != 200 {
		t.Fatalf("%d tries: %+v", len(r.Tries), r.Tries)
	}
}

// TestAProxyDownAloneFailsAtOnce (the review's F-9): the only candidate's
// proxy refused the connection. The agent gets the 502 at once, not after
// the same one is asked again within its patience, and the model's outage
// is said.
func TestAProxyDownAloneFailsAtOnce(t *testing.T) {
	fresh(t)
	clearProxyEnv(t)
	down := deadAddr(t)
	if err := provider.Save(provider.Provider{ID: "a", Name: "A", Key: "k", Models: []string{"m"}, Chat: "http://a.test/v1", Proxy: "http://" + down}); err != nil {
		t.Fatal(err)
	}
	s := New()
	var said []string
	AllDownAlert = func(m string) { said = append(said, m) }
	t.Cleanup(func() { AllDownAlert = nil })

	code, body := postAs(t, s, "", `{"model":"a/m","messages":[{"role":"user","content":"hi"}]}`)
	if code != http.StatusBadGateway || !strings.Contains(strings.ToLower(body), "proxy") {
		t.Fatalf("%d %s", code, body)
	}
	r := s.trace.routes[len(s.trace.routes)-1]
	if len(r.Tries) != 1 || r.Tries[0].Fail != failProxy {
		t.Fatalf("tries: %+v", r.Tries)
	}
	if len(said) != 1 || said[0] != "a/m" {
		t.Fatalf("the outage said as %v, want [a/m]", said)
	}
}
