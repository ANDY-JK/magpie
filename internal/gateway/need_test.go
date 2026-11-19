package gateway

import (
	"testing"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/routes"
)

// provOf is who each candidate is by, for reading an order back.
func provOf(cs []candidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.p.ID
	}
	return out
}

// needTestBody is a Chat request's shape the need is read from, changed by
// what each case asks of the model.
const needTestPlain = `{"model":"m","messages":[{"role":"user","content":"hi"}]}`

// The need a request asks of a model is read off its body: tools when it
// carries any, vision when a turn holds a picture — a tool's included,
// which a parse read for facts alone keeps nothing of — and stream when it
// asked for one. A request that asks nothing says so.
func TestNeedOfBody(t *testing.T) {
	cases := []struct {
		name string
		body string
		want need
	}{
		{"plain", needTestPlain, need{}},
		{"tools", `{"model":"m","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"f"}}]}`, need{tools: true}},
		{"stream", `{"model":"m","messages":[{"role":"user","content":"hi"}],"stream":true}`, need{stream: true}},
		{"vision", `{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}]}`, need{vision: true}},
		{"vision a tool gave back", `{"model":"m","messages":[{"role":"user","content":"hi"},{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"f","arguments":"{}"}}]},{"role":"tool","tool_call_id":"c1","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}]}`, need{vision: true}},
		{"all", `{"model":"m","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}],"tools":[{"type":"function","function":{"name":"f"}}],"stream":true}`, need{tools: true, vision: true, stream: true}},
	}
	for _, c := range cases {
		if got := factsOf(provider.Chat, []byte(c.body)).nd; got != c.want {
			t.Errorf("%s: need %v, want %v", c.name, got, c.want)
		}
	}
}

// A candidate the provider's capabilities test probed unable of is held out
// of the plan, its row among the left saying which tags — and one the test
// settled nothing of, or that needs nothing, is not. When every candidate
// of the plan is probed unable, none is held out: the plan answers with
// them, the score holding each last.
func TestCapOrderedHoldsTheProbedUnableOut(t *testing.T) {
	able := candidate{p: provider.Provider{ID: "able"}, model: "m"}
	lacks := candidate{p: provider.Provider{ID: "lacks", Caps: map[string]provider.ModelCaps{"m": {Tools: "no"}}}, model: "m"}
	nd := need{tools: true}
	row := func(c candidate) Weighed {
		return Weighed{ID: c.p.ID, Provider: c.p.ID, Model: c.model, Routing: provider.Score}
	}
	// the able one stays, the lacking one is out with its row among the left
	out := []candidate{able, lacks}
	pl := planned{order: []Weighed{row(able), row(lacks)}}
	got, pl := capOrdered(out, pl, nd)
	if len(got) != 1 || got[0].p.ID != "able" {
		t.Fatalf("the probed unable is out: %v", provOf(got))
	}
	if len(pl.order) != 1 || pl.order[0].Provider != "able" {
		t.Fatalf("the rows keep their seats: %v", pl.order)
	}
	if len(pl.left) != 1 || pl.left[0].Provider != "lacks" || len(pl.left[0].Lacks) != 1 || pl.left[0].Lacks[0] != "tools" {
		t.Fatalf("the left row says why: %+v", pl.left)
	}
	// every candidate probed unable: none is held out
	lacks2 := lacks
	lacks2.p = provider.Provider{ID: "lacks2", Caps: map[string]provider.ModelCaps{"m": {Tools: "no", Vision: "no"}}}
	out = []candidate{lacks, lacks2}
	pl = planned{order: []Weighed{row(lacks), row(lacks2)}}
	got, pl = capOrdered(out, pl, nd)
	if len(got) != 2 || len(pl.order) != 2 || len(pl.left) != 0 {
		t.Fatalf("none is held out when all lack: %v out, %d left, %d rows", provOf(got), len(pl.left), len(pl.order))
	}
	// a need for nothing holds nobody out
	nd = need{}
	out = []candidate{able, lacks}
	pl = planned{order: []Weighed{row(able), row(lacks)}}
	got, pl = capOrdered(out, pl, nd)
	if len(got) != 2 || len(pl.left) != 0 {
		t.Fatalf("no need holds nobody out: %v out, %d left", provOf(got), len(pl.left))
	}
	// untested says nothing: no letter, no holding out
	untested := candidate{p: provider.Provider{ID: "untested"}, model: "m"}
	out = []candidate{able, untested}
	pl = planned{order: []Weighed{row(able), row(untested)}}
	got, pl = capOrdered(out, pl, need{tools: true})
	if len(got) != 2 || len(pl.left) != 0 {
		t.Fatalf("untested is not held out: %v out, %d left", provOf(got), len(pl.left))
	}
}

// abilityOf is what the provider's letters say of what the request needs:
// a settled yes whole, a settled no nothing, a tag nothing settled half,
// and a need for nothing not weighed at all.
func TestAbilityOfTheLetters(t *testing.T) {
	nd := need{tools: true, vision: true}
	caps := func(tools, vision, stream string) candidate {
		return candidate{p: provider.Provider{ID: "p", Caps: map[string]provider.ModelCaps{"m": {Tools: tools, Vision: vision, Stream: stream}}}, model: "m"}
	}
	if got := abilityOf(caps("yes", "yes", "yes"), nd); got != 1 {
		t.Errorf("all yes: %v, want 1", got)
	}
	if got := abilityOf(caps("no", "no", "no"), nd); got != 0 {
		t.Errorf("all no: %v, want 0", got)
	}
	if got := abilityOf(caps("", "", ""), nd); got != 0.5 {
		t.Errorf("untested: %v, want 0.5", got)
	}
	if got := abilityOf(caps("yes", "", "no"), nd); got != 0.75 {
		t.Errorf("one yes, one untested: %v, want 0.75", got)
	}
	// a tag not needed is not asked of the letters: the stream no above is
	// nothing to a request that needs no stream
	if got := abilityOf(caps("yes", "yes", "no"), need{tools: true}); got != 1 {
		t.Errorf("what is not needed is not weighed: %v, want 1", got)
	}
	if got := abilityOf(caps("yes", "yes", "yes"), need{}); got != 1 {
		t.Errorf("no need is met whole: %v, want 1", got)
	}
}

// The score holds what a test settled unable of behind what it can do of
// the request: a seat that said no to every tag the request needs scores
// behind one that kept half of them, and a settled no is nothing, never
// the whole one a request that asks nothing is met with.
func TestAbilityRanksTheSettledUnableBehind(t *testing.T) {
	base := routes.Seat{Known: true, Health: 1, TTFT: 1000, TPS: 30, Used: 10}
	half := base // tools settled no, vision settled yes: half of the need
	half.Ability = 0.5
	lost := base // tools and vision both settled no: nothing of the need
	lost.Ability = 0
	if got, want := routes.ScoreOf(lost), routes.ScoreOf(half); got >= want {
		t.Errorf("the seat settled unable of everything scores %v, the one that kept half %v; the unable one goes behind", got, want)
	}
	whole := base // what the request needs, all settled yes
	whole.Ability = 1
	if got, want := routes.ScoreOf(whole), routes.ScoreOf(half); got <= want {
		t.Errorf("a seat met whole scores %v, one at half %v; the whole one goes first", got, want)
	}
}
