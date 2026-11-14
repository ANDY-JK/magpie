package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// capsSandbox is a home with a codex signed in, and a relay of two models.
func capsSandbox(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("model = \"magpie/relay/m1\"\n"), 0o600)
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m1", "m2"}}); err != nil {
		t.Fatal(err)
	}
	if err := provider.SetCaps("relay", map[string]provider.ModelCaps{"m1": {Tools: "yes", Vision: "no", Stream: "yes"}}); err != nil {
		t.Fatal(err)
	}
}

// The agent model selector's payload says what Test capabilities settled of
// each model, and says nothing of one not tested.
func TestModelCapsAgentsListSaysThem(t *testing.T) {
	capsSandbox(t)
	mux := http.NewServeMux()
	agentModelsAPI(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/agent-models/codex", nil))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var got struct {
		Models []agentModelJSON
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	caps := map[string]*provider.ModelCaps{}
	for _, m := range got.Models {
		if _, ok := caps[m.ID]; ok || !strings.HasPrefix(m.ID, "relay/") {
			continue
		}
		caps[m.ID] = m.Caps
	}
	if c := caps["relay/m1"]; c == nil || *c != (provider.ModelCaps{Tools: "yes", Vision: "no", Stream: "yes"}) {
		t.Fatalf("relay/m1 = %v, want the letters it settled", caps["relay/m1"])
	}
	if c := caps["relay/m2"]; c != nil {
		t.Fatalf("relay/m2 = %v, want none for a model not tested", c)
	}
}

// The Routing page's member rows and the Providers page (/api/providers)
// read the same letters.
func TestModelCapsRoutingAndProvidersSayThem(t *testing.T) {
	capsSandbox(t)
	if err := provider.SaveGroup(provider.Group{ID: "g", Name: "G", Members: []string{"relay/m1", "relay/m2"}}); err != nil {
		t.Fatal(err)
	}
	var info *memberJSON
	for _, g := range groupsState().Groups {
		if g.ID != "g" {
			continue
		}
		for i := range g.Info {
			if g.Info[i].ID == "relay/m1" {
				info = &g.Info[i]
			} else if g.Info[i].ID == "relay/m2" && g.Info[i].Caps != nil {
				t.Fatalf("relay/m2 memberInfo carries %v, want none for a model not tested", g.Info[i].Caps)
			}
		}
	}
	if info == nil {
		t.Fatal("no relay/m1 among the group's members")
	}
	if info.Caps == nil || *info.Caps != (provider.ModelCaps{Tools: "yes", Vision: "no", Stream: "yes"}) {
		t.Fatalf("relay/m1 memberInfo = %v, want the letters it settled", info.Caps)
	}
	found := false
	for _, p := range providersState().Providers {
		if p.ID != "relay" {
			continue
		}
		found = true
		if p.Caps["m1"] != (provider.ModelCaps{Tools: "yes", Vision: "no", Stream: "yes"}) {
			t.Fatalf("/api/providers caps %v, want m1's letters", p.Caps)
		}
		if _, ok := p.Caps["m2"]; ok {
			t.Fatalf("/api/providers caps %v, want nothing for m2", p.Caps)
		}
	}
	if !found {
		t.Fatal("no relay in /api/providers")
	}
}

// The caps action keeps a run's letters with the provider, and a model the
// run settled nothing of keeps what was kept of it.
func TestModelCapsActionKeepsTheRun(t *testing.T) {
	capsSandbox(t)
	if err := provider.SetCaps("relay", map[string]provider.ModelCaps{"m2": {Stream: "no"}}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	post := func(action, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/provider/"+action, strings.NewReader(body)))
		return w
	}
	if w := post("caps", `{"id":"relay","capsResults":{"m1":{"tools":"no","vision":"yes","stream":"no"}}}`); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	p, err := provider.Find("relay")
	if err != nil {
		t.Fatal(err)
	}
	if p.Caps["m1"] != (provider.ModelCaps{Tools: "no", Vision: "yes", Stream: "no"}) {
		t.Fatalf("m1 = %v, want this run's word", p.Caps["m1"])
	}
	if p.Caps["m2"] != (provider.ModelCaps{Stream: "no"}) {
		t.Fatalf("m2 = %v, want what was kept of it", p.Caps["m2"])
	}
	// an unknown provider is refused, not written
	if w := post("caps", `{"id":"nope","capsResults":{"m1":{"tools":"yes"}}}`); w.Code == 200 {
		t.Fatalf("caps of a provider there isn't: %s", w.Body)
	}
}
