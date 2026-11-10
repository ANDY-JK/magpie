package gui

import (
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// POST /api/provider/test with caps asks the provider for each model's
// capabilities too, and the answer carries them: without the flag nothing
// more is asked than the model's own test.
func TestProviderCaps(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	var mu sync.Mutex
	var asks []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// the provider's list, which a save or a test may read
			_, _ = io.WriteString(w, `{"data":[{"id":"gpt-x"}]}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		mu.Lock()
		asks = append(asks, body)
		mu.Unlock()
		switch {
		case strings.Contains(body, "image_url"):
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"white"}}]}`)
		case strings.Contains(body, `"tools"`):
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"magpie_probe","arguments":"{}"}}]}}]}`)
		case strings.Contains(body, `"stream":true`):
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		default:
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"hi"}}]}`)
		}
	}))
	defer up.Close()
	mux := http.NewServeMux()
	providerRoutes(mux, nil)
	post := func(action, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/provider/"+action, strings.NewReader(body)))
		return w
	}
	if w := post("save", `{"id":"relay","name":"Relay","key":"sk-x","chat":"`+up.URL+`/v1","models":["gpt-x"],"new":true}`); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	test := func(body string) []provider.Result {
		t.Helper()
		w := post("test", body)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		var got struct{ Results []provider.Result }
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got.Results
	}
	asked := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(asks)
	}
	rs := test(`{"id":"relay","test":["gpt-x"],"caps":true}`)
	if len(rs) != 1 || !rs[0].OK {
		t.Fatalf("results %+v", rs)
	}
	want := map[string]string{"tools": "yes", "vision": "yes", "stream": "yes"}
	if !maps.Equal(rs[0].Caps, want) {
		t.Fatalf("caps %v, want %v", rs[0].Caps, want)
	}
	if n := asked(); n != 4 {
		t.Fatalf("upstream was asked %d times, want the test and three probes", n)
	}
	mu.Lock()
	asks = nil
	mu.Unlock()
	rs = test(`{"id":"relay","test":["gpt-x"]}`)
	if len(rs) != 1 || !rs[0].OK || rs[0].Caps != nil {
		t.Fatalf("results without caps %+v", rs)
	}
	if n := asked(); n != 1 {
		t.Fatalf("upstream was asked %d times without caps, want the test alone", n)
	}
}
