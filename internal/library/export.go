package library

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExportFiles writes the library as files under the library's export folder:
// one JSON for each MCP server, hook and subagent, and a README saying how
// Load from files reads them back. They are a mirror of library.json, written
// to be read and copied — and read back: a file edited by hand changes its
// entry at the next Load from files. It answers the folder, how many entries
// it wrote, and how many files it wrote over that had been changed by hand
// since the last export.
func ExportFiles() (string, int, int, error) {
	l, err := load()
	if err != nil {
		return "", 0, 0, err
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
	_, _, _, _ = writeExport(l)
}

// writeExport is ExportFiles for a library already read.
func writeExport(l *Library) (string, int, int, error) {
	root := filepath.Join(Dir(), "export")
	kinds := []struct {
		dir string
		do  func(string) (int, int, error)
	}{
		{"mcp", func(dir string) (int, int, error) { return exportEach(dir, l.MCP) }},
		{"hooks", func(dir string) (int, int, error) { return exportEach(dir, l.Hooks) }},
		{"subagents", func(dir string) (int, int, error) { return exportEach(dir, l.Subagents) }},
	}
	n, changed := 0, 0
	for _, k := range kinds {
		dir := filepath.Join(root, k.dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", 0, 0, err
		}
		w, c, err := k.do(dir)
		if err != nil {
			return "", 0, 0, err
		}
		n, changed = n+w, changed+c
	}
	// what the README says follows what ImportFiles does, sentence by
	// sentence: an edited file updates its entry, a file the library has
	// no entry for adds one, and an entry with no file isn't removed
	readme := "These files are a mirror of magpie's library.json, written for reading and copying —\n" +
		"and for reading back: Load from files on the Library page takes them into the library.\n" +
		"A file whose entry differs from the library's updates that entry, the agents it names\n" +
		"included; a file named for an entry the library doesn't have adds one. Nothing in the\n" +
		"library that has no file here is removed. A file that can't be read, or whose entry\n" +
		"fails magpie's check, stops the whole load: the page says what would change first, and asks.\n"
	if err := os.WriteFile(filepath.Join(root, "README.txt"), []byte(readme), 0o644); err != nil {
		return "", 0, 0, err
	}
	return root, n, changed, nil
}

// exportEach writes one JSON file for each named entry, under a name a file
// can take: two names that come to the same file are kept apart. It answers
// how many it wrote and how many it wrote over that had been changed by hand;
// a file the library no longer has is removed.
func exportEach[T any](dir string, items []*T) (int, int, error) {
	used := map[string]bool{}
	n, changed := 0, 0
	for _, it := range items {
		name, body := nameAndJSON(it)
		if name == "" {
			continue
		}
		base := exportName(name)
		for i := 2; used[base]; i++ {
			base = fmt.Sprintf("%s~%d", exportName(name), i)
		}
		used[base+".json"] = true
		p := filepath.Join(dir, base+".json")
		if old, err := os.ReadFile(p); err == nil && !bytes.Equal(old, body) {
			changed++
		}
		if err := os.WriteFile(p, body, 0o644); err != nil {
			return n, changed, err
		}
		n++
	}
	es, err := os.ReadDir(dir)
	if err != nil {
		return n, changed, err
	}
	for _, e := range es {
		if !e.IsDir() && !used[e.Name()] {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
				return n, changed, err
			}
		}
	}
	return n, changed, nil
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
