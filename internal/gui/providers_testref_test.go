package gui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// POST /api/provider/test with ref asks with the one key it names — by its
// fingerprint or the name given it — alone, on or off, where a test without
// one goes out by the first on that can serve, as ever. A ref of no key of
// the provider's is refused before any request goes out.
func TestProviderTestRef(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	var mu sync.Mutex
	var auths []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// the provider's list, which a save or a test may read
			_, _ = io.WriteString(w, `{"data":[{"id":"gpt-x"}]}`)
			return
		}
		mu.Lock()
		auths = append(auths, r.Header.Get("Authorization"))
		mu.Unlock()
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"hi"}}]}`)
	}))
	defer up.Close()
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	post := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(body)))
		return w
	}
	test := func(body string) (int, []provider.Result, string) {
		t.Helper()
		w := post("/api/provider/test", body)
		var got struct {
			Results []provider.Result
			Error   string
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("%d %s: %v", w.Code, w.Body, err)
		}
		return w.Code, got.Results, got.Error
	}
	authed := func() string {
		t.Helper()
		mu.Lock()
		defer mu.Unlock()
		defer func() { auths = nil }()
		if len(auths) != 1 {
			t.Fatalf("the vendor was asked %d times, want one", len(auths))
		}
		return auths[0]
	}
	if w := post("/api/provider/save", `{"id":"relay","name":"Relay","key":"sk-one","chat":"`+up.URL+`/v1","models":["gpt-x"],"new":true}`); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := post("/api/keys/add", `{"id":"relay","name":"Team","key":"sk-two"}`); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	second := provider.KeyID("sk-two")
	// the ref names the second key: the test goes out with it, not with the
	// first, which without the ref is the one asked with
	if code, rs, _ := test(`{"id":"relay","test":["gpt-x"],"ref":"` + second + `"}`); code != 200 || len(rs) != 1 || !rs[0].OK {
		t.Fatalf("%d %+v", code, rs)
	}
	if got := authed(); got != "Bearer sk-two" {
		t.Fatalf("the test by fingerprint went out as %q, want sk-two", got)
	}
	// by the name given the key, the same
	if code, rs, _ := test(`{"id":"relay","test":["gpt-x"],"ref":"Team"}`); code != 200 || len(rs) != 1 || !rs[0].OK {
		t.Fatalf("%d %+v", code, rs)
	}
	if got := authed(); got != "Bearer sk-two" {
		t.Fatalf("the test by name went out as %q, want sk-two", got)
	}
	// no ref, as the Tests of the models field send: the first key, as ever
	if code, rs, _ := test(`{"id":"relay","test":["gpt-x"]}`); code != 200 || len(rs) != 1 || !rs[0].OK {
		t.Fatalf("%d %+v", code, rs)
	}
	if got := authed(); got != "Bearer sk-one" {
		t.Fatalf("the test without a ref went out as %q, want sk-one", got)
	}
	// a ref of no key of the provider's is said so, and nothing is asked
	code, _, errd := test(`{"id":"relay","test":["gpt-x"],"ref":"no-such"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("%d, want 400", code)
	}
	if !strings.Contains(errd, "no such key") {
		t.Fatalf("the refusal said %q, want no such key", errd)
	}
	if len(auths) != 0 {
		t.Fatalf("the vendor was asked %d times, want none", len(auths))
	}
}

// A key that's off can be tested by its ref too: the probe goes out with it
// alone, and it stays off — testing turns nothing on.
func TestProviderTestRefOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	var mu sync.Mutex
	var auths []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"data":[{"id":"gpt-x"}]}`)
			return
		}
		mu.Lock()
		auths = append(auths, r.Header.Get("Authorization"))
		mu.Unlock()
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"hi"}}]}`)
	}))
	defer up.Close()
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	post := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(body)))
		return w
	}
	if w := post("/api/provider/save", `{"id":"relay","name":"Relay","key":"sk-one","chat":"`+up.URL+`/v1","models":["gpt-x"],"new":true}`); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := post("/api/keys/add", `{"id":"relay","key":"sk-two"}`); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := post("/api/keys/off", `{"id":"relay","ref":"`+provider.KeyID("sk-two")+`"}`); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w := post("/api/provider/test", `{"id":"relay","test":["gpt-x"],"ref":"`+provider.KeyID("sk-two")+`"}`)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var got struct{ Results []provider.Result }
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 1 || !got.Results[0].OK {
		t.Fatalf("results %+v", got.Results)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(auths) != 1 || auths[0] != "Bearer sk-two" {
		t.Fatalf("the off key was asked as %v, want one ask as sk-two", auths)
	}
	p, err := provider.Find("relay")
	if err != nil {
		t.Fatal(err)
	}
	if list := p.KeyList(); len(list) != 2 || list[1].On {
		t.Fatalf("the key list %+v, want the second still off", list)
	}
}
