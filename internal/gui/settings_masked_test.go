package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/settings"
)

// seedMasked saves settings whose fields hold a secret each, the ones the
// settings state is to tell the page only masked (settings.ForUI).
func seedMasked(t *testing.T) {
	t.Helper()
	s := settings.Load()
	s.Proxy = "http://me:pw@proxy.example:8080"
	s.GitHubToken = "gh-token-value"
	s.LANKey = "lan-key-value"
	s.OTel = settings.OTel{Enabled: true, Endpoint: "http://collector.example:4318",
		Headers: map[string]string{"Authorization": "Bearer otel-token", "X-Trace": "on"}}
	s.RedactAllow = []string{"ops@acme.io", "build-01"}
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	// Save keeps the last real proxy as ProxyCustom; off again, so the
	// masked ProxyCustom is its own here
	if s := settings.Load(); s.ProxyCustom == "" {
		t.Fatal("Save kept no last custom proxy")
	} else {
		s.Proxy = "direct"
		if err := settings.Save(s); err != nil {
			t.Fatal(err)
		}
	}
}

// The state and the settings state hold no secret: the proxy and the last
// custom one without their login, the OTLP headers' names alone, the
// masking allowlist masked, and no token nor key whole. The login comes
// back only from the routes that set these fields.
func TestSettingsStateHoldsNoSecret(t *testing.T) {
	sandboxHome(t)
	seedMasked(t)
	forgetLastState()
	t.Cleanup(forgetLastState)
	srv := Handler(nil, nil)
	get := func(path string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
		}
		return rec.Body.String()
	}
	for _, path := range []string{"/api/settings", "/api/state", "/api/state?last=1"} {
		if path == "/api/state?last=1" {
			get("/api/state") // the read that keeps one
		}
		b := get(path)
		for _, secret := range []string{"me:pw", "me@", "gh-token-value", "lan-key-value", "otel-token", "ops@acme.io", "build-01"} {
			if strings.Contains(b, secret) {
				t.Errorf("%s holds %q:\n%.400s", path, secret, b)
			}
		}
		if !strings.Contains(b, "proxy.example:8080") {
			t.Errorf("%s lost the proxy's address", path)
		}
		if !strings.Contains(b, settings.MaskAllow("ops@acme.io")) {
			t.Errorf("%s holds no masked allow value", path)
		}
	}
}

// Custom again in Settings brings the login back: the proxy route is told
// it as it is saved, what the settings state strips.
func TestSettingsProxyRoute(t *testing.T) {
	sandboxHome(t)
	seedMasked(t)
	rec := httptest.NewRecorder()
	Handler(nil, nil).ServeHTTP(rec, httptest.NewRequest("GET", "/api/settings/proxy", nil))
	if rec.Code != 200 {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body)
	}
	var got struct{ Proxy, ProxyCustom string }
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Proxy != "direct" || got.ProxyCustom != "http://me:pw@proxy.example:8080" {
		t.Fatalf("GET /api/settings/proxy: %+v", got)
	}
}

// The OTLP headers' values are set on their own: a save of the settings is
// told none of them and keeps what is saved, and the page reads and writes
// them through their route.
func TestSettingsOTelHeadersOnTheirOwn(t *testing.T) {
	sandboxHome(t)
	seedMasked(t)
	srv := Handler(nil, nil)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		var rd *strings.Reader
		if body == "" {
			rd = strings.NewReader("")
		} else {
			rd = strings.NewReader(body)
		}
		srv.ServeHTTP(rec, httptest.NewRequest(method, path, rd))
		return rec
	}
	saved := func() map[string]string {
		t.Helper()
		rec := call("GET", "/api/settings/otel-headers", "")
		if rec.Code != 200 {
			t.Fatalf("GET headers: %d %s", rec.Code, rec.Body)
		}
		var out struct {
			Headers map[string]string `json:"headers"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.Headers
	}
	// the page's echo of the settings, the names alone: what is saved stands
	body := `{"theme":"dark","otel":{"enabled":true,"endpoint":"http://collector.example:4318","headers":{"Authorization":""}}}`
	if rec := call("POST", "/api/settings", body); rec.Code != 200 {
		t.Fatalf("POST settings: %d %s", rec.Code, rec.Body)
	}
	if got := saved()["Authorization"]; got != "Bearer otel-token" {
		t.Fatalf("a settings save wrote over a header value: %q", got)
	}
	if got := saved()["X-Trace"]; got != "on" {
		t.Fatalf("a settings save lost a header: %q", got)
	}
	// the route sets them, for good or refused as the OTLP check says
	if rec := call("POST", "/api/settings/otel-headers", `{"headers":{"Authorization":"Bearer otel-token-2","X-Trace":"off"}}`); rec.Code != 200 {
		t.Fatalf("POST headers: %d %s", rec.Code, rec.Body)
	}
	if got := settings.Load().OTel.Headers; got["Authorization"] != "Bearer otel-token-2" || got["X-Trace"] != "off" {
		t.Fatalf("the route did not set them: %v", got)
	}
	if rec := call("POST", "/api/settings/otel-headers", `{"headers":{"Bad Header":"x"}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("an invalid header was kept: %d %s", rec.Code, rec.Body)
	}
}

// The masking allowlist the page edits as shown (masked) keeps the values
// it was: a masked one stands for its value, a new one is new.
func TestSettingsRedactAllowRestoresMasked(t *testing.T) {
	sandboxHome(t)
	seedMasked(t)
	srv := Handler(nil, nil)
	post := func(allow string) {
		t.Helper()
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest("POST", "/api/settings/redact-allow",
			strings.NewReader(`{"allow":`+allow+`}`)))
		if rec.Code != 200 {
			t.Fatalf("POST redact-allow %s: %d %s", allow, rec.Code, rec.Body)
		}
	}
	post(`["ops@…e.io","new@acme.io"]`)
	if got := settings.Load().RedactAllow; len(got) != 2 || got[0] != "ops@acme.io" || got[1] != "new@acme.io" {
		t.Fatalf("a masked value was not the value it was: %q", got)
	}
	post(`["gone@acme.io"]`)
	if got := settings.Load().RedactAllow; len(got) != 1 || got[0] != "gone@acme.io" {
		t.Fatalf("the list was not replaced: %q", got)
	}
}
