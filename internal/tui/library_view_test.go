package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/library"
)

// The library page keeps the row picked in view: its window is counted in
// the lines it draws — a row a line, a section's head and the blank before
// it two more — not in the rows alone, so a list of several sections (the
// servers, the skills, those found in the agents) can't scroll the row
// picked off the bottom of a short terminal.
func TestLibraryViewKeepsTheRowPicked(t *testing.T) {
	var lib []libRow
	lib = append(lib, libRow{kind: "instructions", name: "shared instructions"})
	for i := 0; i < 10; i++ {
		lib = append(lib, libRow{kind: "mcp", name: fmt.Sprintf("srv-%02d", i)})
	}
	for i := 0; i < 20; i++ {
		lib = append(lib, libRow{kind: "skill", name: fmt.Sprintf("skill-%02d", i)})
	}
	for i := 0; i < 3; i++ {
		lib = append(lib, libRow{kind: "found-mcp", name: fmt.Sprintf("own-mcp-%d", i)})
	}
	for i := 0; i < 5; i++ {
		lib = append(lib, libRow{kind: "found-skill", name: fmt.Sprintf("own-skill-%d", i)})
	}
	lib = append(lib, libRow{kind: "rtk", name: "RTK"})
	// a terminal some thirty lines tall: the skills alone are more than the
	// page shows at once
	m := model{w: 120, h: 30, lib: lib, libView: &library.View{Dir: "/x"}}
	for _, pick := range []string{"skill-10", "own-mcp-0", "own-skill-4", "RTK"} {
		m.lrow = libIndexOf(lib, pick)
		v := m.viewLibrary()
		if !strings.Contains(v, "▸ "+pick) {
			t.Fatalf("row %d (%s) picked, but the view stops short of it:\n%s", m.lrow, pick, v)
		}
	}
	// the page starts at its top while it all fits
	m.lrow = 0
	if v := m.viewLibrary(); !strings.Contains(v, "shared instructions") {
		t.Fatalf("the top of the list isn't shown:\n%s", v)
	}
}

func libIndexOf(lib []libRow, name string) int {
	for i, r := range lib {
		if r.name == name {
			return i
		}
	}
	return -1
}

// The library page reads a skill's note (the description saved here, often
// Chinese) in place of the SKILL.md's description, as the Library window
// does, and shows a server's description after its command or address.
func TestLibraryRowsShowTheDescriptions(t *testing.T) {
	home(t)
	src := t.TempDir()
	write(t, filepath.Join(src, "pdf", "SKILL.md"), "---\nname: pdf\ndescription: Handle PDFs\n---\n")
	write(t, filepath.Join(src, "xlsx", "SKILL.md"), "---\nname: xlsx\ndescription: Handle sheets\n---\n")
	if _, err := library.InstallSkills(src, []string{"pdf", "xlsx"}, []string{"claude"}); err != nil {
		t.Fatal(err)
	}
	if _, err := library.SetSkillNote("pdf", "处理 PDF 的技能"); err != nil {
		t.Fatal(err)
	}
	if _, err := library.SaveServer("", library.Server{Name: "github", Description: "Issues and PRs",
		Transport: "stdio", Command: "npx", Args: []string{"-y", "gh-mcp"}, Agents: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := library.SaveServer("", library.Server{Name: "plain", Transport: "http",
		URL: "https://example.com/mcp", Agents: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}

	var m model
	m.reloadLibrary()
	about := func(kind, name string) string {
		t.Helper()
		for _, r := range m.lib {
			if r.kind == kind && r.name == name {
				return r.about
			}
		}
		t.Fatalf("no %s row for %s", kind, name)
		return ""
	}
	if got := about("skill", "pdf"); got != "处理 PDF 的技能" {
		t.Fatalf("pdf shows %q, not the note", got)
	}
	if got := about("skill", "xlsx"); got != "Handle sheets" {
		t.Fatalf("xlsx without a note shows %q", got)
	}
	if got := about("mcp", "github"); got != "npx -y gh-mcp · Issues and PRs" {
		t.Fatalf("github's row: %q", got)
	}
	if got := about("mcp", "plain"); got != "https://example.com/mcp" {
		t.Fatalf("plain's row: %q", got)
	}
}
