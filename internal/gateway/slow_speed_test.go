package gateway

import (
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// A start past the group's deadline is a speed sample, the deadline being
// the least it can have taken: Balance weighs the member as slow
// (routes.Seat.TTFT, slowSeats) and asks it after those that answer.
// Before, only answers were timed, so a member that timed out half its
// asks was weighed as fast, and asked first again every time (77625 on
// 11-07: 35 asks past 30s in under two hours, each a 30s wait).
func TestSlowStartIsASpeedSample(t *testing.T) {
	fresh(t)
	up := &keyedSlow{key: "k1", model: "m"}
	up.slow.Store(true)
	serveOn(t, "a", "k1", []string{"m"}, up, "k2")
	// the group's deadline is 1s, so the sample is at least that
	if err := provider.SaveGroup(provider.Group{Name: "G", Members: []string{"a/m"}, Routing: provider.Ordered, FirstToken: 1}); err != nil {
		t.Fatal(err)
	}
	s := New()
	code, body := postAs(t, s, "", `{"model":"group/g","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	if code != 200 || !strings.Contains(body, "from b") {
		t.Fatalf("the group: %d %s", code, body)
	}
	slow := candidate{rest: "a#" + provider.KeyID("k1"), model: "m"}
	if ttft, _ := speedOf(slow, time.Now()); ttft < 1000 {
		t.Fatalf("a start past the deadline is no speed sample: %vms", ttft)
	}
}
