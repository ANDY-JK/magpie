package gui

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/plugin"
)

// A plugin install on a bun this Windows or processor can't run answers
// with a code and the reason, which the page says in the reader's
// language (zgcwkj on Discord), wrapped however deep the install put it.
func TestFailSaysBunUnstartable(t *testing.T) {
	bun := `C:\magpie\cache\bun\1.4.2\magpie-bun.exe`
	ue := &plugin.BunUnstartableError{Bun: bun, Why: "windows", Err: fmt.Errorf("exit status 0xc0000139")}
	rw := httptest.NewRecorder()
	fail(rw, fmt.Errorf("bun add @magpie-community/opencode-raccoon-auth@latest: %w", ue))
	var out map[string]string
	if err := json.Unmarshal(rw.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if rw.Code != 400 || out["code"] != "bun_unstartable" || out["bunWhy"] != "windows" || out["bun"] != bun || out["error"] == "" || out["said"] != bunSaid["windows"] {
		t.Fatalf("fail answered %d %v", rw.Code, out)
	}
	rw = httptest.NewRecorder()
	fail(rw, fmt.Errorf("bun add x: exit status 1"))
	out = nil
	if err := json.Unmarshal(rw.Body.Bytes(), &out); err != nil || out["code"] != "" || out["said"] != "" {
		t.Fatalf("an ordinary failure answered %v (%v)", out, err)
	}
}

// What the page says for it is a key its language files have in every
// language: a t() of a word it lacks shows the English.
func TestBunSaidIsTranslated(t *testing.T) {
	for why, said := range bunSaid {
		key, _ := json.Marshal(said)
		for _, lang := range pageLanguages {
			b, err := os.ReadFile("assets/i18n." + lang + ".js")
			if err != nil {
				t.Fatal(err)
			}
			if n := strings.Count(string(b), string(key)+":"); n != 1 {
				t.Errorf("%s: %s has %q %d times, want once", why, lang, said, n)
			}
		}
	}
}
