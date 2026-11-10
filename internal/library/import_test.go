package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ImportFiles reads the export folder back: a dry run says what would change
// and changes nothing, apply writes the edits in, and a file it can't read
// stops the whole import.
func TestImportFiles(t *testing.T) {
	sandbox(t)
	if _, err := SaveServer("", Server{Name: "one", Description: "old words", Transport: "http", URL: "https://example.com/mcp", Agents: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	dir, _, _, err := ExportFiles()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "mcp", "one.json")
	edit := func(desc string) {
		t.Helper()
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var s Server
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatal(err)
		}
		s.Description = desc
		b, _ = json.MarshalIndent(&s, "", "  ")
		if err := os.WriteFile(p, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	descNow := func() string {
		t.Helper()
		l, err := load()
		if err != nil {
			t.Fatal(err)
		}
		return l.MCP[0].Description
	}

	if rep, err := ImportFiles(false); err != nil || rep.Updates != 0 || rep.Same != 1 || rep.Adds != 0 {
		t.Fatalf("an export just written reads %+v, %v", rep, err)
	}
	edit("new words")
	rep, err := ImportFiles(false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Updates != 1 || rep.Applied || len(rep.Problems) != 0 {
		t.Fatalf("dry run %+v", rep)
	}
	if got := descNow(); got != "old words" {
		t.Fatalf("a dry run changed the library to %q", got)
	}
	if rep, err = ImportFiles(true); err != nil || !rep.Applied {
		t.Fatalf("apply %+v, %v", rep, err)
	}
	if got := descNow(); got != "new words" {
		t.Fatalf("the edit wasn't applied: %q", got)
	}

	// a file that can't be read stops everything, applied or not
	if err := os.WriteFile(filepath.Join(dir, "hooks", "bad.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	edit("newer words")
	rep, err = ImportFiles(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Problems) != 1 || rep.Applied {
		t.Fatalf("a broken file: %+v", rep)
	}
	if got := descNow(); got != "new words" {
		t.Fatalf("an import with a broken file wrote %q", got)
	}
}
