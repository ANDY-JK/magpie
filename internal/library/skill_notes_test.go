package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A skill's Chinese description (the owner, 11-10) is saved beside
// library.json, not in its SKILL.md: the row shows it in place of the
// original, which stays, and the original file is untouched.
func TestSkillNoteSavedBesideTheLibrary(t *testing.T) {
	h := sandbox(t)
	setUpLibrary(t, h) // pdf
	// the skill's own description, as its SKILL.md has it
	before := offView(t, "pdf").Description
	if before == "" {
		t.Fatal("the fixture's skill has no description")
	}
	ok(t)(SetSkillNote("pdf", "  把 PDF 拆成  页，并读出文字  "))
	if v := offView(t, "pdf"); v.Note != "把 PDF 拆成 页，并读出文字" {
		t.Fatalf("the note reads %q", v.Note)
	}
	if v := offView(t, "pdf"); v.Description != before {
		t.Fatalf("the original description changed: %q", v.Description)
	}
	// beside the library, not inside it
	if _, err := os.Stat(filepath.Join(path())); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(h, ".config", "magpie", "skill-notes.json")); err != nil {
		t.Fatalf("the note is not in skill-notes.json: %v", err)
	}
	raw, err := os.ReadFile(path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "把 PDF") {
		t.Fatal("the note went into library.json")
	}
	// read again from the file, as another window or after a restart does
	skillNotes.Lock()
	skillNotes.m = nil
	skillNotes.Unlock()
	if v := offView(t, "pdf"); v.Note != "把 PDF 拆成 页，并读出文字" {
		t.Fatalf("a fresh read has %q", v.Note)
	}
	// a skill that isn't in the library has no note to save
	if _, err := SetSkillNote("no-such-skill", "x"); err == nil {
		t.Fatal("a note was saved for a skill that isn't there")
	}
	// empty takes it away
	ok(t)(SetSkillNote("pdf", "   "))
	if v := offView(t, "pdf"); v.Note != "" {
		t.Fatalf("the note is still %q", v.Note)
	}
	// and it is a description, not the documentation
	if _, err := SetSkillNote("pdf", strings.Repeat("中", maxSkillNote+1)); err == nil {
		t.Fatal("a note past the limit was saved")
	}
}

// A sync neither carries a note to another computer nor takes away the one
// that computer has: the notes are this computer's.
func TestSkillNoteStaysHere(t *testing.T) {
	a := sandbox(t)
	setUpLibrary(t, a)
	ok(t)(SetSkillNote("pdf", "A 的中文说明"))
	fromA := carry(t)

	b := sandbox(t)
	ok(t)(Put(&fromA))
	if v := offView(t, "pdf"); v.Note != "" {
		t.Fatalf("A's note was carried to B: %q", v.Note)
	}
	ok(t)(SetSkillNote("pdf", "B 的中文说明"))

	// A changes the library and syncs again: B's own note stays
	at(t, a)
	ok(t)(SkillAgents("pdf", []string{"claude", "codex"}))
	fromA = carry(t)
	at(t, b)
	ok(t)(Put(&fromA))
	if v := offView(t, "pdf"); v.Note != "B 的中文说明" {
		t.Fatalf("a sync took B's note away: %q", v.Note)
	}
}
