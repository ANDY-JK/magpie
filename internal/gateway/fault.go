package gateway

// A debugging fault injector (F-07, the review): a gateway answering as a
// vendor that fails is the only way to watch a group's fallback, a
// provider's patience or a rest cap work under the fault it was set for.
// It is env-only and off unless MAGPIE_FAULT says otherwise — a switch in
// Settings would be one a user could leave on by accident:
//
//	MAGPIE_FAULT="relay=500,openrouter=delay:2000,gemini=cut"
//
// Each rule is <provider id, or *>=<fault>[,<fault>…], with the first rule
// whose id matches the provider chosen:
//
//	500            answer every request with a 500 (any 4xx/5xx: status:429)
//	delay:2000     wait that many milliseconds before the vendor is asked
//	cut            begin an answer and stop half-way, as a dropped stream
//
// An injected answer carries X-Magpie-Fault naming the rule, and the try's
// error says it, so a Routing trace or the recent calls show it was the
// fault and not the vendor.

import (
	"context"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

type faultRule struct {
	id     string
	mode   string // "status", "delay" or "cut"
	status int
	delay  time.Duration
	raw    string
}

// faultCache keeps the parsed rules of MAGPIE_FAULT, parsed again only
// when the env changes (a test's).
var faultCache struct {
	sync.Mutex
	env    string
	parsed bool
	rules  []faultRule
}

func faultRules() []faultRule {
	v := strings.TrimSpace(os.Getenv("MAGPIE_FAULT"))
	faultCache.Lock()
	defer faultCache.Unlock()
	if faultCache.parsed && faultCache.env == v {
		return faultCache.rules
	}
	faultCache.env, faultCache.parsed = v, true
	faultCache.rules = parseFaults(v)
	return faultCache.rules
}

func parseFaults(v string) []faultRule {
	if v == "" {
		return nil
	}
	var out []faultRule
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, fault, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(fault) == "" {
			continue
		}
		r := faultRule{id: strings.TrimSpace(id), raw: strings.TrimSpace(part)}
		switch {
		case fault == "cut":
			r.mode = "cut"
		case strings.HasPrefix(fault, "delay:"):
			if ms, err := strconv.Atoi(strings.TrimPrefix(fault, "delay:")); err == nil && ms >= 0 {
				r.mode, r.delay = "delay", time.Duration(ms)*time.Millisecond
			}
		case fault == "500":
			r.mode, r.status = "status", http.StatusInternalServerError
		case strings.HasPrefix(fault, "status:"):
			if n, err := strconv.Atoi(strings.TrimPrefix(fault, "status:")); err == nil && n >= 400 && n <= 599 {
				r.mode, r.status = "status", n
			}
		}
		if r.mode != "" {
			out = append(out, r)
		}
	}
	return out
}

// faultOf is the rule for p, if one matches by id or "*".
func faultOf(p provider.Provider) (faultRule, bool) {
	for _, r := range faultRules() {
		if r.id == p.ID || r.id == "*" {
			return r, true
		}
	}
	return faultRule{}, false
}

// faultAnswer waits out a delay, or answers in the rule's place: body is
// the fault's own text, carried by every injected answer.
func faultAnswer(ctx context.Context, r faultRule) (res *http.Response, wait bool, err error) {
	switch r.mode {
	case "delay":
		select {
		case <-time.After(r.delay):
			return nil, true, nil
		case <-ctx.Done():
			return nil, true, ctx.Err()
		}
	case "status":
		body := `{"error":{"message":"magpie fault injection: ` + r.raw + `","type":"injected_fault"}}`
		return &http.Response{
			StatusCode: r.status,
			Status:     http.StatusText(r.status),
			Header:     http.Header{"Content-Type": {"application/json"}, "X-Magpie-Fault": {r.raw}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, false, nil
	case "cut":
		// the first half of a reply, then the stream ends: what a dropped
		// connection looks like to the reader
		half := `{"id":"injected","choices":[{"delta":{"role":"assistant","content":"a reply that sto`
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     http.StatusText(http.StatusOK),
			Header:     http.Header{"Content-Type": {"application/json"}, "X-Magpie-Fault": {r.raw}},
			Body:       io.NopCloser(strings.NewReader(half)),
		}, false, nil
	}
	return nil, true, nil
}
