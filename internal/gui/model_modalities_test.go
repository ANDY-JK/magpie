package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
)

// The Providers page's model rows say what the model's source listed it
// takes and gives and when it was released (catalog.Model's
// InputModalities, OutputModalities, Released), which the editor's chip
// hover reads: till now the row carried Images alone, the one bool agents
// are told, so the hover could not say more. A model its source says
// nothing of carries no key at all, not an empty list. What the page says
// is what the catalog agents are told carries, whose Inputs, Outputs and
// Released are kept out of its JSON by json:"-" tags (Entry), so the page
// is where the browser gets them.
func TestProviderModelModalitiesSaid(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755)
	os.WriteFile(catalog.CachePath(), []byte(`{
	  "zai":{"id":"zai","models":{
	    "glm-5.3-flash":{"id":"glm-5.3-flash","name":"GLM-5.3-Flash","release_date":"2025-11-04","modalities":{"input":["text","image","pdf"],"output":["text"]},"limit":{"context":1000000,"output":131072}},
	    "plain":{"id":"plain","name":"Plain","limit":{"context":128000,"output":8192}}}}}`), 0o644)
	catalog.Reset()
	t.Cleanup(catalog.Reset)
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "http://127.0.0.1:1/v1", Catalog: "zai", Models: []string{"glm-5.3-flash", "plain"}}); err != nil {
		t.Fatal(err)
	}
	p, err := provider.Find("relay")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]modelJSON{}
	for _, m := range providerInfo(*p, nil).Models {
		byID[m.ID] = m
	}
	said, known := byID["glm-5.3-flash"], byID["plain"]
	if len(byID) != 2 || said.ID == "" || known.ID == "" {
		t.Fatalf("models %+v", byID)
	}
	if !slices.Equal(said.Inputs, []string{"text", "image", "pdf"}) || !slices.Equal(said.Outputs, []string{"text"}) {
		t.Errorf("glm-5.3-flash: takes %+v, gives %+v; want text+image+pdf in, text out", said.Inputs, said.Outputs)
	}
	if said.Released != "2025-11-04" {
		t.Errorf("glm-5.3-flash: released %q, want 2025-11-04", said.Released)
	}
	if known.Inputs != nil || known.Outputs != nil || known.Released != "" {
		t.Errorf("plain: takes %+v, gives %+v, released %q; want none said", known.Inputs, known.Outputs, known.Released)
	}
	// the JSON the page serves the browser carries the three under their
	// own keys, omitted for a model its source says nothing of
	shapeJSON := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	shape := func(m modelJSON) map[string]any {
		t.Helper()
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	j := shape(said)
	for k, want := range map[string]any{"inputs": []any{"text", "image", "pdf"}, "outputs": []any{"text"}, "released": "2025-11-04"} {
		got, ok := j[k]
		if !ok {
			t.Errorf("glm-5.3-flash: the page's JSON has no %q", k)
			continue
		}
		gb, wb := shapeJSON(got), shapeJSON(want)
		if gb != wb {
			t.Errorf("glm-5.3-flash: %q is %v, want %v", k, got, want)
		}
	}
	for _, k := range []string{"inputs", "outputs", "released"} {
		if _, ok := shape(known)[k]; ok {
			t.Errorf("plain: the page's JSON says %q, which its source does not", k)
		}
	}
	// what the page says is what agents are told: the catalog's entries
	// carry the same, kept from its JSON by json:"-" tags
	for _, e := range provider.Served() {
		if e.Model == "glm-5.3-flash" && (!slices.Equal(e.Inputs, said.Inputs) || !slices.Equal(e.Outputs, said.Outputs) || e.Released != said.Released) {
			t.Errorf("agents are told takes %+v, gives %+v, released %q; the page says %+v, %+v, %q", e.Inputs, e.Outputs, e.Released, said.Inputs, said.Outputs, said.Released)
		}
	}
}
