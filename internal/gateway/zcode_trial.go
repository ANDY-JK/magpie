package gateway

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/yetone/magpie/internal/provider"
)

// zcodeEndedWords and zcodeSpentWords are the ZCode plugin's own answers
// for a Start Plan trial entry (<model>-Trial) it can't serve, given before
// anything is sent: a 400 "ZCode has no active limited quota for
// GLM-5.3-Trial" when no live plan holds the model any more (the trial
// ended), and a 429
// "ZCode's limited quota for glm-5.3 is exhausted or temporarily
// unavailable" when its bucket is spent. The plugin's words, after any
// "<provider>: " put in front of them.
var (
	zcodeEndedWords = regexp.MustCompile(`(?:^|: )ZCode has no active limited quota for (\S+)$`)
	zcodeSpentWords = regexp.MustCompile(`(?:^|: )ZCode's limited quota for (\S+) is exhausted or temporarily unavailable$`)
)

// zcodeConcurrencyWords is how the ZCode plugin answers a request past the
// plan's in-flight cap: the vendor's own code, kept in the message, and
// the plugin's words for what it means (its BUSY_HINT).
var zcodeConcurrencyWords = regexp.MustCompile(`(?i)\((?:429), code 300[89]\)|ZCode serves one account a few requests at a time`)

// zcodeConcurrency says a 429 from a ZCode plugin account is the plan's
// in-flight cap (3008, "user concurrency limit exceeded", or 3009 for a
// model) and not a rate limit: the request's turn had not come, and the
// same request a moment later is served. The plugin waits about ten
// seconds over it before it answers, so magpie rests the account a short
// while and asks again — no rung of the rate ladder, which read it as the
// account being rate limited over and over: on 11-07 zcode@龙_2608 sat out
// thirty minutes by 01:02 and zcode@唯心_9997 sixteen by 01:04, from
// refusals that only said the account was busy.
func zcodeConcurrency(c candidate, status int, body []byte) bool {
	if status != http.StatusTooManyRequests || c.p.PluginProvider() != "zcode" {
		return false
	}
	return zcodeConcurrencyWords.Match(wordsOf(body))
}

// zcodeTrialRefused says c, a ZCode plugin account asked for one of its
// Start Plan trial entries, was refused for that entry's own quota
// (zcodeEndedWords, zcodeSpentWords), naming the model asked for. The
// trial is that model's: the account's GLM Coding Plan still serves its
// own models, so only the trial entry rests (#3661, orion-nebula-dev: two
// expired trials rested "zcode@zcode", and GLM-5.3 and GLM-5.3-Flash with
// it). Any other
// refusal of a ZCode account, an account-wide 429 or 1113 among them,
// rests the account as before.
func zcodeTrialRefused(c candidate, status int, body []byte) bool {
	if c.p.PluginProvider() != "zcode" || !strings.HasSuffix(strings.ToLower(c.model), "-trial") {
		return false
	}
	said := strings.TrimSpace(vendorMessage(body))
	switch status {
	case http.StatusBadRequest:
		m := zcodeEndedWords.FindStringSubmatch(said)
		return m != nil && strings.EqualFold(m[1], c.model)
	case http.StatusTooManyRequests:
		// the plugin names the bucket's own model, the entry less -Trial
		m := zcodeSpentWords.FindStringSubmatch(said)
		return m != nil && strings.EqualFold(m[1]+"-Trial", c.model)
	}
	return false
}

// zcodeTrialBlocked says c, a ZCode plugin account asked for one of its
// Start Plan trial entries, got zcode.z.ai's edge firewall's block page
// (Alibaba Cloud's 405, provider.EdgeBlocked, as the plugin's startBlocked
// words it). A trial entry is sent to zcode.z.ai alone; a dual account's
// plain models go to its GLM Coding Plan at api.z.ai, which answered while
// zcode.z.ai blocked the address (plugins#545, jinpy666). So the block
// rests the trial entry, and the account's plain model is still asked: on
// a gift-only account it goes to zcode.z.ai too, is blocked the same, and
// that rests the account (windy_66454 on Discord, 10-31: glm-trial's two
// members, GLM-5.3-Flash and GLM-5.3-Flash-Trial, shared one account rest
// and the plain one was never asked). ZCode's risk control blocking the
// account (code 3012, failRestricted) is the account's, and is read before.
func zcodeTrialBlocked(c candidate, status int, body []byte) bool {
	if status < 400 || c.p.PluginProvider() != "zcode" || !strings.HasSuffix(strings.ToLower(c.model), "-trial") {
		return false
	}
	return provider.EdgeBlocked(wordsOf(body)) && !restrictedSaid(status, wordsOf(body))
}
