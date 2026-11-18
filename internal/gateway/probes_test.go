package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// L-03 (the review): a script that only reads GET / can't tell a gateway
// that serves from one whose providers.json can't be read. /healthz says
// the process is up; /readyz says it can serve, and why not when it can't.
func TestProbesSplitAliveFromReady(t *testing.T) {
	fresh(t)
	mux := New().routes()
	get := func(path string) (int, string) {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w.Code, w.Body.String()
	}
	ok := func(path string) {
		t.Helper()
		code, body := get(path)
		if code != 200 {
			t.Fatalf("%s: %d %s", path, code, body)
		}
		var out struct {
			OK bool `json:"ok"`
		}
		if json.Unmarshal([]byte(body), &out) != nil || !out.OK {
			t.Fatalf("%s: %s", path, body)
		}
	}
	ok("/healthz")
	ok("/readyz")

	// a providers.json that doesn't parse: up, but not ready
	if err := os.MkdirAll(strings.TrimSuffix(provider.Path(), "providers.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(provider.Path(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	ok("/healthz")
	code, body := get("/readyz")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("readyz with a broken providers.json: %d %s", code, body)
	}
	var out struct {
		OK  bool   `json:"ok"`
		Why string `json:"why"`
	}
	if json.Unmarshal([]byte(body), &out) != nil || out.OK || out.Why == "" {
		t.Fatalf("a refusal must say why: %s", body)
	}
	if !strings.Contains(out.Why, "providers.json") {
		t.Fatalf("the refusal doesn't name the file: %s", out.Why)
	}

	// readable again: ready
	if err := os.Remove(provider.Path()); err != nil {
		t.Fatal(err)
	}
	ok("/readyz")
}
