package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// /api/providers gives each provider's last list change back, as the row and
// the editor say it: the models the vendor's list gained and dropped, and
// when. A provider no fetch changed carries none, and the editor's Save,
// which sends no listChange, keeps the one a fetch recorded.
func TestProvidersJSONCarriesTheListChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	const u = "http://127.0.0.1:1/v1"
	at := time.Date(2026, 11, 17, 9, 30, 0, 0, time.UTC)
	if err := provider.Save(provider.Provider{
		ID: "relay", Name: "Relay", Chat: u, Key: "sk-a",
		ListChange: &provider.ModelChange{Added: []string{"model-c"}, Removed: []string{"model-b"}, At: at},
	}); err != nil {
		t.Fatal(err)
	}
	if err := provider.Save(provider.Provider{ID: "steady", Name: "Steady", Chat: u, Key: "sk-b"}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	do := func(method, path, body string) string {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		return w.Body.String()
	}
	read := func() map[string]providerRowListChange {
		t.Helper()
		var out struct {
			Providers []providerRowListChange `json:"providers"`
		}
		if err := json.Unmarshal([]byte(do("GET", "/api/providers", "")), &out); err != nil {
			t.Fatal(err)
		}
		by := map[string]providerRowListChange{}
		for _, p := range out.Providers {
			by[p.ID] = p
		}
		return by
	}
	got := read()
	relay, ok := got["relay"]
	if !ok || relay.ListChange == nil {
		t.Fatalf("relay's listChange absent: %+v", relay)
	}
	if len(relay.ListChange.Added) != 1 || relay.ListChange.Added[0] != "model-c" ||
		len(relay.ListChange.Removed) != 1 || relay.ListChange.Removed[0] != "model-b" || !relay.ListChange.At.Equal(at) {
		t.Fatalf("relay's listChange: %+v", *relay.ListChange)
	}
	if steady := got["steady"]; steady.ListChange != nil {
		t.Fatalf("a provider with no change carried one: %+v", *steady.ListChange)
	}
	if body := do("GET", "/api/providers", ""); strings.Contains(body, `"listChange":{"added":[],"removed":[]`) {
		t.Fatalf("an empty change was sent: %s", body)
	}

	// the editor's Save sends no listChange: the recorded one stays
	do("POST", "/api/provider/save", `{"id":"relay","from":"relay","name":"Relay 2","chat":"`+u+`","key":"sk-a"}`)
	relay = read()["relay"]
	if relay.ListChange == nil || len(relay.ListChange.Removed) != 1 || relay.ListChange.Removed[0] != "model-b" {
		t.Fatalf("an editor save dropped the list change: %+v", relay)
	}
}

// providerRowListChange is the part of providerJSON this test reads.
type providerRowListChange struct {
	ID         string                `json:"id"`
	ListChange *provider.ModelChange `json:"listChange"`
}
