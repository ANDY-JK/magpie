package provider

import (
	"sync/atomic"
	"testing"
	"time"
)

// Two changes to providers.json made at one moment both stay. Each mutator
// reads the file, changes it and writes it back under fileMu, so the second
// reads the first's write instead of the version it read before the first
// wrote. The hook holds the readers where the old code, with no lock, let a
// second in with the first's stale file: without fileMu one capability run's
// letters are written over by the other's. Run with -count 3 — a lock that
// held only sometimes would show.
func TestProvidersFileMutationsKeepEachOther(t *testing.T) {
	isolate(t)
	capsHome(t)
	relaySaved(t)

	var arrived atomic.Int32
	release := make(chan struct{})
	mutateTestHook = func() {
		if arrived.Add(1) == 2 {
			close(release)
			return
		}
		select {
		case <-release:
		case <-time.After(2 * time.Second):
		}
	}
	t.Cleanup(func() { mutateTestHook = nil })

	done := make(chan error, 2)
	go func() { done <- SetCaps("relay", map[string]ModelCaps{"m1": {Tools: "yes"}}) }()
	go func() { done <- SetCaps("relay", map[string]ModelCaps{"m2": {Vision: "no"}}) }()
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}

	got := capsOnDisk(t, "relay")
	if got["m1"] != (ModelCaps{Tools: "yes"}) {
		t.Errorf("m1 = %v, want tools yes: a capability run made at the same moment lost its letters", got["m1"])
	}
	if got["m2"] != (ModelCaps{Vision: "no"}) {
		t.Errorf("m2 = %v, want vision no: a capability run made at the same moment lost its letters", got["m2"])
	}
}
