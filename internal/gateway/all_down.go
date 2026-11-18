package gateway

// Every key and account that can answer a model resting is an outage the
// user sees only as failed requests (the review's D-01, 11-16: a channel
// that went resting, or all of them, was told of nowhere but the
// Providers page, which the user has to be on). The gateway notices the
// transition: a request that failed with every candidate of the model
// resting says the channels went out, once on the way in and at most once
// a minute while the outage holds. A request whose every try failed before
// it reached a vendor — the proxy or this computer's network down — is as
// unreachable, and says the same outage. An answer says the outage of that
// model is over, silently: the notice is the outage's, not the
// recovery's. A request the agent canceled (its 499, its going away)
// says nothing: the channels are not at fault for that.

import (
	"sync"
	"time"
)

// AllDownAlert is told of a model whose every key and account rests, as a
// request for it failed. Set by the app, which notifies; nil (magpie
// serve, the CLI) has the outages not said at all.
var AllDownAlert func(model string)

// AllDownRests is how long an outage is held before it is said again.
var AllDownRests = time.Minute

var allDown = struct {
	sync.Mutex
	said map[string]time.Time // model → when its outage was last said
}{}

// allDownSaid resets the state, as a test's home starts without one.
func allDownSaid() {
	allDown.Lock()
	allDown.said = map[string]time.Time{}
	allDown.Unlock()
}

// allResting says every candidate of cs is resting: by its own key or
// account, its model of it, or its provider's whole block. An empty plan
// isn't an outage, it is no one set to serve the model, and says nothing.
func allResting(cs []candidate) bool {
	if len(cs) == 0 {
		return false
	}
	for _, c := range cs {
		if _, resting := restOfAny(c); !resting {
			return false
		}
	}
	return true
}

// downSaid is what a failed request found: every candidate of the model
// resting. The notice goes on the way into the outage, and while it holds
// at most once a minute; none for a failure the agent itself caused (its
// 499).
func downSaid(cs []candidate, model string, status int) {
	if AllDownAlert == nil || status == 499 || model == "" || !allResting(cs) {
		return
	}
	sayDown(model)
}

// transportSaid is downSaid for a request whose every try failed before it
// reached a vendor: the proxy it sends through or this computer's network
// is down, so every candidate left would fail the same way. The model is
// as unreachable as one whose every candidate rests, and says the same
// outage episode, at most once a minute.
func transportSaid(model string, status int) {
	if AllDownAlert == nil || status == 499 || model == "" {
		return
	}
	sayDown(model)
}

// sayDown says model's outage unless it was said within AllDownRests.
func sayDown(model string) {
	allDown.Lock()
	say := allDown.said[model].IsZero() || time.Since(allDown.said[model]) >= AllDownRests
	if say {
		if allDown.said == nil {
			allDown.said = map[string]time.Time{}
		}
		allDown.said[model] = time.Now()
	}
	allDown.Unlock()
	if say {
		AllDownAlert(model)
	}
}

// upSaid says model's outage is over: a candidate of it answered. The
// way out is silent, so the model's next outage says itself again.
func upSaid(model string) {
	if model == "" {
		return
	}
	allDown.Lock()
	delete(allDown.said, model)
	allDown.Unlock()
}
