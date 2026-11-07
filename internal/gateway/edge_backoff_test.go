package gateway

import (
	"net/http"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// A provider whose edge firewall keeps blocking this computer's address
// sits out longer each time the block is still there when its rest ends:
// three minutes, then twice as long, up to an hour. Before, every block
// rested it three minutes, whatever its own history, so zcode was asked
// again every three minutes through the whole of 11-07's block (11:05 to
// past 12:00), each ask another one for the firewall to count. A success
// ends the run at once, and a block answering while its rest still stands
// is that same block, not another.
func TestEdgeBlockBacksOffFurtherEachTime(t *testing.T) {
	fresh(t)
	s := New()
	p := provider.Provider{ID: "zb", Key: "k", Chat: "http://127.0.0.1:1/v1"}
	c := candidate{p: p, model: "m", rest: "zb#k"}
	block := []byte("ZCode: 405 Method Not Allowed — " + provider.BlockedHint)
	rest := func() Rest {
		t.Helper()
		r := s.restAfter(c, http.StatusMethodNotAllowed, http.Header{}, block)
		if r.By != "blocked" || r.Key != blockedKey(p) {
			t.Fatalf("the block's rest: %+v, want the provider's own", r)
		}
		return r
	}
	left := func(r Rest) time.Duration { return time.Until(r.Until) }
	// the first block: three minutes, as the plugin's own hold
	if r := rest(); left(r) < edgeRest-time.Second || left(r) > edgeRest+time.Second {
		t.Fatalf("the first block rests %v, want about %v", left(r).Round(time.Second), edgeRest)
	}
	// another account's 405 in the same block, planned before this rest
	// existed, doesn't climb the ladder
	if r := rest(); left(r) > edgeRest+time.Second {
		t.Fatalf("the same block climbed to %v", left(r).Round(time.Second))
	}
	// still blocked when its rest ends: twice as long, then four times
	for _, want := range []time.Duration{2 * edgeRest, 4 * edgeRest} {
		restingUntil.Lock()
		restingUntil.m[blockedKey(p)] = time.Now().Add(-time.Second)
		restingUntil.Unlock()
		if r := rest(); left(r) < want-time.Second || left(r) > want+time.Second {
			t.Fatalf("still blocked rests %v, want about %v", left(r).Round(time.Second), want)
		}
	}
	// up to an hour, and no further
	var last Rest
	for range 10 {
		restingUntil.Lock()
		restingUntil.m[blockedKey(p)] = time.Now().Add(-time.Second)
		restingUntil.Unlock()
		last = rest()
	}
	if left(last) > edgeCap+time.Second || left(last) < edgeCap-time.Second {
		t.Fatalf("the run's end rests %v, want %v", left(last).Round(time.Second), edgeCap)
	}
	// an answer from any of its seats ends the run: the next block is
	// three minutes again
	servedCandidate(c, 1)
	if r := rest(); left(r) < edgeRest-time.Second || left(r) > edgeRest+time.Second {
		t.Fatalf("after an answer the block rests %v, want about %v", left(r).Round(time.Second), edgeRest)
	}
}

// A block long after the last one starts the ladder over.
func TestEdgeBlockForgetsAnOldRun(t *testing.T) {
	fresh(t)
	s := New()
	p := provider.Provider{ID: "zb", Key: "k", Chat: "http://127.0.0.1:1/v1"}
	c := candidate{p: p, model: "m", rest: "zb#k"}
	block := []byte("ZCode: 405 Method Not Allowed — " + provider.BlockedHint)
	s.restAfter(c, http.StatusMethodNotAllowed, http.Header{}, block)
	restingUntil.Lock()
	restingUntil.m[blockedKey(p)] = time.Now().Add(-time.Second)
	restingUntil.Unlock()
	if r := restAfterAt(s, c, block); time.Until(r.Until) < 2*edgeRest-time.Second {
		t.Fatalf("the second block rests %v, want about %v", time.Until(r.Until).Round(time.Second), 2*edgeRest)
	}
	// the last block was long ago
	routed.Lock()
	routed.edge["zb"] = edgeRun{n: 5, at: time.Now().Add(-edgeForget - time.Minute)}
	routed.Unlock()
	restingUntil.Lock()
	restingUntil.m[blockedKey(p)] = time.Now().Add(-time.Second)
	restingUntil.Unlock()
	if r := restAfterAt(s, c, block); time.Until(r.Until) > edgeRest+time.Second {
		t.Fatalf("an old run carried on: rests %v", time.Until(r.Until).Round(time.Second))
	}
}

func restAfterAt(s *Server, c candidate, block []byte) Rest {
	return s.restAfter(c, http.StatusMethodNotAllowed, http.Header{}, block)
}
