package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
)

// failUp is an upstream that answers a Chat reply, or fails as told.
type failUp struct {
	mu   sync.Mutex
	fail bool
}

func (u *failUp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	io.ReadAll(r.Body)
	u.mu.Lock()
	fail := u.fail
	u.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if fail {
		// a rate limit, whose rest the terminal failure writes (backoff);
		// a plain 5xx of the last one left would not, by design
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"error":{"type":"rate_limit_error","message":"rate limited"}}`)
		return
	}
	io.WriteString(w, `{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
}

// The review's D-01, 11-16: a channel that went resting, or all of them,
// was told of nowhere but the Providers page, which the user has to be on.
// The gateway says the transition instead: the request that found every
// candidate of its model resting says the outage, once; the next failed
// requests are the same episode, until an answer says it is over.
func TestAllDownSaidOncePerEpisode(t *testing.T) {
	fresh(t)
	u := &failUp{}
	up := httptest.NewServer(u)
	t.Cleanup(up.Close)
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Chat: up.URL + "/v1", Key: "k", Models: []string{"m"}}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive("relay", up.URL+"/v1", []catalog.Model{{ID: "m"}}); err != nil {
		t.Fatal(err)
	}
	s := New()
	var said []string
	AllDownAlert = func(m string) {
		said = append(said, m)
	}
	t.Cleanup(func() { AllDownAlert = nil })

	// no candidate resting: nothing is said, answered or failed
	if code := allDownAsk(t, s, u); code != 200 {
		t.Fatalf("the ready request's status = %d", code)
	}
	if len(said) != 0 {
		t.Fatalf("a ready model was said all-down: %v", said)
	}

	// every candidate of the model rests, as a failure before would have
	// (restingUntil, written by restAfterMarked where the request's own
	// rests go)
	p, err := provider.Find("relay")
	if err != nil {
		t.Fatal(err)
	}
	cs, _ := s.planPaused(*p, "m", provider.Chat, nil)
	restEvery(t, cs)
	u.fail = true
	if code := allDownAsk(t, s, u); code != 429 {
		t.Fatalf("the outage's status = %d", code)
	}
	if len(said) != 1 || said[0] != "relay/m" {
		t.Fatalf("the episode said as %v, want [relay/m]", said)
	}

	// while the outage holds, the next failed requests are the same
	// episode, not new ones
	allDownAsk(t, s, u)
	allDownAsk(t, s, u)
	if len(said) != 1 {
		t.Fatalf("the same episode said again: %v", said)
	}

	// the channel back: the episode is over, said nowhere, and its next
	// says itself again
	u.fail = false
	if code := allDownAsk(t, s, u); code != 200 {
		t.Fatalf("the recovery's status = %d", code)
	}
	if len(said) != 1 {
		t.Fatalf("the recovery was said: %v", said)
	}
	restEvery(t, cs)
	u.fail = true
	allDownAsk(t, s, u)
	if len(said) != 2 {
		t.Fatalf("the episode after the recovery not said: %v", said)
	}
}

// A request the agent canceled (its 499) is not an outage: the channels
// are not at fault for the agent going away.
func TestAllDownNotSaidForACanceledRequest(t *testing.T) {
	cs := []candidate{{p: provider.Provider{ID: "relay", Key: "k"}, model: "m", rest: "relay#k"}}
	restingUntil.Lock()
	restingUntil.m["relay#k"] = time.Now().Add(time.Hour)
	restingUntil.note["relay#k"] = Rest{Why: failOther, Status: 500, Key: "relay#k"}
	restingUntil.Unlock()
	allDownSaid()
	var said bool
	AllDownAlert = func(string) { said = true }
	t.Cleanup(func() { AllDownAlert = nil })
	downSaid(cs, "m", 499)
	if said {
		t.Fatal("a canceled request was said all-down")
	}
	downSaid(cs, "m", 500)
	if !said {
		t.Fatal("a failed request with every candidate resting not said")
	}
}

// restEvery rests each of cs's rest keys as the product's own rests go in,
// for as long as the test runs.
func restEvery(t *testing.T, cs []candidate) {
	t.Helper()
	until := time.Now().Add(time.Hour)
	restingUntil.Lock()
	for _, c := range cs {
		restingUntil.m[c.restKey()] = until
		restingUntil.note[c.restKey()] = Rest{Why: failOther, Status: 500, Key: c.restKey()}
	}
	restingUntil.Unlock()
}

// allDownAsk sends one Chat request of the model at the gateway, as the
// user's agent would.
func allDownAsk(t *testing.T, s *Server, u *failUp) int {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions",
		strings.NewReader(`{"model":"relay/m","messages":[{"role":"user","content":"hi"}]}`))
	s.Handler().ServeHTTP(rec, req)
	_ = u
	return rec.Code
}
