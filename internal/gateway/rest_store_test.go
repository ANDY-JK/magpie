package gateway

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// restartRests is a magpie started again: what restingUntil held is
// gone, and the file is read as ListenAndServe reads it.
func restartRests() {
	restingUntil.Lock()
	restingUntil.m, restingUntil.note = map[string]time.Time{}, map[string]Rest{}
	restingUntil.Unlock()
	restsLoaded.Lock()
	restsLoaded.from = map[string]bool{}
	restsLoaded.Unlock()
	restsSaving.Lock()
	restsSaving.path, restsSaving.last = "", ""
	restsSaving.Unlock()
	loadRests()
}

// keyedOpenAI saves a provider with one key, as the user has it, and is
// that key as a candidate.
func keyedOpenAI(t *testing.T) candidate {
	t.Helper()
	p := provider.Provider{ID: "openai", Name: "OpenAI", Key: "sk-x", Chat: "http://127.0.0.1:1/v1", Models: []string{"gpt-x"}}
	if err := provider.Save(p); err != nil {
		t.Fatal(err)
	}
	return candidate{p: p, model: "gpt-x", rest: "openai"}
}

// PaulT on Discord: a restart of magpie started an out-of-credit key's
// rest over with the refusal itself, and lifted it. It now rests out what
// is left of the day, and keeps its refusals in a row.
func TestKeyCreditRestOutlivesRestart(t *testing.T) {
	fresh(t)
	s := New()
	key := keyedOpenAI(t)
	id := key.restKey()
	refuse := func() Rest {
		return s.restAfter(key, http.StatusPaymentRequired, http.Header{}, []byte(openAICreditsGone))
	}
	refuse() // the day
	expire(id)
	saveRests()
	r := refuse() // the day, refusals 2
	if r.Failures != 2 {
		t.Fatalf("second refusal: failures %d", r.Failures)
	}

	restartRests()
	got, ok := restOf(id)
	if !ok {
		t.Fatal("after a restart the key rests no more")
	}
	if !got.Until.Equal(r.Until) || got.Failures != 2 || got.Why != failCredit || got.By != "credit" || got.Key != id {
		t.Fatalf("after a restart: %+v, want until %v, 2 refusals", got, r.Until)
	}
	// refused again while it still rests: the same rest, its count kept
	if r := refuse(); r.Failures != 2 {
		t.Fatalf("refused while resting after a restart: failures %d", r.Failures)
	}
	// its rest ended, magpie started again, and refused: one refusal more
	expire(id)
	saveRests()
	restartRests()
	if _, ok := restOf(id); ok {
		t.Fatal("an ended rest rests again after a restart")
	}
	if r := refuse(); r.Failures != 3 || time.Until(r.Until) < dayRest-time.Minute {
		t.Fatalf("refused after a restart: failures %d, rests %v, want the day", r.Failures, time.Until(r.Until).Round(time.Second))
	}

	// an answer starts it over, after a restart too
	servedCandidate(key, 1)
	restartRests()
	if r := refuse(); r.Failures != 1 {
		t.Fatalf("after an answer and a restart: failures %d", r.Failures)
	}
	// "Try now" too
	expire(id)
	saveRests()
	refuse() // refusals 2
	s.Unrest(id)
	restartRests()
	if _, ok := restOf(id); ok {
		t.Fatal("Try now's lift came back after a restart")
	}
	if r := refuse(); r.Failures != 1 {
		t.Fatalf("after Try now and a restart: failures %d", r.Failures)
	}
}

// An account's day out (restKey: provider@user) is kept over a restart as
// a key's is. Before, only keys were read back, so every restart asked
// each account out of quota again at once: on 11-06 the swap at 22:34 was
// followed by the four ZCode accounts' refusals at 22:35, each a day's
// again and each counted as that account's first of the day, so the newly
// granted plan was asked again while it had not started.
func TestAccountQuotaRestOutlivesRestart(t *testing.T) {
	fresh(t)
	forgetRouting()
	old := allowances
	allowances = func(string) map[string]provider.Allowance { return nil }
	defer func() { allowances = old }()
	s := New()
	if err := provider.Save(provider.Provider{ID: "zplug", Name: "ZCode", Chat: "http://127.0.0.1:1/v1"}); err != nil {
		t.Fatal(err)
	}
	acct := candidate{
		p:     provider.Provider{ID: "zplug", Chat: "http://127.0.0.1:1/v1", Account: &provider.Account{Agent: "plugin", User: "me"}},
		model: "GLM-5.3-Flash",
		rest:  "zplug#3teif9",
	}
	id := acct.restKey()
	if id != "zplug@me" {
		t.Fatalf("an account's rest key is %q, want zplug@me", id)
	}
	// the plugin's own 429 for a gift out of this model, naming no time
	spent := []byte(`{"type":"error","error":{"type":"rate_limit_error","message":"ZCode's limited quota for GLM-5.3-Flash is exhausted or temporarily unavailable"}}`)
	r := s.restAfter(acct, http.StatusTooManyRequests, http.Header{}, spent)
	if r.Why != failQuota || r.By != "quota" || !r.guess || !r.first {
		t.Fatalf("the plugin's spent answer: %+v", r)
	}
	saveRests()

	restartRests()
	got, ok := restOf(id)
	if !ok {
		t.Fatal("after a restart the account rests no more")
	}
	if !got.Until.Equal(r.Until) || got.Why != failQuota || got.By != "quota" || !got.guess || !got.first || got.agent != "plugin" || got.user != "me" {
		t.Fatalf("after a restart: %+v, want until %v", got, r.Until)
	}

	// a provider removed while magpie was off is let go, its accounts with it
	if err := provider.Delete("zplug"); err != nil {
		t.Fatal(err)
	}
	restartRests()
	if _, ok := restOf(id); ok {
		t.Fatal("an account of a removed provider rests after a restart")
	}
}

// A key removed while magpie was off is let go, and one ended over a
// month ago starts over; a rest that isn't a day's isn't kept.
func TestKeyCreditRestFileForgets(t *testing.T) {
	fresh(t)
	s := New()
	key := keyedOpenAI(t)
	s.restAfter(key, http.StatusPaymentRequired, http.Header{}, []byte(openAICreditsGone))
	// a rate limit on another key is no day rest
	other := candidate{p: provider.Provider{ID: "openai"}, model: "gpt-y", rest: "openai#other"}
	s.restAfter(other, http.StatusTooManyRequests, http.Header{"Retry-After": {"600"}}, []byte(`{"error":{"message":"rate limited"}}`))
	saveRests()
	if err := provider.Delete("openai"); err != nil {
		t.Fatal(err)
	}
	restartRests()
	if _, ok := restOf(key.restKey()); ok {
		t.Fatal("a removed key rests after a restart")
	}
	if _, ok := restOf(other.restKey()); ok {
		t.Fatal("a rate limit's rest was kept over a restart")
	}

	key = keyedOpenAI(t)
	s.restAfter(key, http.StatusPaymentRequired, http.Header{}, []byte(openAICreditsGone))
	restingUntil.Lock()
	restingUntil.m[key.restKey()] = time.Now().Add(-restsKeep - time.Hour)
	restingUntil.Unlock()
	restsSaving.Lock()
	restsSaving.last = "" // written as it is now
	restsSaving.Unlock()
	saveRests()
	restartRests()
	if r := s.restAfter(key, http.StatusPaymentRequired, http.Header{}, []byte(openAICreditsGone)); r.Failures != 1 {
		t.Fatalf("a rest ended over a month ago climbed on: failures %d", r.Failures)
	}
}

// A file cut short, or not JSON, rests no one and doesn't stop the
// start; the next change writes it anew.
func TestKeyCreditRestFileCorrupt(t *testing.T) {
	for _, bad := range []string{`{"openai":{"why":"credit","by":"credit","failures":3,"until":"`, "not json", `[1,2]`, `{"openai":{"why":"credit","by":"credit","failures":3,"until":"never"}}`} {
		fresh(t)
		s := New()
		key := keyedOpenAI(t)
		if err := os.WriteFile(restsPath(), []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		restartRests()
		restingUntil.Lock()
		n := len(restingUntil.m)
		restingUntil.Unlock()
		if n != 0 {
			t.Fatalf("%q: %d rests read from a bad file", bad, n)
		}
		if r := s.restAfter(key, http.StatusPaymentRequired, http.Header{}, []byte(openAICreditsGone)); r.Failures != 1 {
			t.Fatalf("%q: failures %d after a bad file", bad, r.Failures)
		}
		restartRests()
		if r, ok := restOf(key.restKey()); !ok || r.Failures != 1 {
			t.Fatalf("%q: the file wasn't written anew: %+v %v", bad, r, ok)
		}
	}
}

// providers.json that can't be read says nothing of which keys are there:
// no rest is read on a guess.
func TestKeyCreditRestProvidersUnreadable(t *testing.T) {
	fresh(t)
	s := New()
	key := keyedOpenAI(t)
	s.restAfter(key, http.StatusPaymentRequired, http.Header{}, []byte(openAICreditsGone))
	// a folder where the file was: read, it fails
	if err := os.Remove(provider.Path()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(provider.Path(), 0o700); err != nil {
		t.Fatal(err)
	}
	restartRests()
	restingUntil.Lock()
	n := len(restingUntil.m)
	restingUntil.Unlock()
	if n != 0 {
		t.Fatalf("%d rests read with providers.json unreadable", n)
	}
}
