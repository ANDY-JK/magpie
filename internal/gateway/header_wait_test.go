package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
)

// F-02: a provider's first-byte wait reaches its own vendor. A vendor that
// begins answering later than the gateway's default is cut there for a
// provider that sets none, and answered for one that sets its own.
func TestProviderHeaderWaitReachesItsVendor(t *testing.T) {
	fresh(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		time.Sleep(60 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	t.Cleanup(up.Close)

	// the gateway's own wait, shrunk so the test needn't wait minutes; a
	// provider's HeaderWait counts these per unit
	wasWait, wasUnit := upstreamHeaderWait, providerHeaderWaitUnit
	upstreamHeaderWait, providerHeaderWaitUnit = 20*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { upstreamHeaderWait, providerHeaderWaitUnit = wasWait, wasUnit })

	for _, p := range []provider.Provider{
		{ID: "short", Name: "Short", Chat: up.URL + "/v1", Key: "k", Models: []string{"m"}},
		{ID: "long", Name: "Long", Chat: up.URL + "/v1", Key: "k", Models: []string{"m"}, HeaderWait: new(int)},
	} {
		if p.ID == "long" {
			*p.HeaderWait = 4 // 4 × 50ms: longer than the vendor's 60ms
		}
		if err := provider.Save(p); err != nil {
			t.Fatal(err)
		}
		if err := catalog.SaveLive(p.ID, up.URL+"/v1", []catalog.Model{{ID: "m"}}); err != nil {
			t.Fatal(err)
		}
	}

	s := New()
	ask := func(model string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/chat/completions",
			strings.NewReader(`{"model":"`+model+`","messages":[{"role":"user","content":"hi"}]}`))
		s.Handler().ServeHTTP(rec, req)
		return rec.Code
	}

	// no HeaderWait: the vendor's 60ms is past the gateway's 20ms
	if code := ask("short/m"); code != http.StatusBadGateway && code != http.StatusGatewayTimeout {
		t.Fatalf("the provider with no wait answered %d, want it cut at the default", code)
	}
	// its own: 200ms, so the vendor's 60ms is answered
	if code := ask("long/m"); code != http.StatusOK {
		t.Fatalf("the provider's own wait answered %d, want 200", code)
	}
}
