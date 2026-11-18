package gui

// The Doctor page's model lists: the real catalogs the desktop apps on this
// computer keep, read from their own config files, so a provider whose
// plugin lists only part of them can be checked against the app's list.
//
// Read-only: it finds and parses, and changes no provider or file. The
// sources are the apps magpie's plugins serve and whose configs name their
// models:
//
//	DuMate    <dataDir>/qianfan_desk_xdg/<account>/config/opencode/opencode.json
//	TeleAgent <configDir>/users/<scope>/TeleAgent.jsonc
//
// Each is optional: a missing app is left out, and one that can't be read
// says why. Nothing here opens a login or a network connection.

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// doctorModel is one model as an app's config names it.
type doctorModel struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Context int    `json:"context,omitempty"`
	Output  int    `json:"output,omitempty"`
}

// doctorCatalog is one app's model list and where it was read.
type doctorCatalog struct {
	App      string        `json:"app"`
	Provider string        `json:"provider"`
	Source   string        `json:"source,omitempty"`
	Count    int           `json:"count"`
	Models   []doctorModel `json:"models"`
	Error    string        `json:"error,omitempty"`
}

// doctorModelRoutes serves the model lists the Doctor page shows.
func doctorModelRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/doctor/model-catalogs", func(rw http.ResponseWriter, r *http.Request) {
		writeJSON(rw, map[string]any{"catalogs": localModelCatalogs()})
	})
}

// localModelCatalogs reads each known app's list. A failed read is that
// catalog's error, never the whole list's: an app that is not installed
// is simply absent.
func localModelCatalogs() []doctorCatalog {
	out := []doctorCatalog{}
	for _, find := range []func() doctorCatalog{dumateCatalog, teleagentCatalog} {
		if c := find(); c.App != "" && c.Source != "" {
			out = append(out, c)
		}
	}
	return out
}

// ---- DuMate ---------------------------------------------------------------

var dumateSkip = map[string]bool{
	"llm-mock": true, "qianfan-ocr": true, "qwen3-embedding-4b": true,
	"qwen3-reranker-4b": true, "dumate-image0.1": true,
	"model-aiassist": true, "model-artifact-validate": true,
	"model-coding-routing": true, "model-followup": true, "model-gui-use": true,
	"model-meeting-notes": true, "model-mm-fallback": true,
	"model-ppt-validate": true, "model-reason-summary": true,
	"model-routing": true, "model-routing2": true, "model-tm-deepseek": true,
	"model-tm-qwen-small": true, "model-wiki": true,
}

var doctorSkipRe = regexp.MustCompile(`(?:^|[-_])(?:mock|ocr|embedding|reranker|rerank|validate|safety)(?:$|[-_])`)

func doctorSkipped(key string) bool {
	return dumateSkip[strings.ToLower(key)] || doctorSkipRe.MatchString(strings.ToLower(key))
}

func dumateCatalog() doctorCatalog {
	c := doctorCatalog{App: "DuMate", Provider: "dumate"}
	src := dumateSource()
	if src == "" {
		c.Error = "no local DuMate config found"
		return c
	}
	c.Source = src
	doc, err := readJSONC(src)
	if err != nil {
		c.Error = err.Error()
		return c
	}
	models, err := modelsInProvider(doc, "QianfanPersonalQuota")
	if err != nil {
		c.Error = err.Error()
		return c
	}
	c.Models, c.Count = models, len(models)
	return c
}

// dumateDataDirs is the app's userData folder, by an explicit override, the
// installer's dataDir, or the OS default, with the desktop-app/ child some
// builds use.
func dumateDataDirs() []string {
	home, _ := os.UserHomeDir()
	bases := []string{}
	if v := strings.TrimSpace(os.Getenv("DUMATE_USER_DATA_DIR")); v != "" {
		bases = append(bases, v)
	} else {
		switch runtime.GOOS {
		case "windows":
			bases = append(bases, filepath.Join(os.Getenv("APPDATA"), "qianfan-desktop-app"))
		case "darwin":
			bases = append(bases, filepath.Join(home, "Library", "Application Support", "qianfan-desktop-app"))
		default:
			bases = append(bases, filepath.Join(home, ".config", "qianfan-desktop-app"))
		}
		for _, b := range append([]string(nil), bases...) {
			for _, f := range []string{"install.json", "dumate-install.json"} {
				var j struct {
					DataDir string `json:"dataDir"`
				}
				if b, err := os.ReadFile(filepath.Join(b, f)); err == nil && json.Unmarshal(b, &j) == nil && j.DataDir != "" {
					bases = append(bases, j.DataDir)
				}
			}
		}
	}
	out := []string{}
	for _, b := range bases {
		for _, d := range []string{b, filepath.Join(b, "desktop-app")} {
			if d != "" && !contains(out, d) {
				out = append(out, d)
			}
		}
	}
	return out
}

// dumateSource finds the account's opencode.json. The active account's
// folder wins; otherwise any account's is taken.
func dumateSource() string {
	var roots []string
	if v := strings.TrimSpace(os.Getenv("DUMATE_XDG_DIR")); v != "" {
		roots = append(roots, v)
	}
	for _, d := range dumateDataDirs() {
		roots = append(roots, filepath.Join(d, "qianfan_desk_xdg"))
	}
	accounts := dumateAccounts()
	cands := []string{}
	for _, root := range roots {
		for _, a := range accounts {
			cands = append(cands, filepath.Join(root, a, "config", "opencode", "opencode.json"))
		}
		cands = append(cands, filepath.Join(root, "global", "config", "opencode", "opencode.json"))
		if ents, err := os.ReadDir(root); err == nil {
			for _, e := range ents {
				if e.IsDir() && e.Name() != "data" && e.Name() != "global" && e.Name() != "cache" {
					cands = append(cands, filepath.Join(root, e.Name(), "config", "opencode", "opencode.json"))
				}
			}
		}
	}
	for _, p := range cands {
		if st, err := os.Stat(p); err == nil && st.Size() > 0 && st.Size() < 4<<20 {
			return p
		}
	}
	return ""
}

// dumateAccounts is the active profile's folder names: the XDG tree uses
// the bceAccountId, while auth.json names the profile by a UUID.
func dumateAccounts() []string {
	out := []string{}
	for _, d := range dumateDataDirs() {
		b, err := os.ReadFile(filepath.Join(d, "auth.json"))
		if err != nil {
			continue
		}
		var auth struct {
			ActiveProfileID string `json:"activeProfileId"`
			Profiles        []struct {
				ProfileID    string `json:"profileId"`
				BCEAccountID string `json:"bceAccountId"`
				UserID       string `json:"userId"`
			} `json:"accountProfiles"`
		}
		if json.Unmarshal(b, &auth) != nil {
			continue
		}
		var p *struct {
			ProfileID    string `json:"profileId"`
			BCEAccountID string `json:"bceAccountId"`
			UserID       string `json:"userId"`
		}
		for i := range auth.Profiles {
			if auth.Profiles[i].ProfileID == auth.ActiveProfileID {
				p = &auth.Profiles[i]
				break
			}
		}
		if p == nil && len(auth.Profiles) > 0 {
			p = &auth.Profiles[0]
		}
		if p == nil {
			continue
		}
		for _, v := range []string{p.BCEAccountID, p.ProfileID, auth.ActiveProfileID} {
			if v != "" && !contains(out, v) {
				out = append(out, v)
			}
		}
	}
	return out
}

// ---- TeleAgent ------------------------------------------------------------

var teleagentSkip = map[string]bool{"tele-safety-answer": true}

func teleagentCatalog() doctorCatalog {
	c := doctorCatalog{App: "TeleAgent", Provider: "teleagent"}
	src := teleagentSource()
	if src == "" {
		c.Error = "no local TeleAgent config found"
		return c
	}
	c.Source = src
	doc, err := readJSONC(src)
	if err != nil {
		c.Error = err.Error()
		return c
	}
	models, err := modelsInProvider(doc, "NewApi")
	if err != nil {
		c.Error = err.Error()
		return c
	}
	models = filterModels(models, teleagentSkip)
	c.Models, c.Count = models, len(models)
	return c
}

// teleagentSource finds the current owner's config. The app keeps its data
// in ~/.local/share/TeleAgent and its config in ~/.config/TeleAgent, and
// names the current owner in current-owner.json.
func teleagentSource() string {
	home, _ := os.UserHomeDir()
	data := strings.TrimSpace(os.Getenv("TELEAGENT_DATA"))
	if data == "" {
		data = filepath.Join(home, ".local", "share", "TeleAgent")
	}
	config := strings.TrimSpace(os.Getenv("TELEAGENT_CONFIG"))
	if config == "" {
		config = filepath.Join(home, ".config", "TeleAgent")
	}
	owners := []string{}
	var owner struct {
		OwnerDir string `json:"ownerDir"`
		Scope    string `json:"scope"`
	}
	if b, err := os.ReadFile(filepath.Join(data, "current-owner.json")); err == nil && json.Unmarshal(b, &owner) == nil {
		for _, v := range []string{owner.OwnerDir, owner.Scope} {
			if v != "" && !contains(owners, v) {
				owners = append(owners, v)
			}
		}
	}
	cands := []string{}
	for _, o := range owners {
		cands = append(cands, filepath.Join(config, "users", o, "TeleAgent.jsonc"))
	}
	if ents, err := os.ReadDir(filepath.Join(config, "users")); err == nil {
		for _, e := range ents {
			if e.IsDir() {
				cands = append(cands, filepath.Join(config, "users", e.Name(), "TeleAgent.jsonc"))
			}
		}
	}
	for _, p := range cands {
		if st, err := os.Stat(p); err == nil && st.Size() > 0 && st.Size() < 4<<20 {
			return p
		}
	}
	return ""
}

// ---- Parsing --------------------------------------------------------------

func readJSONC(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(stripJSONC(string(b))), &doc); err != nil {
		return nil, errors.New("its config doesn't parse")
	}
	return doc, nil
}

// stripJSONC removes // and /* */ comments outside strings, and trailing
// commas, as the apps' own JSON-with-comments files have them.
func stripJSONC(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inStr, esc, line, block := false, false, false, false
	for i := 0; i < len(s); i++ {
		c, n := s[i], byte(0)
		if i+1 < len(s) {
			n = s[i+1]
		}
		if line {
			if c == '\n' {
				line = false
				b.WriteByte(c)
			}
			continue
		}
		if block {
			if c == '*' && n == '/' {
				block = false
				i++
			}
			continue
		}
		if inStr {
			b.WriteByte(c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch {
		case c == '"':
			inStr = true
			b.WriteByte(c)
		case c == '/' && n == '/':
			line = true
			i++
		case c == '/' && n == '*':
			block = true
			i++
		default:
			b.WriteByte(c)
		}
	}
	return regexp.MustCompile(`,\s*([}\]])`).ReplaceAllString(b.String(), "$1")
}

// modelsInProvider reads the models of the first provider named that has
// any, deduped by the API id the request must carry. A key that is only an
// alias (glm-5 -> model-text) doesn't add a second model.
func modelsInProvider(doc map[string]any, names ...string) ([]doctorModel, error) {
	providers, _ := doc["provider"].(map[string]any)
	if providers == nil {
		return nil, errors.New("its config lists no provider")
	}
	var entry map[string]any
	for _, name := range names {
		if p, _ := providers[name].(map[string]any); p != nil {
			entry = p
			break
		}
	}
	if entry == nil {
		for _, v := range providers {
			if p, _ := v.(map[string]any); p != nil {
				if m, _ := p["models"].(map[string]any); m != nil {
					entry = p
					break
				}
			}
		}
	}
	raw, _ := entry["models"].(map[string]any)
	if raw == nil {
		return nil, errors.New("its config lists no models")
	}
	type picked struct {
		m   doctorModel
		key string
	}
	byID := map[string]picked{}
	for key, v := range raw {
		m, _ := v.(map[string]any)
		if m == nil || doctorSkipped(key) {
			continue
		}
		id, _ := m["id"].(string)
		if id == "" {
			id = key
		}
		if id == "" || doctorSkipped(id) {
			continue
		}
		name, _ := m["name"].(string)
		if name == "" {
			name = key
		}
		lim, _ := m["limit"].(map[string]any)
		ctx := intOf(lim["context"])
		if ctx == 0 {
			ctx = intOf(lim["input"])
		}
		got := doctorModel{ID: id, Name: name, Context: ctx, Output: intOf(lim["output"])}
		// the entry whose key IS the API id is the canonical one; an
		// alias (glm-5 -> model-text) only stands in until it comes
		if prev, ok := byID[id]; !ok || (key == id && prev.key != id) {
			byID[id] = picked{got, key}
		}
	}
	if len(byID) == 0 {
		return nil, errors.New("its config lists no chat models")
	}
	out := make([]doctorModel, 0, len(byID))
	for _, p := range byID {
		out = append(out, p.m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func filterModels(ms []doctorModel, skip map[string]bool) []doctorModel {
	out := ms[:0]
	for _, m := range ms {
		if !skip[m.ID] && !doctorSkipRe.MatchString(strings.ToLower(m.ID)) {
			out = append(out, m)
		}
	}
	return out
}

func intOf(v any) int {
	switch n := v.(type) {
	case float64:
		if n > 0 {
			return int(n)
		}
	case int:
		if n > 0 {
			return n
		}
	}
	return 0
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
