package gui

import (
	"context"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/update"
)

func init() {
	resetSkew = func() {
		replacedOlder.Store(0)
		gatewayOther.Store(nil)
	}
}

// A magpie of another version serving the gateway that isn't an older
// copy of this app (magpie serve, a newer magpie, a build of its own) is
// someone else's to quit: it is left running, and the page is told what
// it is, for the Agents page to say so.
func TestGatewaySkewOfAnotherIsSaidNotQuit(t *testing.T) {
	for _, c := range []struct {
		name, v, serves string
		older, newer    bool
	}{
		{"serve", "0.1.2400", "serve", true, false},
		{"newer", "0.1.2600", "app", false, true},
		{"own build", "0.1.2400-3-gabcdef1", "app", true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			addr := skewHome(t, "0.1.2500")
			ended := startFakeMagpie(t, addr, c.v, c.serves)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go watchGateway(ctx, 50*time.Millisecond)
			if update.Independent() {
				// this build follows no release feed (update.Independent):
				// no version of it counts as released, so another magpie
				// serving the gateway is nobody's to judge — the page isn't
				// told of a skew, and nothing is quit
				time.Sleep(time.Second)
				if skew := gatewayOther.Load(); skew != nil {
					t.Fatalf("an independent build told of a skew: %+v", skew)
				}
				select {
				case <-ended:
					t.Fatal("quit a magpie beside a build that follows no feed")
				default:
				}
				return
			}
			var sk *gatewaySkew
			for deadline := time.Now().Add(5 * time.Second); sk == nil; time.Sleep(20 * time.Millisecond) {
				sk = gatewayOther.Load()
				if time.Now().After(deadline) {
					t.Fatal("the page isn't told of it")
				}
			}
			select {
			case <-ended:
				t.Fatal("quit a magpie that isn't an older copy of this app")
			case <-time.After(500 * time.Millisecond):
			}
			if sk.Version != c.v || sk.Mine != "0.1.2500" || sk.Serves != c.serves || sk.Older != c.older || sk.Newer != c.newer || sk.Replacing || sk.Pid == 0 {
				t.Fatalf("told %+v", sk)
			}
			if served.Load() != nil {
				t.Fatal("served beside it")
			}
			if got := state().GatewayOther; got == nil || got.Version != c.v {
				t.Fatalf("state's gatewayOther: %+v", got)
			}
		})
	}
}

// A copy of this very version serving the gateway (a second process of
// it) is not a skew; nor does a dev build of this magpie say one.
func TestGatewaySkewNotForTheSameOrADevBuild(t *testing.T) {
	was := gateway.Version
	defer func() { gateway.Version = was }()
	gateway.Version = "0.1.2500"
	if sk := skewOf(gateway.Served{Running: true, Version: "v0.1.2500", Serves: "app", Pid: 9}); sk != nil {
		t.Fatalf("the same version: %+v", sk)
	}
	gateway.Version = "0.1.2500-4-g0123abc"
	if sk := skewOf(gateway.Served{Running: true, Version: "0.1.2400", Serves: "app", Pid: 9}); sk != nil {
		t.Fatalf("a dev build: %+v", sk)
	}
}
