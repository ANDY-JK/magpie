package gui

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/library"
)

// The hooks and subagents pages turn every entry on or off for the agents
// named through agents-all, as the servers' and skills' pages do: one change
// gives every hook or subagent in the library to those agents, or takes it
// from them, and an agent the pages don't show keeps what it has.
func TestLibraryAgentsAllRoutes(t *testing.T) {
	sandboxHome(t)
	if _, err := library.SaveHook("", library.Hook{Name: "bark", Event: "turn-end", Command: "bark", Agents: []string{"codex"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := library.SaveSubagent("", library.Subagent{Name: "reviewer", Description: "Reviews a diff", Prompt: "You review code.", Agents: []string{"codex"}}); err != nil {
		t.Fatal(err)
	}
	h := Handler(nil, nil)
	post := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", path, strings.NewReader(body)))
		return rec
	}
	agentsOf := func(kind, name string) []string {
		t.Helper()
		v, err := library.Read(nil)
		if err != nil {
			t.Fatal(err)
		}
		switch kind {
		case "hooks":
			for _, hk := range v.Hooks {
				if hk.Name == name {
					return hk.Agents
				}
			}
		case "subagents":
			for _, s := range v.Subagents {
				if s.Name == name {
					return s.Agents
				}
			}
		}
		t.Fatalf("no %s called %s", kind, name)
		return nil
	}

	if rec := post("/api/library/hooks/agents-all", `{"agents":["claude","codex"],"on":true}`); rec.Code != http.StatusOK {
		t.Fatalf("hooks/agents-all: %d %s", rec.Code, rec.Body)
	}
	if got := agentsOf("hooks", "bark"); !slices.Equal(got, []string{"claude", "codex"}) {
		t.Fatalf("bark on %v", got)
	}
	if rec := post("/api/library/subagents/agents-all", `{"agents":["claude","codex"],"on":true}`); rec.Code != http.StatusOK {
		t.Fatalf("subagents/agents-all: %d %s", rec.Code, rec.Body)
	}
	if got := agentsOf("subagents", "reviewer"); !slices.Equal(got, []string{"claude", "codex"}) {
		t.Fatalf("reviewer on %v", got)
	}

	// off leaves an agent not named as it is
	if _, err := library.SaveHook("", library.Hook{Name: "fmt", Event: "after-tool", Matcher: "edit", Command: "fmt", Agents: []string{"gemini", "codex"}}); err != nil {
		t.Fatal(err)
	}
	if rec := post("/api/library/hooks/agents-all", `{"agents":["codex"],"on":false}`); rec.Code != http.StatusOK {
		t.Fatalf("hooks/agents-all off: %d %s", rec.Code, rec.Body)
	}
	if got := agentsOf("hooks", "fmt"); !slices.Equal(got, []string{"gemini"}) {
		t.Fatalf("fmt on %v after codex was taken off", got)
	}
	// no agents is refused, as the Library's own calls never send one
	if rec := post("/api/library/hooks/agents-all", `{"agents":[],"on":true}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("hooks/agents-all with no agents: %d %s", rec.Code, rec.Body)
	}
	if rec := post("/api/library/subagents/agents-all", `{"agents":[]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("subagents/agents-all with no agents: %d %s", rec.Code, rec.Body)
	}
}
