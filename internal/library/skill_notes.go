package library

// A skill's description is its SKILL.md's, in the language its author wrote
// it. The owner (11-10) asked for a Chinese description kept in magpie's
// library, the SKILL.md untouched: one is saved beside library.json by the
// skill's name, and shown under its name in place of the original, which
// stays in the hover. Notes are this computer's: a sync neither carries them
// nor takes them away (Collect and Put leave the file alone).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/steady"
)

// maxSkillNote is how long a note may be: a description under a name, not
// the skill's documentation, which is its SKILL.md's.
const maxSkillNote = 500

var skillNotes struct {
	sync.Mutex
	path string            // the file m was read from: another home reads again
	m    map[string]string // nil until the file is read
}

// skillNotesPath is where the notes are kept: beside library.json, not in
// it, so a sync of the library and every command that writes it leave them
// as they are.
func skillNotesPath() string { return filepath.Join(settings.Dir(), "skill-notes.json") }

// readSkillNotes is the file's notes, an absent file having none.
func readSkillNotes() (map[string]string, error) {
	b, err := os.ReadFile(skillNotesPath())
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", skillNotesPath(), err)
		}
	}
	return m, nil
}

// noteOf is the Chinese description saved for a skill, "" when it has none.
func noteOf(name string) string {
	skillNotes.Lock()
	defer skillNotes.Unlock()
	if p := skillNotesPath(); skillNotes.m == nil || skillNotes.path != p {
		m, err := readSkillNotes()
		if err != nil {
			return ""
		}
		skillNotes.m, skillNotes.path = m, p
	}
	return skillNotes.m[name]
}

// SetSkillNote saves note as the Chinese description shown for a skill, or
// takes it away when note is empty. What a user types keeps its words: only
// runs of white space become one space, as a description read from a
// SKILL.md is normalized.
func SetSkillNote(name, note string) (*Result, error) {
	note = strings.Join(strings.Fields(note), " ")
	if n := utf8.RuneCountInString(note); n > maxSkillNote {
		return nil, fmt.Errorf("a description of at most %d characters, this one has %d", maxSkillNote, n)
	}
	mu.Lock()
	l, err := load()
	found := err == nil && l.skill(name) != nil
	mu.Unlock()
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("no skill called %s", name)
	}
	skillNotes.Lock()
	defer skillNotes.Unlock()
	// read again rather than write what this process holds: another window,
	// or a command, may have saved a note since
	m, err := readSkillNotes()
	if err != nil {
		return nil, err
	}
	if note == "" {
		delete(m, name)
	} else {
		m[name] = note
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := steady.WriteFile(skillNotesPath(), append(b, '\n'), 0o600); err != nil {
		return nil, err
	}
	skillNotes.m, skillNotes.path = m, skillNotesPath()
	return &Result{}, nil
}
