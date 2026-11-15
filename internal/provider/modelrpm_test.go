package provider

import (
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/settings"
)

// modelRPMHome is one relay of the user's own serving two models, with a
// MaxRPM of its key's to fall back to.
func modelRPMHome(t *testing.T) {
	t.Helper()
	azureHome(t)
	base := "https://relay.example.invalid/v1"
	if err := Save(Provider{ID: "relay", Name: "Relay", Key: "sk-relay", Chat: base, MaxRPM: 50}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive("relay", base, []catalog.Model{{ID: "m"}, {ID: "free"}}); err != nil {
		t.Fatal(err)
	}
}

// giveRPM stages an RPM of one model with the editor's Save
// (SetModelPrefs), as the Names & levels row does.
func giveRPM(t *testing.T, model string, n int) {
	t.Helper()
	if err := SetModelPrefs("relay", map[string]ModelPref{model: {RPM: &n}}); err != nil {
		t.Fatal(err)
	}
}

// The editor's Names & levels sends an RPM with its Save: it is kept in
// settings' ModelRPM under the provider id and the model's own, 0 takes it
// away again, and the limit a request on the model faces is the model's
// own over the key or account's, the smaller of the two when both are set.
func TestModelRPMSaveRoundTrip(t *testing.T) {
	modelRPMHome(t)
	if n := settings.Load().ModelRPM["relay/free"]; n != 0 {
		t.Fatalf("nothing kept yet, found %d", n)
	}
	if got := findProvider(t, "relay").ModelRPMLimit("free"); got != 50 {
		t.Fatalf("with none of its own the model sends what the key's does: %d, want 50", got)
	}
	giveRPM(t, "free", 20)
	if n := settings.Load().ModelRPM["relay/free"]; n != 20 {
		t.Fatalf("saved %d, want 20", n)
	}
	if got := findProvider(t, "relay").ModelRPMLimit("free"); got != 20 {
		t.Fatalf("the model's own is the limit in force: %d, want 20", got)
	}
	// both set, the smaller: a limit only ever tightens
	giveRPM(t, "free", 80)
	if got := findProvider(t, "relay").ModelRPMLimit("free"); got != 50 {
		t.Fatalf("a model's own above the key's is no loosening: %d, want 50", got)
	}
	// a model with none of its own keeps the key's, whatever its neighbours hold
	if got := findProvider(t, "relay").ModelRPMLimit("m"); got != 50 {
		t.Fatalf("unset: %d, want the key's 50", got)
	}
	// 0 takes it away, and a limit past the bound is refused
	giveRPM(t, "free", 0)
	if n := settings.Load().ModelRPM["relay/free"]; n != 0 {
		t.Fatalf("taken away, found %d", n)
	}
	n := MaxRPMLimit + 1
	if err := SetModelPrefs("relay", map[string]ModelPref{"free": {RPM: &n}}); err == nil {
		t.Fatal("a limit past the bound was kept")
	}
	// one for a model the provider doesn't serve is never sent, and refused
	// as a name for one is (SetModelName)
	other := 20
	if err := SetModelPrefs("relay", map[string]ModelPref{"nope": {RPM: &other}}); err == nil {
		t.Fatal("a limit for a model not served was kept")
	}
	if got := findProvider(t, "relay").ModelRPMLimit("free"); got != 50 {
		t.Fatalf("a refused limit changed nothing: %d, want 50", got)
	}
}

// The user's limit goes with the model to the id it is listed by
// (SetModelID), as every per-model setting does.
func TestModelRPMKeyMovesWithID(t *testing.T) {
	modelRPMHome(t)
	giveRPM(t, "free", 20)
	setID(t, "relay/free", "free-2")
	s := settings.Load()
	if _, held := s.ModelRPM["relay/free"]; held {
		t.Fatal("the key under the old id is still there")
	}
	if n := s.ModelRPM["relay/free-2"]; n != 20 {
		t.Fatalf("relay/free-2 holds %d, want 20", n)
	}
	if got := findProvider(t, "relay").ModelRPMLimit("free-2"); got != 20 {
		t.Fatalf("the limit is in force under the new id: %d, want 20", got)
	}
}
