package gui

// The Doctor page's model lists are read from the desktop apps' own config
// files, so their fixtures here are those files as the apps write them:
// JSON with comments and trailing commas for TeleAgent, the XDG tree for
// DuMate, and the account folder named by the owner file, not the UUID.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func doctorFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("DUMATE_USER_DATA_DIR", "")
	t.Setenv("DUMATE_XDG_DIR", "")
	t.Setenv("TELEAGENT_DATA", "")
	t.Setenv("TELEAGENT_CONFIG", "")

	dm := filepath.Join(home, "AppData", "Roaming", "qianfan-desktop-app")
	mustWrite(t, filepath.Join(dm, "install.json"), `{"dataDir":`+quote(dm)+`,"installDir":`+quote(filepath.Join(home, "Programs", "DuMate"))+`}`)
	mustWrite(t, filepath.Join(dm, "auth.json"), `{"activeProfileId":"uuid-1","accountProfiles":[{"profileId":"uuid-1","bceAccountId":"acct-1","displayName":"Tester"}]}`)
	mustWrite(t, filepath.Join(dm, "qianfan_desk_xdg", "acct-1", "config", "opencode", "opencode.json"), `{
  // the app's own file has comments and a trailing comma
  "provider": {
    "QianfanPersonalQuota": {
      "models": {
        "glm-5": {"id":"model-text","name":"glm-5","limit":{"input":1000000,"output":128000}},
        "model-text": {"id":"model-text","name":"model-text","limit":{"input":1000000,"output":384000}},
        "deepseek-v41-flash": {"id":"deepseek-v41-flash","name":"DeepSeek V4.1 Flash","limit":{"input":1000000,"output":384000}},
        "qwen2.5-vl-32b-instruct": {"id":"qwen2.5-vl-32b-instruct","name":"Qwen2.5 VL"},
        "llm-mock": {"id":"llm-mock","name":"mock"},
        "qwen3-embedding-4b": {"id":"qwen3-embedding-4b","name":"embed"},
        "model-routing": {"id":"model-routing","name":"routing"},
      },
    },
  },
}`)

	ta := filepath.Join(home, ".local", "share", "TeleAgent")
	mustWrite(t, filepath.Join(ta, "current-owner.json"), `{"ownerDir":"v1_public_x","userId":"x"}`)
	mustWrite(t, filepath.Join(home, ".config", "TeleAgent", "users", "v1_public_x", "TeleAgent.jsonc"), `{
  "provider": {
    "NewApi": {
      "models": {
        "chat-pro": {"id":"chat-pro","name":"智能","limit":{"input":400000,"output":65536}},
        "chat-flagship": {"id":"chat-flagship","name":"旗舰","limit":{"input":400000,"output":65536}},
        "chat-flash": {"id":"chat-flash","name":"极速","limit":{"input":400000,"output":65536}},
        "chat-lite": {"id":"chat-lite","name":"轻量","limit":{"input":100000,"output":16384}},
        "chat-nano": {"id":"chat-nano","name":"轻速","limit":{"input":100000,"output":16384}},
        "tele-safety-answer": {"id":"tele-safety-answer","name":"tele-safety-answer"},
      }
    }
  }
}`)
	return home
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func catalogByID(t *testing.T, cats []doctorCatalog, app string) doctorCatalog {
	t.Helper()
	for _, c := range cats {
		if c.App == app {
			return c
		}
	}
	t.Fatalf("no catalog for %s in %d", app, len(cats))
	return doctorCatalog{}
}

func TestDoctorModelCatalogs(t *testing.T) {
	doctorFixture(t)
	cats := localModelCatalogs()
	if len(cats) != 2 {
		t.Fatalf("catalogs %d, want DuMate and TeleAgent: %+v", len(cats), cats)
	}

	dm := catalogByID(t, cats, "DuMate")
	if dm.Provider != "dumate" || dm.Count != 3 {
		t.Fatalf("DuMate %+v", dm)
	}
	ids := doctorModelIDs(dm.Models)
	if !contains(ids, "model-text") || !contains(ids, "deepseek-v41-flash") || contains(ids, "glm-5") {
		t.Fatalf("DuMate ids %v: the alias goes, the API id stays", ids)
	}
	if contains(ids, "llm-mock") || contains(ids, "qwen3-embedding-4b") || contains(ids, "model-routing") {
		t.Fatalf("DuMate ids %v: internal entries stay out", ids)
	}
	if m := doctorModelOf(dm.Models, "model-text"); m.Output != 384000 {
		t.Fatalf("model-text %+v: the canonical entry's own limits win over the alias's", m)
	}

	ta := catalogByID(t, cats, "TeleAgent")
	if ta.Provider != "teleagent" || ta.Count != 5 {
		t.Fatalf("TeleAgent %+v", ta)
	}
	if contains(doctorModelIDs(ta.Models), "tele-safety-answer") {
		t.Fatalf("TeleAgent ids %v: the safety model stays out", doctorModelIDs(ta.Models))
	}
}

func TestDoctorModelCatalogsRoute(t *testing.T) {
	doctorFixture(t)
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/doctor/model-catalogs", nil))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var got struct {
		Catalogs []struct {
			App    string `json:"app"`
			Count  int    `json:"count"`
			Source string `json:"source"`
			Models []struct {
				ID string `json:"id"`
			} `json:"models"`
		} `json:"catalogs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Catalogs) != 2 {
		t.Fatalf("catalogs %d", len(got.Catalogs))
	}
	for _, c := range got.Catalogs {
		if c.Source == "" || c.Count != len(c.Models) {
			t.Fatalf("%s: source %q count %d models %d", c.App, c.Source, c.Count, len(c.Models))
		}
	}
}

// An app that isn't installed is left out, not an error row: the page shows
// only what this computer has.
func TestDoctorModelCatalogsAbsentApp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("DUMATE_USER_DATA_DIR", "")
	t.Setenv("DUMATE_XDG_DIR", "")
	t.Setenv("TELEAGENT_DATA", "")
	t.Setenv("TELEAGENT_CONFIG", "")
	if cats := localModelCatalogs(); len(cats) != 0 {
		t.Fatalf("catalogs %d on an empty machine", len(cats))
	}
}

func doctorModelIDs(ms []doctorModel) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}

func doctorModelOf(ms []doctorModel, id string) doctorModel {
	for _, m := range ms {
		if m.ID == id {
			return m
		}
	}
	return doctorModel{}
}
