package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ExportFiles writes the library as files: one JSON for each MCP server, hook
// and subagent, under names a file can take, and a README saying they are a
// mirror magpie never reads back. A second export drops what the library no
// longer has.
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

	dir, n, err := ExportFiles()
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
	readme, err := os.ReadFile(filepath.Join(dir, "README.txt"))
	if err != nil || !strings.Contains(string(readme), "never reads them back") {
		t.Fatalf("README: %v %q", err, readme)
	}

	// the mirror drops what the library no longer has
	if _, err := RemoveServer("github"); err != nil {
		t.Fatal(err)
	}
	dir, n, err = ExportFiles()
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
