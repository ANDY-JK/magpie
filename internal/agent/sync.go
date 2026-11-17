package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/filememo"
	"github.com/yetone/magpie/internal/provider"
	"gopkg.in/yaml.v3"
)

// Most agents can't ask the gateway which models it has: magpie writes the
// catalog into a file of the agent's own (Pi's models.json, OpenCode's
// provider block, Codex's model catalog …) when a model is picked through
// magpie. That list was the catalog as it was then, so a provider added
// later never reached the agent, and one removed stayed listed. SyncCatalog
// rewrites each such list whenever the catalog may have changed.

var syncing struct {
	sync.Mutex
	running, again bool
}

// syncFailedMu guards syncFailed, the sync failures a round of SyncCatalog
// or a rewire didn't say: they were dropped by an error that no caller
// carries on, and a model list that wasn't written is one the next change,
// or a pick, writes again. A rekey and a catalog round run at once, so the
// map is one lock's, not two processes' files.
var (
	syncFailedMu sync.Mutex
	syncFailed   map[string][]error
)

// syncFailures are the sync failures a round of SyncCatalog swallowed
// (agent id, the error): one no caller carries is otherwise gone, and a
// model list that didn't write reaches the agent on its next pick, not
// now. A sync that wrote again drops its failures (clearSyncFailure); the
// same failure, said by another round, is one kept error, not two
// (noteSyncFailure).
func syncFailures() map[string][]error {
	syncFailedMu.Lock()
	defer syncFailedMu.Unlock()
	if syncFailed == nil {
		return nil
	}
	out := make(map[string][]error, len(syncFailed))
	for id, es := range syncFailed {
		out[id] = slices.Clone(es)
	}
	return out
}

// noteSyncFailure keeps a sync's failure in syncFailed and says it, once
// per distinct error of an agent's: a round that fails the same way says
// nothing new, a new one does.
func noteSyncFailure(id string, err error) {
	syncFailedMu.Lock()
	for _, e := range syncFailed[id] {
		if e.Error() == err.Error() {
			syncFailedMu.Unlock()
			return
		}
	}
	if syncFailed == nil {
		syncFailed = map[string][]error{}
	}
	syncFailed[id] = append(syncFailed[id], err)
	syncFailedMu.Unlock()
	log.Printf("agent sync: %s: %v", id, err)
}

// clearSyncFailure drops the failures of a sync that wrote now.
func clearSyncFailure(id string) {
	syncFailedMu.Lock()
	defer syncFailedMu.Unlock()
	delete(syncFailed, id)
	if len(syncFailed) == 0 {
		syncFailed = nil
	}
}

// resetSyncFailures is a test's seam back to nothing kept, as the package's
// other seams (kiroCLIDB, downloadClaudeCode) are.
func resetSyncFailures() {
	syncFailedMu.Lock()
	defer syncFailedMu.Unlock()
	syncFailed = nil
}

// syncFaultFor, when set (a test's seam, as kiroCLIDB and downloadClaudeCode
// are), fails the sync of the agent named in a round of SyncCatalog, so the
// round's recording of that failure is what a test reads back, on every
// platform.
var syncFaultFor func(agentID string) error

// SyncCatalog brings every model list magpie wrote into an agent's files up
// to the catalog as it is now. It is what catalog.Changed is set to, so a
// provider saved or removed, or a vendor's list fetched anew, reaches the
// agents; one already running asks for another round rather than waiting.
// Errors are left for the picker: a file magpie can't write now is one the
// next change, or a pick, writes.
func SyncCatalog() {
	syncing.Lock()
	if syncing.running {
		syncing.again = true
		syncing.Unlock()
		return
	}
	syncing.running = true
	syncing.Unlock()
	for {
		// the catalog built once for every agent's lists, not for each
		// look-up of each (thousands at magpie's start, with 30 providers)
		release := provider.Hold()
		// and each agent's files looked at once a round, not each time
		// one is asked about (#1665: settings.json hundreds of times a
		// round); a write is made from what is on the disk (edit.Read)
		unhold := filememo.Hold()
		renaming := setToFoundGroups()
		for _, a := range All() {
			// a provider named magpie written by hand is the user's, its
			// model list too (handWired)
			if a.Sync != nil && !a.handWired() && (!a.ListOnly || a.Wired()) {
				// the sync's errors are the picker's: a file that can't be
				// written now is one the next change, or a pick, writes. They
				// are not dropped: a failure is kept and said once (noteSyncFailure),
				// so a model list that never reached the agent shows in the log,
				// and a sync that wrote again drops it (clearSyncFailure)
				var err error
				if f := syncFaultFor; f != nil {
					err = f(a.ID)
				} else {
					err = synced(a.Sync)
				}
				if err != nil {
					noteSyncFailure(a.ID, err)
				} else {
					clearSyncFailure(a.ID)
				}
			}
			// a field magpie set to a found group's id before (autoref.go)
			if renaming[a.ID] {
				a.renameFoundGroups()
			}
		}
		unhold()
		release()
		syncing.Lock()
		if !syncing.again {
			syncing.running = false
			syncing.Unlock()
			return
		}
		syncing.again = false
		syncing.Unlock()
	}
}

// soon counts the SyncCatalogSoon rounds not done yet. Not a WaitGroup:
// a save may start one while a command's end waits for them.
var soon struct {
	sync.Mutex
	n    int
	idle *sync.Cond
}

func init() { soon.idle = sync.NewCond(&soon.Mutex) }

// SyncCatalogSoon is SyncCatalog behind the caller, which is what
// catalog.Changed is set to: a group or a provider saved answers at once,
// and the agents' files follow a moment later. The sync builds every
// agent's lists and writes the ones that differ. On Windows a file another
// program holds open refuses the rename over it, and steady.Rename waits
// half a second on each such file before giving up; in WSL each file is
// read and written through wsl.exe and \\wsl.localhost. A save that waited
// on it took seconds now and then on Windows, while an agent was at work
// (CyberCaius on Discord: 路由组和供应商编辑保存有时候保存会卡很久). A sync
// already running takes the change in its next round, as SyncCatalog does.
func SyncCatalogSoon() {
	soon.Lock()
	soon.n++
	soon.Unlock()
	go func() {
		defer func() {
			soon.Lock()
			if soon.n--; soon.n == 0 {
				soon.idle.Broadcast()
			}
			soon.Unlock()
		}()
		SyncCatalog()
	}()
}

// SyncCatalogSettled waits for every SyncCatalogSoon round started so far:
// a command that saved something ends with the agents' files written.
func SyncCatalogSettled() {
	soon.Lock()
	for soon.n > 0 {
		soon.idle.Wait()
	}
	soon.Unlock()
}

// syncJSON rewrites the value at key of a JSON file, if the file has one
// there and it differs: an agent that watches its files isn't told of a
// change that isn't one.
func syncJSON(path, key string, value func() any) error {
	cur, ok := edit.GetJSON(path, key)
	if !ok {
		return nil
	}
	v := value()
	if sameJSON(cur, v) {
		return nil
	}
	return edit.SetJSON(path, edit.KV{Path: key, Value: v})
}

// theirsKept is value, magpie's provider block for the agent, with what
// else the block at key of the JSON file holds kept: a key magpie doesn't
// write there, at any depth, is the user's or another tool's (Pi's
// providers.magpie.compat.sendSessionAffinityHeaders, which
// pi-cache-optimizer adds, #1103) and stays. The keys named in whole (the
// model list) are magpie's alone, so a model gone from the catalog leaves
// the agent's list.
func theirsKept(path, key string, value func() any, whole ...string) func() any {
	return func() any {
		v := value()
		cur, ok := edit.GetJSON(path, key)
		if !ok {
			return v
		}
		var have, want map[string]json.RawMessage
		b, err := json.Marshal(v)
		if err != nil || json.Unmarshal([]byte(cur), &have) != nil || have == nil || json.Unmarshal(b, &want) != nil || want == nil {
			return v
		}
		return mergeTheirs(have, want, whole)
	}
}

// mergeTheirs is want with each key of have that want lacks, objects in
// both merged alike; a key named in whole is want's as it is.
func mergeTheirs(have, want map[string]json.RawMessage, whole []string) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(have)+len(want))
	for k, w := range want {
		out[k] = w
	}
	for k, h := range have {
		w, ok := want[k]
		if !ok {
			out[k] = h
			continue
		}
		if slices.Contains(whole, k) {
			continue
		}
		var hm, wm map[string]json.RawMessage
		if json.Unmarshal(h, &hm) != nil || hm == nil || json.Unmarshal(w, &wm) != nil || wm == nil {
			continue
		}
		if b, err := json.Marshal(mergeTheirs(hm, wm, nil)); err == nil {
			out[k] = b
		}
	}
	return out
}

// syncJSONInOrder is syncJSON for a block whose keys' order the agent reads
// (OpenCode lists a model's variants in theirs): one that says the same in
// another order is rewritten too, as an older magpie wrote the variants
// alphabetically (#713).
func syncJSONInOrder(path, key string, value func() any) error {
	cur, ok := edit.GetJSON(path, key)
	if !ok {
		return nil
	}
	v := value()
	if sameJSON(cur, v) && sameOrder(cur, v) {
		return nil
	}
	return edit.SetJSON(path, edit.KV{Path: key, Value: v})
}

// sameOrder reports whether raw JSON and what v marshals to read alike token
// by token, keys in the same order; whitespace and escapes aside.
func sameOrder(raw string, v any) bool {
	b, err := json.Marshal(v)
	if err != nil {
		return false
	}
	x, y := json.NewDecoder(strings.NewReader(raw)), json.NewDecoder(bytes.NewReader(b))
	x.UseNumber()
	y.UseNumber()
	for {
		a, errA := x.Token()
		c, errC := y.Token()
		if errA != nil || errC != nil {
			return errA == io.EOF && errC == io.EOF
		}
		if a != c {
			return false
		}
	}
}

// sameJSON reports whether raw JSON says what v marshals to.
func sameJSON(raw string, v any) bool {
	b, err := json.Marshal(v)
	if err != nil {
		return false
	}
	var x, y any
	if json.Unmarshal([]byte(raw), &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

// yamlKept is value laid over the mapping at key in a YAML file, as
// theirsKept does for JSON: a key magpie doesn't write stays as the user, or
// the app itself, left it, and mappings in both are merged alike. The keys
// named in own are magpie's: its value, or none when it writes none (#2030's
// siblings: Hermes writes default_model into magpie's entry itself, and a
// whole rewrite dropped it).
func yamlKept(path, key string, value any, own ...string) any {
	raw, err := edit.Read(path)
	if err != nil || raw == nil {
		return value
	}
	var doc any
	if yaml.Unmarshal(raw, &doc) != nil {
		return value
	}
	for _, k := range strings.Split(key, ".") {
		m, _ := doc.(map[string]any)
		if doc = m[k]; doc == nil {
			return value
		}
	}
	have, ok := doc.(map[string]any)
	if !ok {
		return value
	}
	var want map[string]any
	b, err := yaml.Marshal(value)
	if err != nil || yaml.Unmarshal(b, &want) != nil || want == nil {
		return value
	}
	return mergeKept(have, want, own)
}

// mergeKept is want with each key of have that want lacks, mappings in both
// merged alike; a key named in own is want's, or gone. have and want are
// decoded JSON or YAML.
func mergeKept(have, want map[string]any, own []string) map[string]any {
	out := make(map[string]any, len(have)+len(want))
	for k, w := range want {
		out[k] = w
	}
	for k, h := range have {
		if slices.Contains(own, k) {
			// magpie's key for the gateway, where the user put a key of
			// the gateway's own instead: theirs stays (keyHolds)
			if hs, ok := h.(string); ok {
				if ws, ok := want[k].(string); ok && hs != ws && keyHolds(hs, ws) {
					out[k] = hs
				}
			}
			continue
		}
		w, ok := want[k]
		if !ok {
			out[k] = h
			continue
		}
		hm, hok := h.(map[string]any)
		wm, wok := w.(map[string]any)
		if hok && wok {
			out[k] = mergeKept(hm, wm, nil)
		}
	}
	return out
}

// syncYAML is syncJSON for a YAML file.
func syncYAML(path, key string, value func() any) error {
	raw, err := edit.Read(path)
	if err != nil || raw == nil {
		return nil
	}
	var doc any
	if yaml.Unmarshal(raw, &doc) != nil {
		return nil
	}
	for _, k := range strings.Split(key, ".") {
		m, _ := doc.(map[string]any)
		if doc = m[k]; doc == nil {
			return nil
		}
	}
	v := value()
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	var want any
	if yaml.Unmarshal(b, &want) == nil && reflect.DeepEqual(doc, want) {
		return nil
	}
	return edit.SetYAML(path, edit.KV{Path: key, Value: v})
}
