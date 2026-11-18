package provider

import (
	"path/filepath"
	"testing"
)

// A routing a plugin serves ("<plugin>/<name>") is kept as saved, on a
// provider and a group alike, whether a plugin serves it now or not; what
// is neither magpie's nor a plugin's is Smart again.
func TestPluginRoutingKept(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	for _, c := range []struct{ in, want string }{
		{"spread/fewest-tokens", "spread/fewest-tokens"},
		{"magpie-fastest/byTTFT", "magpie-fastest/byTTFT"},
		{"rotate", "rotate"},
		{"bogus", ""},
		{"a b/c", ""},
		{"a/b/c", ""},
		{"/c", ""},
	} {
		if err := Save(Provider{ID: "a", Name: "A", Chat: "http://127.0.0.1:1/v1", Key: "k", Routing: c.in}); err != nil {
			t.Fatal(err)
		}
		if p, err := Find("a"); err != nil || p.Routing != c.want {
			t.Errorf("provider routed %q: kept %q, want %q (%v)", c.in, p.Routing, c.want, err)
		}
		if err := SaveGroup(Group{ID: "g", Name: "g", Members: []string{"a/m1", "a/m2"}, Routing: c.in}); err != nil {
			t.Fatal(err)
		}
		f, err := read()
		if err != nil || len(f.Groups) != 1 || f.Groups[0].Routing != c.want {
			t.Errorf("group routed %q: kept %+v, want %q (%v)", c.in, f.Groups, c.want, err)
		}
	}
}

// Score is magpie's own routing, so a provider or a group saved with it
// keeps it: without keptRouting's case a save would read it as an unknown
// routing and clear it, and the user's pick would be gone.
func TestScoreRoutingKept(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	if err := Save(Provider{ID: "a", Name: "A", Chat: "http://127.0.0.1:1/v1", Key: "k", Routing: Score}); err != nil {
		t.Fatal(err)
	}
	if p, err := Find("a"); err != nil || p.Routing != Score {
		t.Fatalf("provider routed score: kept %q, want %q (%v)", p.Routing, Score, err)
	}
	if err := SaveGroup(Group{ID: "g", Name: "g", Members: []string{"a/m1", "a/m2"}, Routing: Score}); err != nil {
		t.Fatal(err)
	}
	if g, _, ok := FindGroup(GroupPrefix + "g"); !ok || g.Routing != Score {
		t.Fatalf("group routed score: kept %+v, %v", g, ok)
	}
}
