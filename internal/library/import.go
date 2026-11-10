package library

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ImportReport says what reading the export folder back into the library
// would change: how many entries it would add, update, or leave as they are,
// the files it can't read, and whether it was applied.
type ImportReport struct {
	Dir      string   `json:"dir"`
	Adds     int      `json:"adds"`
	Updates  int      `json:"updates"`
	Same     int      `json:"same"`
	Problems []string `json:"problems,omitempty"`
	Applied  bool     `json:"applied,omitempty"`
}

// ImportFiles reads the export folder back into the library. Without apply it
// only says what would change; with it, entries whose file differs are written
// into library.json (agents included, as the file says) and new ones added.
// A file it can't read, or one whose entry fails the same check the page's
// editor makes, is a problem: nothing is applied while there is one. Nothing
// in the library that has no file is removed.
func ImportFiles(apply bool) (ImportReport, error) {
	l, err := load()
	if err != nil {
		return ImportReport{}, err
	}
	rep := ImportReport{Dir: filepath.Join(Dir(), "export")}
	var servers []*Server
	var hooks []*Hook
	var subs []*Subagent
	read := func(kind string, each func(name string, b []byte) error) {
		dir := filepath.Join(rep.Dir, kind)
		es, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return
			}
			rep.Problems = append(rep.Problems, fmt.Sprintf("%s: %v", kind, err))
			return
		}
		for _, e := range es {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				rep.Problems = append(rep.Problems, fmt.Sprintf("%s/%s: %v", kind, e.Name(), err))
				continue
			}
			if err := each(e.Name(), b); err != nil {
				rep.Problems = append(rep.Problems, fmt.Sprintf("%s/%s: %v", kind, e.Name(), err))
			}
		}
	}
	read("mcp", func(name string, b []byte) error {
		var s Server
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if err := s.check(); err != nil {
			return err
		}
		old := findIn(l.MCP, s.Name, func(x *Server) string { return x.Name })
		if old < 0 {
			rep.Adds++
			servers = append(servers, &s)
		} else if sameJSON(l.MCP[old], &s) {
			rep.Same++
		} else {
			rep.Updates++
			servers = append(servers, &s)
		}
		return nil
	})
	read("hooks", func(name string, b []byte) error {
		var h Hook
		if err := json.Unmarshal(b, &h); err != nil {
			return err
		}
		if err := h.check(); err != nil {
			return err
		}
		old := findIn(l.Hooks, h.Name, func(x *Hook) string { return x.Name })
		if old < 0 {
			rep.Adds++
			hooks = append(hooks, &h)
		} else if sameJSON(l.Hooks[old], &h) {
			rep.Same++
		} else {
			rep.Updates++
			hooks = append(hooks, &h)
		}
		return nil
	})
	read("subagents", func(name string, b []byte) error {
		var s Subagent
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if err := s.check(); err != nil {
			return err
		}
		old := findIn(l.Subagents, s.Name, func(x *Subagent) string { return x.Name })
		if old < 0 {
			rep.Adds++
			subs = append(subs, &s)
		} else if sameJSON(l.Subagents[old], &s) {
			rep.Same++
		} else {
			rep.Updates++
			subs = append(subs, &s)
		}
		return nil
	})
	if !apply || len(rep.Problems) > 0 {
		return rep, nil
	}
	if rep.Adds+rep.Updates == 0 {
		rep.Applied = true
		return rep, nil
	}
	_, err = change(func(l *Library) error {
		for _, s := range servers {
			if i := findIn(l.MCP, s.Name, func(x *Server) string { return x.Name }); i >= 0 {
				l.MCP[i] = s
			} else {
				l.MCP = append(l.MCP, s)
			}
		}
		for _, h := range hooks {
			if i := findIn(l.Hooks, h.Name, func(x *Hook) string { return x.Name }); i >= 0 {
				l.Hooks[i] = h
			} else {
				l.Hooks = append(l.Hooks, h)
			}
		}
		for _, s := range subs {
			if i := findIn(l.Subagents, s.Name, func(x *Subagent) string { return x.Name }); i >= 0 {
				l.Subagents[i] = s
			} else {
				l.Subagents = append(l.Subagents, s)
			}
		}
		return nil
	})
	if err != nil {
		return rep, err
	}
	rep.Applied = true
	return rep, nil
}

// findIn is the place of the entry named name in list, -1 for none.
func findIn[T any](list []*T, name string, of func(*T) string) int {
	for i, x := range list {
		if of(x) == name {
			return i
		}
	}
	return -1
}

// sameJSON is whether two entries say the same thing in their files.
func sameJSON(a, b any) bool {
	x, err := json.Marshal(a)
	if err != nil {
		return false
	}
	y, err := json.Marshal(b)
	return err == nil && bytes.Equal(x, y)
}
