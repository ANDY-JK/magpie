package gateway

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// giveModelRPM keeps the model's own Requests a minute (settings' ModelRPM,
// the editor's Names & levels) for the test's provider, as its editor's
// Save would.
func giveModelRPM(t *testing.T, pid, model string, n int) {
	t.Helper()
	if err := provider.SetModelPrefs(pid, map[string]provider.ModelPref{model: {RPM: &n}}); err != nil {
		t.Fatal(err)
	}
}

// cappedMinute is the minute's shape cap requests fill: they all go at
// once on the held clock, the next waits to go a window and the slack
// after them, and the vendor sees no more than cap until then. The limit
// is the one in force for the model the body asks for.
func cappedMinute(t *testing.T, v *minuteVendor, gw string, cap int, what string) {
	t.Helper()
	clk := v.clock
	t0 := clk.Now()
	for i := range cap {
		if r := clk.atOnce(t, fmt.Sprintf("request %d, with room in the minute,", i+1), askRPM(context.Background(), gw, rlPath, rlBody)); r.err != nil || r.code != 200 || !strings.Contains(r.body, `"ok"`) {
			t.Fatalf("request %d: %v %d %s", i+1, r.err, r.code, r.body)
		}
	}
	next := askRPM(context.Background(), gw, rlPath, rlBody)
	until := clk.waitsUntil(t, what)
	if want := t0.Add(rpmWindow + rpmSlack); !until.Equal(want) {
		t.Fatalf("%s waits to go %s after the first, want the window and slack, %s", what, until.Sub(t0), want.Sub(t0))
	}
	if n := len(v.times()); n != cap {
		t.Fatalf("the vendor saw %d while %s waits, want %d", n, what, cap)
	}
	clk.advance(until.Sub(clk.Now()))
	if r := rpmAnswered(t, what, next); r.err != nil || r.code != 200 || !strings.Contains(r.body, `"ok"`) {
		t.Fatalf("%s: %v %d %s", what, r.err, r.code, r.body)
	}
}

// The key or account is not the only one a minute is capped for: a model
// with no MaxRPM of its key's, but 2 a minute of its own (coeo91's free
// models), sends two and the third waits for room in the model's minute.
func TestModelRPMModelTierCapsTheMinute(t *testing.T) {
	slackMinute(t, 1500*time.Millisecond, time.Minute)
	v := &minuteVendor{clock: holdMinute(t)}
	_, gw := rpmGateway(t, v, 0)
	giveModelRPM(t, "rl", "m", 2)
	cappedMinute(t, v, gw, 2, "the third")
}

// Both set, the smaller is the one that counts — a limit only ever
// tightens: the model's 2 under the key's 5 caps the model at 2, and the
// key's 1 under the model's 5 caps it at 1.
func TestModelRPMOfKeyAndModelIsTheSmaller(t *testing.T) {
	slackMinute(t, 1500*time.Millisecond, time.Minute)
	v := &minuteVendor{clock: holdMinute(t)}
	_, gw := rpmGateway(t, v, 5)
	giveModelRPM(t, "rl", "m", 2)
	cappedMinute(t, v, gw, 2, "the third, the model's two counting")

	// and the key's one, under the model's five
	v2 := &minuteVendor{clock: holdMinute(t)}
	_, gw2 := rpmGateway(t, v2, 1)
	giveModelRPM(t, "rl", "m", 5)
	cappedMinute(t, v2, gw2, 1, "the second, the key's one counting")
}

// No RPM of the model's own — none kept, none said — the model sends what
// its key or account's MaxRPM allows, as it did before there was a model's
// own to set.
func TestModelRPMUnsetKeepsTheKeyTier(t *testing.T) {
	slackMinute(t, 1500*time.Millisecond, time.Minute)
	v := &minuteVendor{clock: holdMinute(t)}
	_, gw := rpmGateway(t, v, 2)
	cappedMinute(t, v, gw, 2, "the third, the key's two counting")
}
