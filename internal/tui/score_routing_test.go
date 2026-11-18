package tui

import (
	"slices"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// A group routed by score is offered and named as it is, and the next key
// leaves it: an unknown routing read as "smart", and cycling from one
// started again at In order, so a saved score turned into order on one key
// press.
func TestScoreRoutingInTheGroupPage(t *testing.T) {
	if got := routingName(provider.Score); got != "by score" {
		t.Fatalf("routingName(%q) = %q, want by score", provider.Score, got)
	}
	if !slices.Contains(routings, provider.Score) {
		t.Fatalf("routings doesn't offer score: %v", routings)
	}
	g := provider.Group{Routing: provider.Score}
	if err := nextRouting(&g); err != nil {
		t.Fatal(err)
	}
	if g.Routing != provider.Manual {
		t.Fatalf("next from score = %q, want manual", g.Routing)
	}
}
