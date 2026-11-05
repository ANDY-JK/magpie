package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/usage"
	"github.com/yetone/magpie/internal/usageapi"
)

// The Usage page's ranking tab reads /api/usage/rank, the same ranking
// the gateway's /v1/magpie/usage/rank answers, in the reader's own zone: a
// provider set up and not hidden is offered for removal, a deleted one is
// kept in the ranking under its old name and never offered again, and a
// provider with too few calls to say anything is named beside it.
func TestUsageRankRoute(t *testing.T) {
	sandboxHome(t)
	mux := http.NewServeMux()
	usageRoutes(mux, folderOnly{})
	if err := provider.Save(provider.Provider{ID: "plan", Name: "Plan", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m1"}}); err != nil {
		t.Fatal(err)
	}
	if err := provider.Save(provider.Provider{ID: "quiet", Name: "Quiet", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m2"}, Hidden: true}); err != nil {
		t.Fatal(err)
	}
	if err := provider.Save(provider.Provider{ID: "gone", Name: "Gone", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"m3"}}); err != nil {
		t.Fatal(err)
	}
	if err := provider.Delete("gone"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 0; i < usageapi.RankMinCalls; i++ {
		usage.Append(usage.Record{Time: now.Add(-time.Duration(i) * time.Minute), Agent: "codex", Provider: "plan", Model: "m1", Input: 100, Output: 50, Status: 200, TTFT: 300, Millis: 5000})
		usage.Append(usage.Record{Time: now.Add(-time.Duration(i) * time.Minute), Agent: "codex", Provider: "quiet", Model: "m2", Input: 100, Output: 50, Status: 200, TTFT: 300, Millis: 5000})
		usage.Append(usage.Record{Time: now.Add(-time.Duration(i) * time.Minute), Agent: "codex", Provider: "gone", Model: "m3", Input: 100, Output: 50, Status: 200, TTFT: 300, Millis: 5000})
	}
	usage.Append(usage.Record{Time: now, Agent: "codex", Provider: "plan", Model: "m1", Input: 100, Output: 50, Status: 200})
	get := func(path string) usageapi.Ranking {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		var r usageapi.Ranking
		if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := get("/api/usage/rank?period=all")
	if len(r.Providers) != 3 || r.Providers[0].ID != "plan" || r.Providers[0].Rank != 1 {
		t.Fatalf("ranked: %+v", r.Providers)
	}
	byID := map[string]usageapi.RankEntry{}
	for _, e := range r.Providers {
		byID[e.ID] = e
	}
	if !byID["plan"].Removable || byID["quiet"].Removable {
		t.Fatal("removable: plan is offered, quiet is not:", byID["plan"], byID["quiet"])
	}
	if byID["gone"].Removable || byID["gone"].Removed == nil || byID["gone"].Name != "Gone" {
		t.Fatalf("a deleted provider: %+v", byID["gone"])
	}
	// a model's own place under its provider, and the words the page says
	e := byID["plan"]
	if len(e.Models) != 1 || e.Models[0].Model != "m1" || e.Models[0].Score != e.Score {
		t.Fatalf("plan's models: %+v", e.Models)
	}
	for _, p := range e.Parts {
		if !isRankWord(p.Key) {
			t.Fatalf("the page has no word for %q", p.Key)
		}
	}
	// read in the reader's zone: the period's end is that zone's now
	if e.Days != 1 {
		t.Fatalf("days: %+v", e)
	}
	// a period named wrongly is read as the period a reader gets without one
	if r2 := get("/api/usage/rank?period=nonsense"); r2.Period != usage.Month {
		t.Fatalf("an unnamed period: %+v", r2.Period)
	}
}

// isRankWord is whether key is one of the ranking's own words, which the
// page translates: none of them is a vendor's type or a word of its body.
func isRankWord(key string) bool {
	switch key {
	case "served", "stood", "fast", "soon",
		"steady", "failing", "slow", "slow-start", "unpriced",
		"rate_limit", "quota", "credit", "auth", "overflow", "loop",
		"restricted", "verify", "refused", "prompt", "cut", "upstream", "failed":
		return true
	}
	return false
}
