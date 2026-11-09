package settings

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

// Only warn and block are kept, for a kind masking names; mask, the
// default, is left out, and anything else is refused (#1977).
func TestRedactActions(t *testing.T) {
	got, err := redactActions(map[string]string{"API_KEY": "Block", " EMAIL ": "warn", "PHONE": "mask", "SSN": ""})
	if err != nil || len(got) != 2 || got["API_KEY"] != "block" || got["EMAIL"] != "warn" {
		t.Fatalf("%v %v", got, err)
	}
	if got, err := redactActions(map[string]string{"PHONE": "mask"}); err != nil || got != nil {
		t.Fatalf("all masked: %v %v", got, err)
	}
	if _, err := redactActions(map[string]string{"API_KEY": "drop"}); err == nil {
		t.Fatal("an unknown action was kept")
	}
	if _, err := redactActions(map[string]string{"api key": "warn"}); err == nil {
		t.Fatal("a kind masking can't name was kept")
	}
}

// The allow list is trimmed, each value once whatever its case, and
// bounded.
func TestRedactAllow(t *testing.T) {
	got, err := redactAllow([]string{" ops@acme.io ", "OPS@acme.io", "", "build-01"})
	if err != nil || !slices.Equal(got, []string{"ops@acme.io", "build-01"}) {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := redactAllow([]string{strings.Repeat("x", maxAllowLen+1)}); err == nil {
		t.Fatal("a value too long was kept")
	}
	many := make([]string, maxAllow+1)
	for i := range many {
		many[i] = strings.Repeat("v", i+1)
	}
	if _, err := redactAllow(many); err == nil {
		t.Fatal("too many values were kept")
	}
}

// A proxy address for the UI keeps the scheme and address, and drops the
// login before the last @, as the page's own split reads it.
func TestProxyWithoutLogin(t *testing.T) {
	for p, want := range map[string]string{
		"http://me:pw@proxy.example:8080":  "http://proxy.example:8080",
		"socks5://me@proxy.example:1080/x": "socks5://proxy.example:1080/x",
		"http://me:p@w@proxy.example":      "http://proxy.example",
		"http://proxy.example:8080/a?b#c":  "http://proxy.example:8080/a?b#c",
		"proxy.example:8080":               "proxy.example:8080",
		"direct":                           "direct",
		"":                                 "",
	} {
		if got := ProxyWithoutLogin(p); got != want {
			t.Errorf("ProxyWithoutLogin(%q) = %q, want %q", p, got, want)
		}
	}
}

// The settings the UI is told hold no secret: the GitHub token and the LAN
// key whole, the proxy and the last custom one's login, the OTLP header
// values and the masking allowlist's values are gone, and nothing of the
// original is rewritten under the masked copy.
func TestSettingsForUI(t *testing.T) {
	s := Settings{
		Proxy:       "http://me:pw@proxy.example:8080",
		ProxyCustom: "socks5://me@proxy.example:1080",
		GitHubToken: "gh-token-value",
		LANKey:      "lan-key-value",
		OTel:        OTel{Headers: map[string]string{"Authorization": "Bearer otel-token", "X-Trace": "on"}},
		RedactAllow: []string{"ops@acme.io", "build-01"},
	}
	u := s.ForUI()
	if u.Proxy != "http://proxy.example:8080" || u.ProxyCustom != "socks5://proxy.example:1080" {
		t.Fatalf("proxy with its login: %q %q", u.Proxy, u.ProxyCustom)
	}
	if u.GitHubToken != "" || u.LANKey != "" {
		t.Fatalf("a token went out whole: %q %q", u.GitHubToken, u.LANKey)
	}
	if !maps.Equal(u.OTel.Headers, map[string]string{"Authorization": "", "X-Trace": ""}) {
		t.Fatalf("OTLP header values went out: %v", u.OTel.Headers)
	}
	if !slices.Equal(u.RedactAllow, []string{MaskAllow("ops@acme.io"), MaskAllow("build-01")}) {
		t.Fatalf("allow values went out whole: %v", u.RedactAllow)
	}
	// the masked copy holds nothing of the original's maps and slices
	if !maps.Equal(s.OTel.Headers, map[string]string{"Authorization": "Bearer otel-token", "X-Trace": "on"}) {
		t.Fatalf("ForUI rewrote the saved headers: %v", s.OTel.Headers)
	}
	if !slices.Equal(s.RedactAllow, []string{"ops@acme.io", "build-01"}) {
		t.Fatalf("ForUI rewrote the saved allow list: %v", s.RedactAllow)
	}
}

// An allowlist value shown masked keeps its shape whatever it is, and the
// page sending it back gets the value it was.
func TestMaskAllowAndUnmaskAllow(t *testing.T) {
	if got := MaskAllow("ops@acme.io"); got != "ops@…e.io" {
		t.Fatalf("MaskAllow = %q", got)
	}
	if got := MaskAllow("build-01"); got != "••••••••" {
		t.Fatalf("MaskAllow of a short value = %q", got)
	}
	had := []string{"ops@acme.io", "build-01", "zz@acme.io"}
	// masked ones restored, a written one kept, a new one sent as it is,
	// a dropped one gone
	sent := []string{"ops@…e.io", "build-01", "zz@a…e.io", "new@acme.io"}
	if got := UnmaskAllow(had, sent); !slices.Equal(got, []string{"ops@acme.io", "build-01", "zz@acme.io", "new@acme.io"}) {
		t.Fatalf("UnmaskAllow = %q", got)
	}
	// one masked value dropped of two the same shape: one left
	if got := UnmaskAllow([]string{"aa-1", "bb-2"}, []string{"••••"}); !slices.Equal(got, []string{"aa-1"}) {
		t.Fatalf("UnmaskAllow of a dropped twin = %q", got)
	}
	// a masked form standing for a value already taken once is still
	// itself, never a masked form written over the list
	if got := UnmaskAllow([]string{"ops@acme.io"}, []string{"ops@…e.io", "ops@…e.io"}); !slices.Equal(got, []string{"ops@acme.io", "ops@…e.io"}) {
		t.Fatalf("UnmaskAllow of a doubled mask = %q", got)
	}
	if got := UnmaskAllow(nil, []string{" ", "a"}); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("UnmaskAllow of an empty list = %q", got)
	}
}
