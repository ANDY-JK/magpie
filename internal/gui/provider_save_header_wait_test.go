package gui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// F-02: how many minutes a provider's vendors are given to begin answering
// is set in its editor and saved; a save that leaves it out keeps it, null
// puts it back to magpie's 10, and 0 or 1441 are refused rather than saved.
func TestProviderSaveHeaderWait(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	post := func(body string) (int, string) {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/provider/save", strings.NewReader(body)))
		return w.Code, w.Body.String()
	}
	const base = `"id":"relay","name":"Relay","key":"sk-r","chat":"http://127.0.0.1:1/v1","icon":"generic"`
	wait := func(id string) *int {
		t.Helper()
		p, err := provider.Find(id)
		if err != nil {
			t.Fatal(err)
		}
		return p.HeaderWait
	}

	if code, body := post(`{` + base + `,"new":true}`); code != 200 {
		t.Fatalf("add: %d %s", code, body)
	}
	if got := wait("relay"); got != nil {
		t.Fatalf("a new provider waits magpie's 10, got %d set", *got)
	}

	if code, body := post(`{` + base + `,"from":"relay","headerWait":5}`); code != 200 {
		t.Fatalf("save 5: %d %s", code, body)
	}
	if got := wait("relay"); got == nil || *got != 5 {
		t.Fatalf("saved 5, got %v", got)
	}

	// left out: kept, not reset
	if code, body := post(`{` + base + `,"from":"relay"}`); code != 200 {
		t.Fatalf("save leaving it out: %d %s", code, body)
	}
	if got := wait("relay"); got == nil || *got != 5 {
		t.Fatalf("a save without it reset the wait to %v", got)
	}

	// 0 is not a wait: refused, and what was set stays
	if code, body := post(`{` + base + `,"from":"relay","headerWait":0}`); code == 200 || !strings.Contains(body, "1 to 1440") {
		t.Fatalf("0: %d %s", code, body)
	}
	if code, body := post(`{` + base + `,"from":"relay","headerWait":1441}`); code == 200 || !strings.Contains(body, "1 to 1440") {
		t.Fatalf("1441: %d %s", code, body)
	}
	if got := wait("relay"); got == nil || *got != 5 {
		t.Fatalf("a refused save changed the wait to %v", got)
	}

	// null: back to magpie's 10
	if code, body := post(`{` + base + `,"from":"relay","headerWait":null}`); code != 200 {
		t.Fatalf("save null: %d %s", code, body)
	}
	if got := wait("relay"); got != nil {
		t.Fatalf("null left %d set", *got)
	}
}
