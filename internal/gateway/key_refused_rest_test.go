package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// A key its vendor refuses outright — Kimi's 401 "The API Key appears to
// be invalid or may have expired", which says nothing of an allowance —
// is no wait's to mend. Asked on every request, one such key was sent
// 1743 requests in two days, each refused, and an agent left on it had
// nothing but the vendor's 401; as the last one left it rested nothing at
// all (restAfterMarked is not called for it). It rests as an account out
// of credit does now, and a request within the rest is answered from it
// without asking again (keyRefusedRest, quotaHeld).

// refusedKeys answers a key starting with "refused" as Kimi answers a
// dead one, and serves any other.
type refusedKeys struct {
	mu   sync.Mutex
	keys []string
}

const refusedKeyWords = `{"error":{"message":"The API Key appears to be invalid or may have expired. Please verify your credentials and try again.","type":"invalid_authentication_error"}}`

func (d *refusedKeys) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	io.Copy(io.Discard, r.Body)
	key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	d.mu.Lock()
	d.keys = append(d.keys, key)
	d.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if strings.HasPrefix(key, "refused") {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, refusedKeyWords)
		return
	}
	io.WriteString(w, `{"id":"c1","object":"chat.completion","model":"glm-5.3-flash","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
}

func (d *refusedKeys) took() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.keys
	d.keys = nil
	return out
}

func refusedChain(t *testing.T, keys ...string) (*refusedKeys, *Server) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	forgetRouting()
	t.Cleanup(forgetRouting)
	d := &refusedKeys{}
	up := httptest.NewServer(d)
	t.Cleanup(up.Close)
	p := provider.Provider{ID: "mockrefused", Name: "mockrefused", Chat: up.URL + "/v1", Models: []string{"glm-5.3-flash"}, Routing: provider.Ordered}
	p.Key = keys[0]
	for _, k := range keys[1:] {
		p.Keys = append(p.Keys, provider.KeyAccount{Key: k})
	}
	if err := provider.Save(p); err != nil {
		t.Fatal(err)
	}
	return d, New()
}

func postRefused(t *testing.T, srv *Server, model string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"`+model+`","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func TestKeyRefusedRestsAndIsAnsweredFromTheRest(t *testing.T) {
	d, srv := refusedChain(t, "refused-1")

	rec := postRefused(t, srv, "mockrefused/glm-5.3-flash")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if took := d.took(); len(took) != 1 {
		t.Fatalf("asked %v, want the one key once", took)
	}
	r, ok := restOf("mockrefused")
	if !ok {
		t.Fatal("the key the vendor refuses doesn't rest: it is asked on every request")
	}
	if r.Why != failOther || r.By != "key" {
		t.Fatalf("rests %v by %s, want a refused key (other, key)", r.Why, r.By)
	}
	if until := time.Until(r.Until); until > keyRefusedRest || until < keyRefusedRest-time.Minute {
		t.Fatalf("rests %v, want %v", until.Round(time.Second), keyRefusedRest)
	}

	// the next request is answered from that rest, without asking again
	rec = postRefused(t, srv, "mockrefused/glm-5.3-flash")
	if took := d.took(); len(took) != 0 {
		t.Fatalf("asked again within the rest: %v", took)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	said := rec.Body.String()
	if !strings.Contains(said, "invalid") || !strings.Contains(said, "its key was refused") {
		t.Fatalf("the agent was told %q, want the vendor's words and why", said)
	}
}

// A key refused while a sibling of its provider serves rests alone: the
// sibling answers, and the refused one isn't asked on the next request.
func TestKeyRefusedIsPassedOverForItsSiblings(t *testing.T) {
	d, srv := refusedChain(t, "refused-1", "good-1")

	rec := postRefused(t, srv, "mockrefused/glm-5.3-flash")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	r, ok := restOf("mockrefused#" + provider.KeyID("refused-1"))
	if !ok {
		t.Fatalf("the refused key doesn't rest; asked %v", d.took())
	}
	if r.By != "key" {
		t.Fatalf("the refused key rests %v by %s, want a refused key (key)", r.Why, r.By)
	}
	if _, ok := restOf("mockrefused#" + provider.KeyID("good-1")); ok {
		t.Fatal("the key that answered rests")
	}

	d.took()
	rec = postRefused(t, srv, "mockrefused/glm-5.3-flash")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	for _, k := range d.took() {
		if strings.HasPrefix(k, "refused") {
			t.Fatalf("the refused key is asked again: %v", k)
		}
	}
}

// Only a key, and only a 401 that says nothing else: a 401 that names a
// used-up allowance is one, and a subscription's sign-in is its own
// account's to mark, resting as it did.
func TestKeyRefusedIsAKeyAndA401ThatSaysNothingElse(t *testing.T) {
	key := candidate{p: provider.Provider{ID: "k", Key: "sk-1"}, model: "m", rest: "k"}
	acct := candidate{p: provider.Provider{ID: "a", Account: &provider.Account{Agent: "codex", User: "u@example.com"}}, model: "m", rest: "a"}

	if !keyRefused(key, http.StatusUnauthorized, failure(http.StatusUnauthorized, []byte(refusedKeyWords))) {
		t.Fatal("a key its vendor refuses outright isn't read as one")
	}
	if keyRefused(key, http.StatusForbidden, failOther) {
		t.Error("a 403 read as a refused key")
	}
	if keyRefused(key, http.StatusUnauthorized, failQuota) {
		t.Error("a 401 that names a used-up allowance read as a refused key")
	}
	if keyRefused(acct, http.StatusUnauthorized, failOther) {
		t.Error("an account read as a refused key: its sign-in has its own marks")
	}
	if keyRefused(candidate{p: provider.Provider{ID: "k"}, model: "m", rest: "k"}, http.StatusUnauthorized, failOther) {
		t.Error("a candidate with no key read as one")
	}

	s := &Server{}
	forgetRouting()
	t.Cleanup(forgetRouting)
	// the rest it takes is not a sign-in's: another vendor's key may carry a
	// token its own app renews (#716)
	if r := s.restAfter(key, http.StatusUnauthorized, nil, []byte(`{"error":{"message":"OAuth access token has been revoked"}}`)); r.Why != failOther || r.By != "key" {
		t.Fatalf("a refused key rests %v by %s, want other by key", r.Why, r.By)
	}
	// a 401 that says the allowance is used up still rests as one
	if r := s.restAfter(key, http.StatusUnauthorized, nil, []byte(`{"error":{"message":"you have reached your usage limit"}}`)); r.Why != failQuota {
		t.Fatalf("a 401 naming the allowance rests %v by %s", r.Why, r.By)
	}
	// and an account's plain 401 is the backoff it was
	if r := s.restAfter(acct, http.StatusUnauthorized, nil, []byte(`{"error":{"message":"Unauthorized"}}`)); r.By != "backoff" {
		t.Fatalf("an account's plain 401 rests by %s, want backoff", r.By)
	}
}

// The rest a refused key takes is one the requests are answered from,
// as a key out of its allowance is (#1772): otherwise a key alone on its
// provider is asked on every request again.
func TestQuotaHeldIncludesARefusedKey(t *testing.T) {
	s := &Server{}
	forgetRouting()
	t.Cleanup(forgetRouting)
	key := candidate{p: provider.Provider{ID: "k", Key: "sk-1"}, model: "m", rest: "k"}
	s.restAfter(key, http.StatusUnauthorized, nil, []byte(refusedKeyWords))
	if _, _, ok := quotaHeld([]candidate{key}); !ok {
		t.Fatal("a key its vendor refuses isn't held: every request asks it again")
	}
	if _, _, ok := quotaHeld([]candidate{key, {p: provider.Provider{ID: "k2", Key: "sk-2"}, model: "m", rest: "k2"}}); ok {
		t.Fatal("held with a key that isn't resting")
	}
}
