package gui

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/profile"
)

// B-05 (the review): applying a profile no profile has read as success with
// 0 changed, so a mistyped or stale name silently did nothing. It is
// refused, and a saved name still applies.
func TestProfileUseRefusesAMissingName(t *testing.T) {
	home := sandboxHome(t)
	t.Setenv("CODEX_HOME", home+"/.codex")
	h := Handler(nil, nil)
	call := func(body string) (int, string) {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/profile/use", strings.NewReader(body)))
		return rec.Code, rec.Body.String()
	}
	if code, body := call(`{"name":"nope"}`); code == 200 || !strings.Contains(body, "no profile named") {
		t.Fatalf("a missing profile: %d %s", code, body)
	}
	if code, body := call(`{"name":""}`); code == 200 || !strings.Contains(body, "no profile named") {
		t.Fatalf("an empty name: %d %s", code, body)
	}
	if err := profile.Save("work", profile.Profile{}); err != nil {
		t.Fatal(err)
	}
	if code, body := call(`{"name":"work"}`); code != 200 {
		t.Fatalf("a saved profile: %d %s", code, body)
	}
}
