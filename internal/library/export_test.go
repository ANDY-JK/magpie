package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ExportFiles writes the library as files: one JSON for each MCP server, hook
// and subagent, under names a file can take, and a README saying Load from
// files reads them back — an edited file updates its entry, a file the
// library has no entry for adds one. A second export drops what the library
// no longer has.
func TestExportFiles(t *testing.T) {
	sandbox(t)
	if _, err := SaveServer("", Server{Name: "github", Description: "Issues and PRs", Transport: "stdio", Command: "npx", Agents: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveHook("", Hook{Name: "fmt", Description: "Formats after an edit", Event: "after-tool", Matcher: "shell", Command: "fmt", Timeout: 30, Agents: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveSubagent("", Subagent{Name: "reviewer", Description: "Reviews a diff", Prompt: "You review code.", Agents: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}

	dir, n, _, err := ExportFiles()
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("%d files written, want 3", n)
	}
	for _, rel := range []string{"mcp/github.json", "hooks/fmt.json", "subagents/reviewer.json"} {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		var v struct{ Name, Description string }
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		if v.Name == "" || v.Description == "" {
			t.Fatalf("%s holds %q", rel, b)
		}
	}
	// the README says what Load from files really does (import.go), not the
	// old claim that magpie never reads these files back, and says where
	// each of the five kinds lives — and that a file put in the folder is
	// not imported by being there. The wraps are the text's, not the
	// wording's, so the wants are checked against one word to a line.
	readme, err := os.ReadFile(filepath.Join(dir, "README.txt"))
	if err != nil {
		t.Fatalf("README: %v", err)
	}
	flat := strings.Join(strings.Fields(string(readme)), " ")
	for _, want := range []string{
		"Load from files", "updates that entry", "no file here is removed",
		"the library's skills/ folder",
		"instruction-sets/<id>.md", "is in library.json",
		"not imported by being here",
		"Dropping a .md into instruction-sets/ does not make a set",
		"A folder dropped into skills/ is not a skill either",
	} {
		if !strings.Contains(flat, want) {
			t.Fatalf("README doesn't say %q: %q", want, readme)
		}
	}
	if strings.Contains(string(readme), "never reads") {
		t.Fatalf("README still says magpie never reads the files back: %q", readme)
	}

	// the mirror drops what the library no longer has
	if _, err := RemoveServer("github"); err != nil {
		t.Fatal(err)
	}
	dir, n, _, err = ExportFiles()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("%d files after the server was removed, want 2", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "mcp", "github.json")); !os.IsNotExist(err) {
		t.Fatalf("github.json stayed: %v", err)
	}
}

// A name a file can't take is written with _ for what it can't.
func TestExportName(t *testing.T) {
	for in, want := range map[string]string{
		"a/b:c": "a_b_c",
		"a\\b":  "a_b",
		"  x  ": "x",
		"   ":   "unnamed",
		"plain": "plain",
	} {
		if got := exportName(in); got != want {
			t.Fatalf("%q -> %q, want %q", in, got, want)
		}
	}
}

// A folder the reader asked for stays current as the library changes; one
// never asked for isn't written on every save.
func TestExportStaysCurrent(t *testing.T) {
	sandbox(t)
	if _, err := SaveServer("", Server{Name: "one", Transport: "http", URL: "https://example.com/mcp", Agents: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(Dir(), "export")); !os.IsNotExist(err) {
		t.Fatalf("an export folder appeared without being asked for: %v", err)
	}
	dir, _, _, err := ExportFiles()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SaveServer("", Server{Name: "two", Transport: "http", URL: "https://example.com/two", Agents: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mcp", "two.json")); err != nil {
		t.Fatalf("the change didn't reach the mirror: %v", err)
	}
	if _, err := RemoveServer("one"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mcp", "one.json")); !os.IsNotExist(err) {
		t.Fatalf("the removed server stayed: %v", err)
	}
}

// An export says how many files it wrote over that had been changed by hand,
// and writes the library's JSON over them.
func TestExportSaysWhatItOverwrote(t *testing.T) {
	sandbox(t)
	if _, err := SaveServer("", Server{Name: "one", Transport: "http", URL: "https://example.com/mcp", Agents: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	dir, _, changed, err := ExportFiles()
	if err != nil {
		t.Fatal(err)
	}
	if changed != 0 {
		t.Fatalf("a first export wrote over %d files", changed)
	}
	p := filepath.Join(dir, "mcp", "one.json")
	if err := os.WriteFile(p, []byte("edited by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, changed, err = ExportFiles(); err != nil {
		t.Fatal(err)
	}
	if changed != 1 {
		t.Fatalf("%d files written over, want 1", changed)
	}
	b, err := os.ReadFile(p)
	if err != nil || !strings.Contains(string(b), "\"name\": \"one\"") {
		t.Fatalf("the mirror wasn't written back: %v %q", err, b)
	}
}

// The page's payload says whether the export folder is there, and where:
// the header's Export folder button opens it when it is, and is disabled,
// saying what makes one, when it isn't. The fields also name the library's
// own skills folder and the one the instruction sets besides the first are
// kept in, whose tabs had no way in from the page.
func TestReadSaysWhetherTheExportFolderIsThere(t *testing.T) {
	sandbox(t)
	if _, err := SaveServer("", Server{Name: "one", Transport: "http", URL: "https://example.com/mcp", Agents: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	v, err := Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Exported || v.Export != "" {
		t.Fatalf("a library never exported says exported=%v export=%q", v.Exported, v.Export)
	}
	// exported:false has to reach the page: left out of the JSON, the
	// button looked lit and opened nothing
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"exported":false`) {
		t.Fatalf("exported:false was left out of the page's payload: %s", b)
	}
	if v.SkillsDir != skillsDir() || v.SetsDir != setsDir() {
		t.Fatalf("skillsDir %q setsDir %q, want %q and %q", v.SkillsDir, v.SetsDir, skillsDir(), setsDir())
	}

	dir, _, _, err := ExportFiles()
	if err != nil {
		t.Fatal(err)
	}
	v, err = Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Exported {
		t.Fatal("the export folder is there, and the page isn't told")
	}
	if v.Export != dir {
		t.Fatalf("export %q, want the folder ExportFiles made (%q)", v.Export, dir)
	}
}
