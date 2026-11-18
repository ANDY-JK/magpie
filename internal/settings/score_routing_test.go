package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/lastgood"
)

// Score routing is on for a new install, for a settings file written before
// the routing existed and for one nobody can read: a file that never chose
// has no value. An explicit off is written and stays off.
func TestScoreRoutingOnByDefault(t *testing.T) {
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	lastgood.Reset()
	t.Cleanup(lastgood.Reset)

	if s := Load(); !s.ScoreRoutingOn() || s.ScoreRouting != nil {
		t.Fatalf("a new install: %v %+v", s.ScoreRouting, s)
	}
	if !Defaults().ScoreRoutingOn() {
		t.Fatal("the defaults put score routing off")
	}
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	// a file written before the routing has no scoreRouting
	if err := os.WriteFile(Path(), []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := Load(); !s.ScoreRoutingOn() || s.Theme != "dark" {
		t.Fatalf("an older file: %v %+v", s.ScoreRouting, s)
	}
	// a choice to turn it off is written, and stays off
	s := Load()
	off := false
	s.ScoreRouting = &off
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"scoreRouting": false`) {
		t.Fatalf("the choice wasn't written: %s", b)
	}
	if s := Load(); s.ScoreRoutingOn() {
		t.Fatalf("an explicit off: %+v", s)
	}
	// reset puts back a new install's default
	if err := Save(Defaults()); err != nil {
		t.Fatal(err)
	}
	if s := Load(); !s.ScoreRoutingOn() {
		t.Fatalf("reset: %+v", s)
	}
	// a file nobody can read is no choice either: the default is served
	lastgood.Reset()
	if err := os.Remove(lastgood.Bak(Path())); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(), []byte(`{"theme":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := Load(); !s.ScoreRoutingOn() {
		t.Fatalf("a garbled file put score routing off: %+v", s)
	}
}
