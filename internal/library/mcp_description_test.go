package library

import (
	"path/filepath"
	"strings"
	"testing"
)

// The owner (11-10): an MCP server says in its own words what it is for, as
// a hook does (#2011). The library keeps it through a save, a rename and a
// change of agents, the page reads it, and no agent's file is given it.
func TestServerDescription(t *testing.T) {
	h := sandbox(t)
	ok(t)(SaveServer("", Server{Name: "files", Description: "  Reads and\nwrites files in a folder ", Transport: "stdio", Command: "fs-mcp", Agents: []string{"claude"}}))
	if d := mustLoad(t).server("files").Description; d != "Reads and writes files in a folder" {
		t.Errorf("kept %q, want the words on one line", d)
	}
	if s := serverIn(t, "files"); s == nil || s.Description != "Reads and writes files in a folder" {
		t.Fatalf("the page reads %+v, want its description", s)
	}
	// a rename and another agent keep the words
	ok(t)(ServerAgents("files", []string{"claude", "codex"}))
	ok(t)(SaveServer("files", Server{Name: "fs", Description: "Reads and writes files in a folder", Transport: "stdio", Command: "fs-mcp", Agents: []string{"claude", "codex"}}))
	if s := serverIn(t, "fs"); s == nil || s.Description != "Reads and writes files in a folder" {
		t.Fatalf("after a rename the page reads %+v, want its description", s)
	}
	// only the library keeps it: no agent's file is given the words
	if c := read(t, filepath.Join(h, ".claude.json")); strings.Contains(c, "Reads and writes files") {
		t.Errorf("Claude Code's file was given the description:\n%s", c)
	}
	// a description past 200 characters is refused, the server left as it was
	long := strings.Repeat("x", maxServerDescription+1)
	if _, err := SaveServer("fs", Server{Name: "fs", Description: long, Transport: "stdio", Command: "fs-mcp"}); err == nil || !strings.Contains(err.Error(), "200") {
		t.Fatalf("a description of %d characters: %v", len(long), err)
	}
	if s := serverIn(t, "fs"); s == nil || s.Description != "Reads and writes files in a folder" {
		t.Fatalf("a refused save left %+v", s)
	}
	// a server added from the market arrives saying what the market says
	ok(t)(InstallServer("context7", nil, []string{"claude"}))
	if s := serverIn(t, "context7"); s == nil || s.Description != "Up-to-date documentation and code examples for any library, straight into the prompt." {
		t.Fatalf("the market's server: %+v, want the market's description", s)
	}
}
