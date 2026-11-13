package provider

import (
	"slices"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/settings"
)

// The provider editor's Names & levels are made with its Save, all at
// once: each is made as SetModelName, SetModelEfforts and SetModelImage
// make it, and the agents' files are rewritten once, not once a change
// (ARNO on Discord: every reasoning level ticked was applied on its own,
// so picking them lagged).
func TestSetModelPrefs(t *testing.T) {
	prefsHome(t)
	touched := 0
	catalog.Changed = func() { touched++ }
	t.Cleanup(func() { catalog.Changed = nil })
	name, yes := "My Sol", true
	efforts := []string{"low", "high"}
	if err := SetModelPrefs("a", map[string]ModelPref{"sol": {Name: &name, Efforts: &efforts, Images: &yes}}); err != nil {
		t.Fatal(err)
	}
	if touched != 1 {
		t.Fatalf("agents told %d times, want once", touched)
	}
	s := settings.Load()
	if s.ModelNames["a/sol"] != "My Sol" || !slices.Equal(s.ModelEfforts["a/sol"], efforts) || !s.ModelImages["a/sol"] {
		t.Fatalf("names %v, efforts %v, images %v", s.ModelNames, s.ModelEfforts, s.ModelImages)
	}
	if _, ok := s.ModelNames["b/sol"]; ok {
		t.Fatal("b's sol was named too")
	}
	// nothing changed: nothing to tell
	if err := SetModelPrefs("a", map[string]ModelPref{"sol": {Name: &name}}); err != nil || touched != 1 {
		t.Fatalf("%v, told %d times", err, touched)
	}
	// Restore default: its own name, every level, the vendor's answer
	own, all := "", []string{}
	if err := SetModelPrefs("a", map[string]ModelPref{"sol": {Name: &own, Efforts: &all, OwnImages: true}}); err != nil {
		t.Fatal(err)
	}
	s = settings.Load()
	if len(s.ModelNames)+len(s.ModelEfforts)+len(s.ModelImages) != 0 || touched != 2 {
		t.Fatalf("names %v, efforts %v, images %v, told %d times", s.ModelNames, s.ModelEfforts, s.ModelImages, touched)
	}
	// a level it hasn't is refused, the change before it still told
	bad := []string{"xhigh"}
	err := SetModelPrefs("a", map[string]ModelPref{"sol": {Name: &name, Efforts: &bad}})
	if err == nil || !strings.Contains(err.Error(), "xhigh") {
		t.Fatalf("got %v", err)
	}
	if settings.Load().ModelNames["a/sol"] != "My Sol" || touched != 3 {
		t.Fatalf("names %v, told %d times", settings.Load().ModelNames, touched)
	}
}

// The wire name the Names & levels say beside the name is made with the
// save too, as `magpie model wire` gives it: kept under the id the model
// is listed by, taken away by a blank one, moved with it when the same
// save lists the model by another id, and refused for a model the
// provider does not serve — that gate is SetUpstreamName's, not a second
// one here. Like a price it tells the agents nothing: which model an
// agent picks, and what it is called, have not changed.
func TestSetModelPrefsWiresTheModel(t *testing.T) {
	prefsHome(t)
	touched := 0
	catalog.Changed = func() { touched++ }
	t.Cleanup(func() { catalog.Changed = nil })
	wire := "vendor-c/sol"
	name := "My Sol"
	if err := SetModelPrefs("a", map[string]ModelPref{"sol": {Wire: &wire}}); err != nil {
		t.Fatal(err)
	}
	if got := settings.Load().ModelWires["a/sol"]; got != "vendor-c/sol" {
		t.Fatalf("wire %q", got)
	}
	if touched != 0 {
		t.Fatalf("agents told %d times, want none", touched)
	}
	// "" asks for it by the name magpie knows it by again
	blank := ""
	if err := SetModelPrefs("a", map[string]ModelPref{"sol": {Wire: &blank}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := settings.Load().ModelWires["a/sol"]; ok {
		t.Fatal("a blank wire was kept")
	}
	// a name for a model the provider does not serve is refused, and
	// nothing of it is stored
	if err := SetModelPrefs("a", map[string]ModelPref{"nope": {Wire: &wire}}); err == nil || !strings.Contains(err.Error(), "no model nope") {
		t.Fatalf("got %v", err)
	}
	if _, ok := settings.Load().ModelWires["a/nope"]; ok {
		t.Fatal("a refused wire was stored anyway")
	}
	// a save that stops at a refused wire has told the agents of the
	// changes made before it: sol comes before zope
	if err := SetModelPrefs("a", map[string]ModelPref{"sol": {Name: &name}, "zope": {Wire: &wire}}); err == nil || !strings.Contains(err.Error(), "no model zope") {
		t.Fatalf("got %v", err)
	}
	if settings.Load().ModelNames["a/sol"] != "My Sol" || touched != 1 {
		t.Fatalf("names %v, told %d times", settings.Load().ModelNames, touched)
	}
	// the wire goes with the model to the id the same save lists it by
	listed := "sol-1017"
	if err := SetModelPrefs("a", map[string]ModelPref{"sol": {Wire: &wire, ID: &listed}}); err != nil {
		t.Fatal(err)
	}
	s := settings.Load()
	if got := s.ModelWires["a/sol-1017"]; got != "vendor-c/sol" {
		t.Fatalf("wire after the id %v", s.ModelWires)
	}
	if _, ok := s.ModelWires["a/sol"]; ok {
		t.Fatal("the wire stayed under the id the model had")
	}
}
