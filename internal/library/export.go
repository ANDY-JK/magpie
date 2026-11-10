package library

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExportFiles writes the library as files under the library's export folder:
// one JSON for each MCP server, hook and subagent, and a README saying where
// they come from. They are a mirror of library.json, written to be read and
// copied — magpie never reads them back, so editing one changes nothing. It
// answers the folder and how many entries it wrote.
func ExportFiles() (string, int, error) {
	l, err := load()
	if err != nil {
		return "", 0, err
	}
	return writeExport(l)
}

// syncExport keeps a folder the reader asked for current after a change: a
// library without one isn't given one, so nothing is written until Save as
// files is used once. Errors are the reader's next export to show.
func syncExport(l *Library) {
	if _, err := os.Stat(filepath.Join(Dir(), "export")); err != nil {
		return
	}
	_, _, _ = writeExport(l)
}

// writeExport is ExportFiles for a library already read.
func writeExport(l *Library) (string, int, error) {
	root := filepath.Join(Dir(), "export")
	kinds := []struct {
		dir string
		do  func(string) error
	}{
		{"mcp", func(dir string) error { return exportEach(dir, l.MCP) }},
		{"hooks", func(dir string) error { return exportEach(dir, l.Hooks) }},
		{"subagents", func(dir string) error { return exportEach(dir, l.Subagents) }},
	}
	n := 0
	for _, k := range kinds {
		dir := filepath.Join(root, k.dir)
		// a mirror, not a collection: what the library no longer has goes
		if err := os.RemoveAll(dir); err != nil {
			return "", 0, err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", 0, err
		}
		if err := k.do(dir); err != nil {
			return "", 0, err
		}
		n += count(dir)
	}
	readme := "These files are a mirror of magpie's library.json, written for reading and copying.\n" +
		"magpie never reads them back: editing one changes nothing. Use the Library page instead.\n"
	if err := os.WriteFile(filepath.Join(root, "README.txt"), []byte(readme), 0o644); err != nil {
		return "", 0, err
	}
	return root, n, nil
}

// exportEach writes one JSON file for each named entry, under a name a file
// can take: two names that come to the same file are kept apart.
func exportEach[T any](dir string, items []*T) error {
	used := map[string]bool{}
	for _, it := range items {
		name, body := nameAndJSON(it)
		if name == "" {
			continue
		}
		base := exportName(name)
		for i := 2; used[base]; i++ {
			base = fmt.Sprintf("%s~%d", exportName(name), i)
		}
		used[base] = true
		if err := os.WriteFile(filepath.Join(dir, base+".json"), body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// nameAndJSON is an entry's name and its JSON, indented.
func nameAndJSON[T any](it *T) (string, []byte) {
	b, err := json.MarshalIndent(it, "", "  ")
	if err != nil {
		return "", nil
	}
	var head struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(b, &head)
	return head.Name, append(b, '\n')
}

// exportName is name as a file name: the characters a file can't take, and
// the spaces around them, become _. Names magpie makes are already safe; one
// imported by hand may not be.
func exportName(name string) string {
	s := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			return '_'
		}
		return r
	}, name)
	s = strings.TrimSpace(s)
	if s == "" {
		s = "unnamed"
	}
	return s
}

// count is how many files dir holds.
func count(dir string) int {
	es, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range es {
		if !e.IsDir() {
			n++
		}
	}
	return n
}
