package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/gateway"
)

// The lanes' answer carries the rests the gateway is passing over, so the
// page's two-second tick sees a rest gained or lifted without waiting for
// the whole read (WP4's badge followed the last read's rests alone).
func lanesRests(t *testing.T) map[string]any {
	t.Helper()
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/lanes", nil))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// A rest in force is in the summary by its rest key, to its until in
// milliseconds, and the lanes ride beside it: a key's (id#key), an
// account's (id@user) and a provider blocked as a whole's (id*).
func TestLaneRestsCarriedByTheLanes(t *testing.T) {
	sandboxHome(t)
	until := time.Now().Add(time.Hour)
	was := restsUnder
	restsUnder = func(string) []gateway.Rest {
		return []gateway.Rest{
			{Why: "failRate", Status: 429, Until: until, Key: "relay#k2"},
			{Why: "failQuota", Status: 429, Until: until, By: "resets", Key: "codex@me@example.com"},
			{Why: "failOther", Status: 403, Until: until, By: "blocked", Key: "relay*"},
		}
	}
	defer func() { restsUnder = was }()
	got := lanesRests(t)
	rests, ok := got["rests"].(map[string]any)
	if !ok {
		t.Fatalf("rests %v", got["rests"])
	}
	if len(rests) != 3 {
		t.Fatalf("rests %v", rests)
	}
	if ms, ok := rests["relay#k2"].(float64); !ok || ms != float64(until.UnixMilli()) {
		t.Fatalf("relay#k2 %v", rests["relay#k2"])
	}
	if ms, ok := rests["codex@me@example.com"].(float64); !ok || ms != float64(until.UnixMilli()) {
		t.Fatalf("codex@me@example.com %v", rests["codex@me@example.com"])
	}
	if ms, ok := rests["relay*"].(float64); !ok || ms != float64(until.UnixMilli()) {
		t.Fatalf("relay* %v", rests["relay*"])
	}
	if _, ok := got["lanes"].(map[string]any); !ok {
		t.Fatalf("lanes %v", got["lanes"])
	}
}

// A rest that has ended, and a model's own rest (which the row badge
// doesn't count, the model's rows say it), are none of the summary.
func TestLaneRestsExpireOutOfThem(t *testing.T) {
	sandboxHome(t)
	until := time.Now().Add(time.Hour)
	was := restsUnder
	restsUnder = func(string) []gateway.Rest {
		return []gateway.Rest{
			{Why: "failRate", Status: 429, Until: time.Now().Add(-time.Minute), Key: "relay"},
			{Why: "failQuota", Status: 429, Until: until, Key: "codex@me@example.com/model"},
		}
	}
	defer func() { restsUnder = was }()
	got := lanesRests(t)
	if rest := got["rests"]; rest != nil {
		t.Fatalf("rests %v", rest)
	}
}

// With no gateway served here and none answering elsewhere, no lane and no
// rest is in force: the answer says both plainly.
func TestLaneRestsNoneWithoutAGateway(t *testing.T) {
	sandboxHome(t)
	// the owner's lanes are asked at an address nothing serves, as
	// TestLaneLimitRoutes does
	t.Setenv("MAGPIE_ADDR", "127.0.0.1:1")
	was := restsUnder
	restsUnder = func(string) []gateway.Rest { return nil }
	defer func() { restsUnder = was }()
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/lanes", nil))
	if body := strings.TrimSpace(w.Body.String()); w.Code != 200 || body != `{"lanes":{},"rests":null}` {
		t.Fatalf("lanes %d %s", w.Code, w.Body)
	}
}
