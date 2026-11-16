package gui

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/yetone/magpie/internal/library"
)

// The Library page may ask the file manager only for paths it shows: the
// whitelist holds the library's own folder and backups, the skills folder
// and the instruction sets' (whose tabs had no way in from the page), and
// the export folder once Save as files has made one.
func TestRevealableHoldsTheFoldersThePageShows(t *testing.T) {
	sandboxHome(t)
	v, err := library.Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Export != "" || v.Exported {
		t.Fatalf("a library never exported says exported=%v export=%q", v.Exported, v.Export)
	}
	out := revealable(v)
	for _, p := range []string{v.Dir, v.Backups, v.SkillsDir, v.SetsDir} {
		if !slices.Contains(out, p) {
			t.Fatalf("%q is not shown in the file manager", p)
		}
	}
	if slices.Contains(out, "") {
		t.Fatal("the whitelist holds an empty path while there is no export folder")
	}
	if slices.Contains(out, filepath.Join(v.Dir, "export")) {
		t.Fatalf("the export folder is offered before Save as files has made one")
	}

	if _, _, _, err := library.ExportFiles(); err != nil {
		t.Fatal(err)
	}
	v, err = library.Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Export != filepath.Join(v.Dir, "export") {
		t.Fatalf("export %q, want the folder under the library's own (%q)", v.Export, v.Dir)
	}
	if !slices.Contains(revealable(v), v.Export) {
		t.Fatalf("the export folder %q is not shown in the file manager", v.Export)
	}
}
