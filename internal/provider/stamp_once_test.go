package provider

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestStampLooksOnceARequest: a request holding the catalog looks at
// StampNow's files once, not once for each provider it lists, and the
// next request sees a file another process wrote between them.
func TestStampLooksOnceARequest(t *testing.T) {
	isolate(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	defer func(d time.Duration) { heldFor = d }(heldFor)
	heldFor = time.Hour // a loaded machine's pause isn't the bound
	if err := Save(Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m1"}}); err != nil {
		t.Fatal(err)
	}
	release := Hold()
	looks := stampLooks.Load()
	first := StampNow()
	for range 50 {
		if StampNow() != first {
			t.Fatal("the stamp moved within a request")
		}
	}
	if n := stampLooks.Load() - looks; n != 1 {
		t.Fatalf("51 stamps in one request looked %d times, want 1", n)
	}
	release()

	// another process rewrites the providers' file: a size of its own
	b, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(), append(b, '\n', '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	defer Hold()()
	if StampNow() == first {
		t.Fatal("the next request's stamp missed another process's write")
	}
}

// TestStampReadsTheEnvironmentOnceARequest: a request holding the catalog
// reads the environment once, at the Hold that started it (held.env), not
// once for each stamp keptFit and heldFrom ask for; outside a hold a
// stamp reads it afresh, and an environment changed between requests
// moves the next request's stamp.
func TestStampReadsTheEnvironmentOnceARequest(t *testing.T) {
	isolate(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	release := Hold()
	before := envSums.Load()
	first := StampNow()
	for range 50 {
		if StampNow() != first {
			t.Fatal("the stamp moved within a request")
		}
	}
	if n := envSums.Load() - before; n != 0 {
		t.Fatalf("51 stamps in one request read the environment %d times, want 0", n)
	}
	release()

	// a stamp outside a hold reads the environment afresh
	before = envSums.Load()
	StampNow()
	if n := envSums.Load() - before; n != 1 {
		t.Fatalf("a stamp outside a hold read the environment %d times, want 1", n)
	}

	// an environment changed between requests moves the next request's
	// stamp, though no file or folder moved
	t.Setenv("MAGPIE_STAMP_ENV_PROBE", "1")
	defer Hold()()
	if StampNow() == first {
		t.Fatal("the next request's stamp missed the environment's change")
	}
}

// BenchmarkStampNowHeld is a request holding the catalog taking a stamp
// after its first: keptFit and heldFrom ask for one several times a
// request, and each hashed every environment variable again.
func BenchmarkStampNowHeld(b *testing.B) {
	isolate(b)
	home := b.TempDir()
	b.Setenv("HOME", home)
	b.Setenv("USERPROFILE", home)
	b.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	b.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	defer Hold()()
	StampNow() // the request's first stamp looks at the files; the rest read what it kept
	b.ResetTimer()
	for range b.N {
		StampNow()
	}
}

// BenchmarkStampNowUnheld is a stamp taken outside a hold, as the
// gateway's and the agents' own stamped caches do there: the environment
// is read afresh, before and after.
func BenchmarkStampNowUnheld(b *testing.B) {
	isolate(b)
	home := b.TempDir()
	b.Setenv("HOME", home)
	b.Setenv("USERPROFILE", home)
	b.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	b.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	b.ResetTimer()
	for range b.N {
		StampNow()
	}
}
