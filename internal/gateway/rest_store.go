package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/steady"
)

// A key out of credit, or out of quota, is skipped for the day
// (dayRest), and a key its vendor refuses outright (keyRefusedRest) the
// same: restingUntil, which is memory, a restart of magpie started each
// over with the refusal itself, its rest gone too (PaulT on Discord).
// Each such rest — its refusals in a row (Failures) and when it ends —
// is kept in rests.json beside providers.json, and read back once at
// start (loadRests), so the key rests out what is left and keeps its
// count. So is one out until the window it filled starts again, which is
// days off. The rest of them are a minute to a few hours, and the next
// request finds them again.

// restsPath is the file the day rests are kept in.
func restsPath() string { return filepath.Join(filepath.Dir(provider.Path()), "rests.json") }

// restsKeep: a rest that ended this long ago is let go from the file. In
// memory an ended rest keeps its count until an answer or "Try now"; a
// key left untried for over a month starts over.
const restsKeep = longestMonthly

// keptRest says r is a rest magpie keeps in rests.json: a key's day out
// of credit, a key's or account's day out of quota with no word of when
// (Failures counts its refusals in a row), a key its vendor refuses
// outright (keyRefusedRest), or one until the window it filled starts
// again (OpenCode Go's week: days off, read from the key's own usage). A
// credit refusal that said it can pass soon (creditRest) is not kept: it
// ends within the hour.
func keptRest(r Rest) bool {
	switch {
	case r.Why == failCredit && r.By == "credit" && r.Failures > 0:
		return true
	case r.Why == failQuota && r.By == "quota":
		return true
	case r.By == "key":
		return true
	case r.By == "window":
		// the window it filled is days out, read from the account's or
		// key's own usage, and nothing asks the vendor again without it:
		// OpenCode Go's week rested its key until 11-11 22:40, the swap
		// of 11-07 17:07 started magpie again, and the key was asked at
		// 17:08:39 and refused, as if it had never rested. A reading that
		// finds the window renewed lifts it (renewed), and that lift is
		// written too.
		return true
	}
	return false
}

// savedRest is a day rest as rests.json has it.
type savedRest struct {
	Rest
	First bool      `json:"first,omitempty"`
	AskAt time.Time `json:"askAt,omitzero"`
	Guess bool      `json:"guess,omitempty"`
	Agent string    `json:"agent,omitempty"`
	User  string    `json:"user,omitempty"`
	Said  string    `json:"said,omitempty"`
}

var restsSaving struct {
	sync.Mutex
	// last is what was written to path, so that an unchanged set isn't
	// written again
	path string
	last string
}

// restsLoaded: the paths whose file this process read already, read once
// each: a test's HOME is its own.
var restsLoaded = struct {
	sync.Mutex
	from map[string]bool
}{from: map[string]bool{}}

// saveRests writes the day rests to rests.json, through a file renamed
// over it, when they changed. Called after any change that set or dropped
// one, outside restingUntil.
func saveRests() {
	path := restsPath()
	restsSaving.Lock()
	defer restsSaving.Unlock()
	out := map[string]savedRest{}
	now := time.Now()
	restingUntil.Lock()
	for k, until := range restingUntil.m {
		r, ok := restingUntil.note[k]
		if !ok {
			continue
		}
		r.Until = until
		if !keptRest(r) || now.Sub(until) > restsKeep {
			continue
		}
		out[k] = savedRest{Rest: r, First: r.first, AskAt: r.askAt, Guess: r.guess, Agent: r.agent, User: r.user, Said: r.said}
	}
	restingUntil.Unlock()
	b, err := json.Marshal(out)
	if err != nil {
		return
	}
	if restsSaving.path == path && restsSaving.last == string(b) {
		return
	}
	if len(out) == 0 {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			restsSaving.path, restsSaving.last = path, string(b)
			return // none kept, none to write
		}
	}
	if os.MkdirAll(filepath.Dir(path), 0o700) != nil {
		return
	}
	target, err := edit.Target(path) // a symlink stays, its target written
	if err != nil {
		return
	}
	tmp := target + ".magpie-tmp"
	if os.WriteFile(tmp, b, 0o600) != nil {
		return
	}
	if steady.Rename(tmp, target) != nil {
		os.Remove(tmp)
		return
	}
	restsSaving.path, restsSaving.last = path, string(b)
}

// loadRests reads the day rests a magpie before this one kept, once for
// each file. A rest ended since keeps its count, as it would have here,
// unless it ended over restsKeep ago; a key no longer in providers.json
// is let go. A file that can't be read or parsed adds nothing: no rest is
// guessed from it, and the next change writes it anew. A rest this
// process has already is kept over the file's.
func loadRests() {
	path := restsPath()
	restsLoaded.Lock()
	defer restsLoaded.Unlock()
	if restsLoaded.from[path] {
		return
	}
	restsLoaded.from[path] = true
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var saved map[string]savedRest
	if json.Unmarshal(b, &saved) != nil {
		return
	}
	known, err := restKeysStored()
	if err != nil {
		return // which keys are there is unknown: none are rested by guess
	}
	now := time.Now()
	restingUntil.Lock()
	for k, v := range saved {
		r := v.Rest
		if !keptRest(r) || r.Until.IsZero() || now.Sub(r.Until) > restsKeep || !known(k) {
			continue
		}
		if _, ok := restingUntil.m[k]; ok {
			continue
		}
		// a rest further off than a rest can be was written with the clock
		// set back since: cut to the longest a rest is (longestMonthly),
		// as one ended that long ago is let go. A day rest is a day out
		// still; a window's, until the window renews
		if longest := now.Add(restsKeep); r.Until.After(longest) {
			r.Until = longest
		}
		r.Key = k
		r.first, r.askAt, r.guess, r.agent, r.user, r.said = v.First, v.AskAt, v.Guess, v.Agent, v.User, v.Said
		restingUntil.m[k] = r.Until
		restingUntil.note[k] = r
	}
	restingUntil.Unlock()
}

// restKeysStored says whether a rest key — a provider's id, or id#key,
// either followed by /model — names a key providers.json still has. An
// account's rest is kept by id@user (restKey), and is read back while its
// provider is there: the accounts are the sign-ins', which a start may not
// have read yet, and an account signed out since matches no candidate. Not
// read back, an account's day out started over at every restart — a swap
// asked each account out of quota again at once (zcode's four at the
// 11-06 22:34 swap), which is what the day rest is for.
func restKeysStored() (func(string) bool, error) {
	ps, err := provider.Stored()
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	ids := map[string]bool{} // every provider there is, key or account
	for _, p := range ps {
		keys[p.ID] = len(p.KeyList()) > 0
		ids[p.ID] = true
		for _, k := range p.KeyList() {
			keys[p.ID+"#"+k.ID] = true
		}
	}
	return func(k string) bool {
		k, _, _ = strings.Cut(k, "/")
		if i := strings.IndexByte(k, '@'); i > 0 {
			return ids[k[:i]]
		}
		return keys[k]
	}, nil
}
