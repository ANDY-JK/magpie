package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// The editor's save sets a provider's requests a day, shows it in the
// providers' state, keeps it when a save leaves it out, takes 0 or null for
// no limit, and refuses one out of range.
func TestDayCallsSave(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := provider.Save(provider.Provider{ID: "dc", Name: "Daily", Chat: "https://example.invalid/v1", Key: "k"}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	post := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/provider/save", strings.NewReader(body)))
		return w
	}
	dayCalls := func() int {
		got, err := provider.Find("dc")
		if err != nil {
			t.Fatal(err)
		}
		return got.DayCalls
	}
	const base = `{"id":"dc","name":"Daily","chat":"https://example.invalid/v1"`
	if w := post(base + `,"dayCalls":500}`); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if n := dayCalls(); n != 500 {
		t.Fatalf("saved %d", n)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/providers", nil))
	var state providersJSON
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	shown := -1
	for _, q := range state.Providers {
		if q.ID == "dc" {
			shown = q.DayCalls
		}
	}
	if shown != 500 {
		t.Fatalf("shown %d", shown)
	}
	if w := post(base + `}`); w.Code != 200 || dayCalls() != 500 {
		t.Fatalf("an older form: %d %s, limit %d", w.Code, w.Body, dayCalls())
	}
	for _, bad := range []string{`"dayCalls":-1`, `"dayCalls":100001`, `"dayCalls":"x"`, `"dayCalls":2.5`} {
		if w := post(base + `,` + bad + `}`); w.Code == 200 {
			t.Fatalf("%s taken", bad)
		}
	}
	if dayCalls() != 500 {
		t.Fatalf("a refused save changed it: %d", dayCalls())
	}
	if w := post(base + `,"dayCalls":null}`); w.Code != 200 || dayCalls() != 0 {
		t.Fatalf("null: %d %s, limit %d", w.Code, w.Body, dayCalls())
	}
}
