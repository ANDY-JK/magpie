package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// healthRecord is how many models have a health record and the counts of
// the one that does: one model per request, so one record.
func healthRecord(t *testing.T) (n int, wins, loss float64) {
	t.Helper()
	modelHealth.Lock()
	defer modelHealth.Unlock()
	n = len(modelHealth.m)
	for _, v := range modelHealth.m {
		wins, loss = v.wins, v.loss
	}
	return
}

// A reply the upstream cut mid-way and the continuation recovered is still
// one observation against the model: it did cut off mid-reply, and the next
// ask finishing it doesn't undo that (health.go). Before continuation.go
// recorded it, a recovered cut left no trace at all — the try loop only
// saw the answer.
func TestMidStreamCutRecoveredCountsAgainstHealth(t *testing.T) {
	f := &cutChat{
		cuts:  map[int][]string{0: {chatText("kimi-k3", "hel")}},
		whole: []string{chatText("kimi-k3", "lo"), chatStop("kimi-k3")},
	}
	body := continuationServer(t, "kimi-k3", f)
	if got := textOf(body); got != "hello" {
		t.Fatalf("the continuation didn't finish the reply: %q", got)
	}
	if f.calls() != 2 {
		t.Fatalf("the cut reply wasn't asked again: %d calls", f.calls())
	}
	n, wins, loss := healthRecord(t)
	if n != 1 {
		t.Fatalf("health records %d models, want 1", n)
	}
	// the cut is one loss, and the answer that finished the reply is one
	// win, as every answer is (noteStable): two observations of one turn
	if wins != 1 || loss != 1 {
		t.Fatalf("wins %v, loss %v, want 1 and 1: the cut is a loss, the answer a win", wins, loss)
	}
}

// A reply that is cut and NOT recovered — a tool call was begun, so there
// is nothing to go on from — is counted once, by the try loop's own
// failure path: the continuation must not add a second observation of the
// same cut.
func TestMidStreamCutNotRecoveredCountsOnce(t *testing.T) {
	f := &cutChat{
		cuts: map[int][]string{0: {
			chatText("kimi-k3", "hel"),
			`{"id":"c1","model":"kimi-k3","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"ci"}}]}}]}`,
		}},
	}
	body := continuationServer(t, "kimi-k3", f)
	if f.calls() != 1 {
		t.Fatalf("a reply with a tool call begun was asked again: %d calls", f.calls())
	}
	if _, _, errs := messageStops(body); errs != 1 {
		t.Fatalf("no error event in the stream: %s", body)
	}
	n, wins, loss := healthRecord(t)
	if n != 1 || wins != 0 || loss != 1 {
		t.Fatalf("%d models, wins %v, loss %v, want one model, no win, one loss", n, wins, loss)
	}
}

// A reply that never cut leaves no loss: its only observation is the
// answer itself, so nothing is held against the model.
func TestMidStreamNoCutLeavesNoHealth(t *testing.T) {
	f := &cutChat{whole: []string{chatText("kimi-k3", "hello"), chatStop("kimi-k3")}}
	continuationServer(t, "kimi-k3", f)
	if n, wins, loss := healthRecord(t); n != 1 || wins != 1 || loss != 0 {
		t.Fatalf("%d models, wins %v, loss %v, want one model, one win, no loss", n, wins, loss)
	}
}

// A plan of one member is the common case, and its 500s must count: the
// try loop's own failure path notes an intermittent failure only where a
// rest is made, which a last candidate has none of (health.go).
func TestMidStreamLastMemberFailureCounts(t *testing.T) {
	fresh(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":{"message":"upstream blew up"}}`)
	}))
	t.Cleanup(up.Close)
	p := provider.Provider{ID: "solo", Name: "Solo", Key: "k", Models: []string{"m"}, Chat: up.URL + "/v1"}
	if err := provider.Save(p); err != nil {
		t.Fatal(err)
	}
	code, body := post(t, "/v1/chat/completions", `{"model":"solo/m","messages":[{"role":"user","content":"hi"}]}`)
	if code == 200 {
		t.Fatalf("a 500 was answered 200: %s", body)
	}
	n, wins, loss := healthRecord(t)
	if n != 1 || loss < 1 || wins != 0 {
		t.Fatalf("%d models, wins %v, loss %v: want one model with a loss", n, wins, loss)
	}
}
