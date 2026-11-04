package gateway

import (
	"bytes"
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/yetone/magpie/internal/provider"
)

// scanMemoBody is a Chat conversation long enough to be held (validMemoMin)
// with everything the three body walks look for: a tool result mark enough
// times for a loop, an image, and a HEIC's type. Its top level is small
// and its messages list holds the length, as an agent's is.
func scanMemoBody(prefix string) []byte {
	var turns []string
	for i := range 12 {
		turns = append(turns,
			`{"role":"assistant","content":"answer `+strings.Repeat("a", 2000)+`"}`,
			`{"role":"tool","tool_call_id":"c`+prefix+`-`+string(rune('a'+i))+`","content":"`+strings.Repeat("line of output ", 200)+`"}`)
	}
	turns = append(turns,
		`{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://x/a.png"}}]}`,
		`{"role":"user","content":"a photo sent as image/heic"}`)
	return []byte(`{"model":"m","messages":[` + strings.Join(turns, ",") + `]}`)
}

// A request asks the same body whether it holds an image, a HEIC or a tool
// loop more than once as it is served (hasImage at several places of
// serve, the walk inside textOnlyBody, and heicAsJPEG's check before its
// walk), and each walk reads every byte. A held body is walked once for
// each, and a body not held, or released, is walked again: its memory may
// be given to another body that would read as this one.
func TestBodyScansRunOncePerHeldBody(t *testing.T) {
	ctx, done := context.WithCancel(context.Background())
	defer done()
	body := scanMemoBody("held")
	if len(body) < validMemoMin {
		t.Fatalf("the fixture is %d bytes, shorter than a held body", len(body))
	}
	holdBody(ctx, body)

	var walks atomic.Int64
	seen := func([]byte, scanKind) { walks.Add(1) }
	scanWalked.Store(&seen)
	defer scanWalked.Store(nil)

	walked := func() int64 { return walks.Load() }

	// every reader a request has for the same body, twice
	for range 2 {
		if !mayHoldHEIC(body) {
			t.Fatal("the fixture's HEIC was not found")
		}
	}
	if !hasImage(provider.Chat, body) {
		t.Fatal("the fixture's image was not found")
	}
	textOnlyBody(provider.Chat, body) // walks again before its walk
	if !hasImage(provider.Chat, body) {
		t.Fatal("the fixture's image was not found the second time")
	}
	for range 2 {
		if !mayLoop(provider.Chat, body) {
			t.Fatal("the fixture's tool results are not a loop's worth")
		}
	}
	if got := walked(); got != 3 {
		t.Fatalf("a held body was walked %d times for its three questions, want 3", got)
	}

	// a rewritten body is a different []byte: its own walks, not the held
	// body's
	rewritten := bytes.Clone(body)
	before := walked()
	hasImage(provider.Chat, rewritten)
	if walked() == before {
		t.Fatal("a rewritten body was served a held body's walk")
	}

	// what was kept goes when the request ends
	done()
	for deadline := time.Now().Add(5 * time.Second); ; {
		heldBodies.Lock()
		_, listed := heldBodies.scans[unsafe.SliceData(body)]
		heldBodies.Unlock()
		if !listed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a body's walks are still kept after its request ended")
		}
		time.Sleep(time.Millisecond)
	}
	before = walked()
	hasImage(provider.Chat, body)
	if walked() == before {
		t.Fatal("a released body was served its walks again")
	}
}
