package agent

import (
	"errors"
	"path/filepath"
	"testing"
)

// A round of SyncCatalog that can't write a model list keeps the failure,
// not drops it: a failure no caller carries is in syncFailures, said once
// per distinct error, and a sync that writes again drops its failures.
func TestSyncFailureKeptAndSaid(t *testing.T) {
	home := syncHome(t)
	// Pi's files, as TestSyncCatalogRewritesAgentLists lays them: a list
	// magpie wrote, and settings pointing at magpie, so Pi's sync runs at
	// all (a ListOnly agent's sync, the round's, runs only while wired)
	piModels := filepath.Join(home, ".pi", "agent", "models.json")
	writeFile(t, piModels, `{"providers":{"mine":{"baseUrl":"http://x"},"magpie":{"name":"magpie","models":[]}}}`)
	writeFile(t, filepath.Join(home, ".pi", "agent", "settings.json"), `{"defaultProvider":"magpie","defaultModel":"relay/glm-4.6"}`)
	setUpInMagpie("pi")
	resetSyncFailures()
	t.Cleanup(resetSyncFailures)

	fail := errors.New("the agent's folder is read-only")
	syncFaultFor = func(id string) error {
		if id == "pi" {
			return fail
		}
		return nil
	}
	t.Cleanup(func() { syncFaultFor = nil })

	SyncCatalog()
	if got := syncFailures(); len(got["pi"]) != 1 || got["pi"][0] != fail {
		t.Fatalf("the round kept %v, want pi's failure once", got)
	}
	// the same failure, said by another round: one kept error, not two
	SyncCatalog()
	if got := syncFailures(); len(got["pi"]) != 1 {
		t.Fatalf("the same failure kept twice: %v", got)
	}
	// and a sync that writes again drops its failures
	syncFaultFor = nil
	SyncCatalog()
	if got := syncFailures(); len(got) != 0 {
		t.Fatalf("an agent whose sync wrote is kept in the failures: %v", got)
	}
}

// setAgain, the rewire's own path, keeps a sync's failure the same way:
// the rewire's reply says a reset's failure; a model list's is no one's
// and is kept for the next round to say once.
func TestRewireKeepsSyncFailure(t *testing.T) {
	syncHome(t)
	resetSyncFailures()
	t.Cleanup(resetSyncFailures)

	a := &Agent{ID: "pi", Name: "Pi",
		Sync: func() error { return errors.New("pi's list couldn't be written") }}

	moved, errs := setAgain([]*Agent{a})
	if len(moved) != 1 || len(errs) != 0 {
		t.Fatalf("the rewire's reply: moved %v, errs %v", moved, errs)
	}
	if got := syncFailures(); len(got["pi"]) != 1 {
		t.Fatalf("the rewire's sync failure kept: %v", got)
	}
}
