package provider

import (
	"context"
	"slices"
	"testing"
	"time"
)

// A fetch that changes the vendor's list keeps what changed with the
// provider: a model listed since in Added, one it stopped listing in
// Removed, and when. The first fetch has no earlier list to compare, so it
// records nothing; the record is written to providers.json, so a fresh read
// (as after a restart) has it; and a Save that carries none, as the editor's
// does, keeps it.
func TestListChangeRecordedWhenTheListChanges(t *testing.T) {
	autolistHome(t)
	r := newRelayList(t, "model-a", "model-b")
	if err := Save(Provider{ID: "relay", Name: "Relay", Chat: r.srv.URL + "/v1", Key: "sk-relay"}); err != nil {
		t.Fatal(err)
	}
	refetchRelay(t, "relay")
	if got := ListChanges("relay"); len(got) != 0 {
		t.Fatalf("a first fetch recorded %v; it has no earlier list to compare", got)
	}

	r.set("model-a", "model-c")
	refetchRelay(t, "relay")
	got := ListChanges("relay")
	if len(got) != 1 {
		t.Fatalf("list changes: %v, want one", got)
	}
	if !slices.Equal(got[0].Added, []string{"model-c"}) {
		t.Errorf("added: %v, want [model-c]", got[0].Added)
	}
	if !slices.Equal(got[0].Removed, []string{"model-b"}) {
		t.Errorf("removed: %v, want [model-b]", got[0].Removed)
	}
	if got[0].At.IsZero() || time.Since(got[0].At) > time.Minute {
		t.Errorf("when: %v, want the fetch's own time", got[0].At)
	}

	// read back from providers.json, as a restart does
	p, err := Find("relay")
	if err != nil || p.ListChange == nil {
		t.Fatalf("saved provider: %+v, %v", p, err)
	}
	if !slices.Equal(p.ListChange.Added, []string{"model-c"}) || !slices.Equal(p.ListChange.Removed, []string{"model-b"}) {
		t.Fatalf("kept change: %+v", *p.ListChange)
	}

	// the editor's Save sends no ListChange; it keeps the one recorded
	p.ListChange = nil
	p.Name = "My Relay"
	if err := Save(*p); err != nil {
		t.Fatal(err)
	}
	if got := ListChanges("relay"); len(got) != 1 || !slices.Equal(got[0].Removed, []string{"model-b"}) {
		t.Fatalf("a save dropped the list change: %v", got)
	}
}

// A fetch whose list is the one before writes nothing and keeps no change:
// "no change" is not a new change.
func TestListChangeNotRecordedWhenNothingChanges(t *testing.T) {
	autolistHome(t)
	r := newRelayList(t, "model-a", "model-b")
	if err := Save(Provider{ID: "relay", Name: "Relay", Chat: r.srv.URL + "/v1", Key: "sk-relay"}); err != nil {
		t.Fatal(err)
	}
	refetchRelay(t, "relay")
	before, err := readPlain(Path())
	if err != nil {
		t.Fatal(err)
	}
	refetchRelay(t, "relay")
	if got := ListChanges("relay"); len(got) != 0 {
		t.Fatalf("a fetch with the same list recorded %v", got)
	}
	after, err := readPlain(Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("providers.json written by a fetch with no change:\n%s\n%s", before, after)
	}
}

// The gateway's own scheduled fetch (KeepListsFresh) keeps the change too,
// so a model gone from the vendor's list is said on the provider's row and
// in its editor, not only in the log.
func TestScheduledRefreshKeepsTheListChange(t *testing.T) {
	autolistHome(t)
	r := newRelayList(t, "model-a", "model-b")
	if err := Save(Provider{ID: "relay", Name: "Relay", Chat: r.srv.URL + "/v1", Key: "sk-relay"}); err != nil {
		t.Fatal(err)
	}
	if err := SetRefreshEvery("relay", "6h"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		listsTried.Lock()
		clear(listsTried.m)
		listsTried.Unlock()
	})
	now := time.Now()
	if got := refreshDueLists(context.Background(), now); !slices.Equal(got, []string{"relay"}) {
		t.Fatalf("asked %v, want [relay]", got)
	}
	if got := ListChanges("relay"); len(got) != 0 {
		t.Fatalf("the first scheduled fetch recorded %v", got)
	}

	r.set("model-a", "model-c")
	if got := refreshDueLists(context.Background(), now.Add(6*time.Hour+time.Minute)); !slices.Equal(got, []string{"relay"}) {
		t.Fatalf("asked %v after its 6 hours, want [relay]", got)
	}
	got := ListChanges("relay")
	if len(got) != 1 || !slices.Equal(got[0].Added, []string{"model-c"}) || !slices.Equal(got[0].Removed, []string{"model-b"}) {
		t.Fatalf("the scheduled fetch kept %v, want model-c added and model-b removed", got)
	}
}
