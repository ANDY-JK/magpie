package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// F-07: MAGPIE_FAULT makes a provider answer as a broken vendor, so a
// group's fallback or a rest can be watched under the fault it was set
// for; off unless the env names it, and every injected answer says so.
func TestFaultInjection(t *testing.T) {
	fresh(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"real"}}]}`)
	}))
	defer up.Close()
	p := provider.Provider{ID: "fake", Name: "Fake", Key: "k", Chat: up.URL + "/v1"}
	s := New()
	body := []byte(`{"model":"m1","messages":[{"role":"user","content":"hi"}]}`)
	once := func() *http.Response {
		t.Helper()
		res, err := s.forwardOnce(context.Background(), p, provider.Chat, "/v1/chat/completions", body, http.Header{})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// nothing set: the vendor answers
	t.Setenv("MAGPIE_FAULT", "")
	res := once()
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(b), "real") {
		t.Fatalf("no fault asked, the answer is %q", b)
	}

	// a status: the vendor is never asked, and the answer says it was the fault
	t.Setenv("MAGPIE_FAULT", "fake=status:503")
	res = once()
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 503 || res.Header.Get("X-Magpie-Fault") != "fake=status:503" || !strings.Contains(string(b), "injected_fault") {
		t.Fatalf("injected status: %d %q %q", res.StatusCode, res.Header.Get("X-Magpie-Fault"), b)
	}

	// a cut: a 200 whose body stops half-way
	t.Setenv("MAGPIE_FAULT", "fake=cut")
	res = once()
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(string(b), `{"id":"injected"`) || strings.HasSuffix(string(b), "}") {
		t.Fatalf("injected cut: %d %q", res.StatusCode, b)
	}

	// a delay: the vendor is asked, after the wait
	t.Setenv("MAGPIE_FAULT", "fake=delay:120")
	start := time.Now()
	res = once()
	res.Body.Close()
	if waited := time.Since(start); waited < 120*time.Millisecond {
		t.Fatalf("injected delay waited %v", waited)
	}

	// another provider's rule is not this one's
	t.Setenv("MAGPIE_FAULT", "other=status:503")
	res = once()
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(b), "real") {
		t.Fatalf("another provider's rule was taken: %q", b)
	}
}
