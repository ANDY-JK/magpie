package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/testenv"
)

// capsHome is a sandbox home with a models.dev cache that names one model.
func capsHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	if runtime.GOOS != "windows" {
		bin := t.TempDir()
		testenv.Program(t, filepath.Join(bin, "security"), "#!/bin/sh\nexit 44\n")
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	os.MkdirAll(filepath.Dir(catalog.CachePath()), 0o755)
	data := `{"relay":{"models":{"sol":{"id":"sol","name":"Sol"}}}}`
	if err := os.WriteFile(catalog.CachePath(), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// relaySaved is a provider of two picked models saved for the caps tests.
func relaySaved(t *testing.T) {
	t.Helper()
	if err := Save(Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m1", "m2"}}); err != nil {
		t.Fatal(err)
	}
}

// capsOnDisk is the caps providers.json keeps for the provider, as its
// entry carries them.
func capsOnDisk(t *testing.T, id string) map[string]ModelCaps {
	t.Helper()
	b, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Providers []Provider
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	for _, p := range f.Providers {
		if p.ID == id {
			return p.Caps
		}
	}
	t.Fatalf("no %s in providers.json", id)
	return nil
}

// A Test capabilities run's letters are kept with the provider
// (providers.json), each the word the endpoint said; a word it didn't say
// is no letter.
func TestModelCapsSavedBesideTheProvider(t *testing.T) {
	capsHome(t)
	relaySaved(t)
	if err := SetCaps("relay", map[string]ModelCaps{
		"m1": {Tools: "yes", Vision: "no", Stream: "yes"},
		"m2": {Stream: "no"},
	}); err != nil {
		t.Fatal(err)
	}
	got := capsOnDisk(t, "relay")
	if got["m1"] != (ModelCaps{Tools: "yes", Vision: "no", Stream: "yes"}) {
		t.Fatalf("m1 = %v", got["m1"])
	}
	if got["m2"] != (ModelCaps{Stream: "no"}) {
		t.Fatalf("m2 = %v", got["m2"])
	}
	if len(got) != 2 {
		t.Fatalf("caps %v", got)
	}
	// the run's word now is what is kept for it: a re-test that said less
	// takes the old letters off
	if err := SetCaps("relay", map[string]ModelCaps{"m1": {Tools: "no"}}); err != nil {
		t.Fatal(err)
	}
	if got = capsOnDisk(t, "relay"); got["m1"] != (ModelCaps{Tools: "no"}) {
		t.Fatalf("re-tested m1 = %v, want this run's word alone", got["m1"])
	}
	if got["m2"] != (ModelCaps{Stream: "no"}) {
		t.Fatalf("m2 = %v, want what was kept of it", got["m2"])
	}
	// a run that settled nothing says nothing, and keeps what was kept
	before := capsOnDisk(t, "relay")
	if err := SetCaps("relay", map[string]ModelCaps{"m1": {Tools: "maybe"}}); err != nil {
		t.Fatal(err)
	}
	if got = capsOnDisk(t, "relay"); got["m1"] != before["m1"] {
		t.Fatalf("m1 = %v, want what was kept (a word the endpoint didn't settle)", got["m1"])
	}
}

// A save that carries no caps — the editor's sends none, as does a save of
// another setting — keeps what was kept of every model; a model the user
// deleted from the picks takes its letters with it, and one set aside
// (delisted, to come back with its list) keeps them.
func TestModelCapsKeptBesideASaveWithoutThem(t *testing.T) {
	capsHome(t)
	relaySaved(t)
	if err := SetCaps("relay", map[string]ModelCaps{"m1": {Tools: "yes"}, "m2": {Vision: "no"}}); err != nil {
		t.Fatal(err)
	}
	// the editor's save: the fields it sends, and none of the caps
	if err := Save(Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m1", "m2"}}); err != nil {
		t.Fatal(err)
	}
	got := capsOnDisk(t, "relay")
	if got["m1"] != (ModelCaps{Tools: "yes"}) || got["m2"] != (ModelCaps{Vision: "no"}) {
		t.Fatalf("caps after a save without them: %v", got)
	}
	// m1 unpicked is deleted: its letters go with it
	if err := Save(Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m2"}}); err != nil {
		t.Fatal(err)
	}
	if got = capsOnDisk(t, "relay"); (got["m1"] != ModelCaps{}) {
		t.Fatalf("m1 is gone from the picks, its caps %v came with it", got["m1"])
	}
	if got["m2"] != (ModelCaps{Vision: "no"}) {
		t.Fatalf("m2 = %v, want what was kept of it", got["m2"])
	}
	// one set aside is not deleted: relist brings it back with its letters
	if err := Save(Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m1"}, Delisted: []string{"m1"}}); err != nil {
		t.Fatal(err)
	}
	if err := SetCaps("relay", map[string]ModelCaps{"m1": {Tools: "yes"}}); err != nil {
		t.Fatal(err)
	}
	if err := Save(Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "http://127.0.0.1:1/v1", Delisted: []string{"m1"}}); err != nil {
		t.Fatal(err)
	}
	if got = capsOnDisk(t, "relay"); got["m1"] != (ModelCaps{Tools: "yes"}) {
		t.Fatalf("delisted m1 = %v, want its letters kept for coming back", got["m1"])
	}
	// the vendor's list served (no picks): what is deleted isn't magpie's
	// to say, so nothing is dropped
	if err := Save(Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "http://127.0.0.1:1/v1"}); err != nil {
		t.Fatal(err)
	}
	if got = capsOnDisk(t, "relay"); got["m1"] != (ModelCaps{Tools: "yes"}) {
		t.Fatalf("no picks: m1 = %v, want its letters kept", got["m1"])
	}
}

// A letter no answer said — "maybe" in a file written by hand — is a word
// the endpoint didn't settle: it reads as no letter, beside the ones it
// did say.
func TestModelCapsReadAsUnknown(t *testing.T) {
	capsHome(t)
	relaySaved(t)
	b, err := json.Marshal(file{Providers: []Provider{{ID: "relay", Name: "Relay", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m1", "m2"},
		Caps: map[string]ModelCaps{"m1": {Tools: "maybe", Vision: "no", Stream: "yes"}, "m2": {Tools: ""}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(), b, 0o600); err != nil {
		t.Fatal(err)
	}
	Changed()
	p, err := Find("relay")
	if err != nil {
		t.Fatal(err)
	}
	got := p.Caps["m1"]
	if got != (ModelCaps{Vision: "no", Stream: "yes"}) {
		t.Fatalf("m1 = %v, want the letters the answer said and not the word it didn't", got)
	}
	if _, ok := p.Caps["m2"]; ok {
		t.Fatalf("m2 = %v, want a model settled nothing of left out", p.Caps["m2"])
	}
}

// What was settled reaches the agents' lists (the pickers' rows say it) and
// follows the account (withPicks, and a copy of the provider carries its
// own letters alone).
func TestModelCapsOnEntriesAndAccounts(t *testing.T) {
	capsHome(t)
	relaySaved(t)
	if err := SetCaps("relay", map[string]ModelCaps{"m1": {Tools: "yes", Stream: "no"}}); err != nil {
		t.Fatal(err)
	}
	listed, _ := ListedFor("codex")
	found := map[string]*ModelCaps{}
	for _, e := range listed {
		if e.Provider.ID == "relay" {
			found[e.Model] = e.Caps
		}
	}
	if c := found["m1"]; c == nil || *c != (ModelCaps{Tools: "yes", Stream: "no"}) {
		t.Fatalf("m1's entry = %v, want the letters it settled", found["m1"])
	}
	if c := found["m2"]; c != nil {
		t.Fatalf("m2's entry = %v, want none for a model not tested", c)
	}
	// a signed-in account keeps them as its picks do
	pk, err := Find("relay")
	if err != nil {
		t.Fatal(err)
	}
	a := withPicks(Provider{ID: "relay"}, *pk)
	if a.Caps["m1"] != (ModelCaps{Tools: "yes", Stream: "no"}) {
		t.Fatalf("account caps %v, want what the provider keeps", a.Caps)
	}
	// a copy carries its letters alone, not the one it was copied from
	q := pk.clone()
	q.Caps["m1"] = ModelCaps{Tools: "no"}
	if pk.Caps["m1"] != (ModelCaps{Tools: "yes", Stream: "no"}) {
		t.Fatalf("the copy changed the provider's: %v", pk.Caps["m1"])
	}
}

// A model the provider's picks don't hold is tested all the same when the
// vendor's list serves it (no picks): its letters are kept, and the served
// entries say them.
func TestModelCapsOfTheVendorsList(t *testing.T) {
	capsHome(t)
	relaySaved(t)
	if err := Save(Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "http://127.0.0.1:1/v1", Catalog: "relay"}); err != nil {
		t.Fatal(err)
	}
	if err := SetCaps("relay", map[string]ModelCaps{"sol": {Tools: "yes"}}); err != nil {
		t.Fatal(err)
	}
	listed, _ := ListedFor("codex")
	i := slices.IndexFunc(listed, func(e Entry) bool { return e.Provider.ID == "relay" && e.Model == "sol" })
	if i < 0 {
		t.Fatalf("no relay/sol among %+v", listed)
	}
	if c := listed[i].Caps; c == nil || c.Tools != "yes" {
		t.Fatalf("relay/sol caps %v, want tools yes", listed[i].Caps)
	}
}
