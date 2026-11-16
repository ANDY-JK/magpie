package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/lastgood"
)

// The daily check-ins are on for a new install and for a settings file
// written before they were; an explicit off is written and stays off, and
// a file nobody can read leaves them unknown, and off.
func TestCheckinOnByDefault(t *testing.T) {
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	lastgood.Reset()
	t.Cleanup(lastgood.Reset)

	on := func(s Settings) bool {
		return s.WorkBuddyCheckin && s.TraeCheckin && s.MiniMaxCheckin && s.QoderCheckin && s.CheckinSettled
	}
	if s := Load(); !on(s) {
		t.Fatalf("a new install: %+v", s)
	}
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	// a file written before the switches were on has no checkinSettled
	if err := os.WriteFile(Path(), []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Load()
	if !on(s) || s.Theme != "dark" {
		t.Fatalf("an older file: %+v", s)
	}
	// a choice to turn one off is written, and stays off
	s.WorkBuddyCheckin = false
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"checkinSettled": true`) {
		t.Fatalf("the settled mark wasn't written: %s", b)
	}
	s = Load()
	if s.WorkBuddyCheckin || !s.TraeCheckin || !s.MiniMaxCheckin || !s.QoderCheckin || !s.CheckinSettled {
		t.Fatalf("an explicit off: %+v", s)
	}
	// reset puts back a new install's defaults
	if err := Save(Defaults()); err != nil {
		t.Fatal(err)
	}
	if s := Load(); !on(s) {
		t.Fatalf("reset: %+v", s)
	}
	// a file nobody can read is unknown: nothing is checked in
	lastgood.Reset()
	if err := os.Remove(lastgood.Bak(Path())); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(), []byte(`{"theme":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := Load(); s.WorkBuddyCheckin || s.TraeCheckin || s.MiniMaxCheckin || s.QoderCheckin {
		t.Fatalf("a garbled file turned check-ins on: %+v", s)
	}
}
