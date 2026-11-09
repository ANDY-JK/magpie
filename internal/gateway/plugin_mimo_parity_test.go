package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/plugin"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/testenv"
)

// MiMo's plugin serves the retired built-in ("mimo-app") the way its
// account asked at the gateway: on the chat API alone (the built-in's
// mimoProvider gave Chat and nothing else), from an account that only
// streams (the built-in's Account.Stream), with the Token Plan's credits
// for its usage. The mover is migrate_mimo.go.

// The MiMo subscription, moved onto its plugin or served by it from the
// Plugins page, answers as the built-in did: a streamed chat request is
// relayed as the client sent it, a whole one is asked streamed and given
// back gathered, a Codex turn goes out on the chat API the plugin serves,
// and the reply's usage is the request's row, named for the subscription.
func TestMovedMiMoServedByItsPlugin(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	var asked []map[string]any
	pid := besideFake(t, provider.MiMoID, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var q map[string]any
		json.Unmarshal(b, &q)
		mu.Lock()
		paths, asked = append(paths, r.URL.Path), append(asked, q)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, sse(
			`data: {"id":"c","object":"chat.completion.chunk","model":"fake-1","choices":[{"index":0,"delta":{"role":"assistant","content":"pong"}}]}`,
			`data: {"id":"c","object":"chat.completion.chunk","model":"fake-1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":3,"total_tokens":12}}`,
			`data: [DONE]`))
	}))
	// the plugin is "mimo-app", not mimo-app-plugin beside it: the built-in
	// is retired (provider.BuiltinRetired)
	if pid != provider.MiMoID {
		t.Fatalf("the MiMo plugin stands as %q", pid)
	}
	ask := func(path, body string) (int, string) {
		rec := httptest.NewRecorder()
		New().Handler().ServeHTTP(rec, httptest.NewRequest("POST", path, strings.NewReader(body)))
		return rec.Code, rec.Body.String()
	}
	code, body := ask("/v1/chat/completions", `{"model":"`+pid+`/fake-1","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	if code != 200 || !strings.Contains(body, "pong") {
		t.Fatalf("streamed chat: %d %s", code, body)
	}
	code, body = ask("/v1/chat/completions", `{"model":"`+pid+`/fake-1","messages":[{"role":"user","content":"hi"}]}`)
	if code != 200 || !strings.Contains(body, `"pong"`) || !strings.Contains(body, `"finish_reason":"stop"`) {
		t.Fatalf("whole chat: %d %s", code, body)
	}
	code, body = ask("/v1/responses", `{"model":"`+pid+`/fake-1","input":"hi"}`)
	if code != 200 || !strings.Contains(body, "pong") {
		t.Fatalf("responses: %d %s", code, body)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 3 || strings.Join(paths, ",") != strings.Repeat("/v1/chat/completions,", 2)+"/v1/chat/completions" {
		t.Fatalf("the plugin was asked at %v", paths)
	}
	if q := asked[0]; q["model"] != "fake-1" || q["stream"] != true {
		t.Fatalf("the streamed chat went as %v", q)
	}
	if q := asked[1]; q["stream"] != true {
		// the account only streams: a whole request is asked streamed
		t.Fatalf("the whole chat went as %v", q)
	}
	if q := asked[2]; q["model"] != "fake-1" || q["stream"] != true || q["messages"] == nil {
		t.Fatalf("Codex's turn went as %v", q)
	}
	if u := lastUsage(t); u.Provider != provider.MiMoID || u.Input != 9 || u.Output != 3 {
		t.Fatalf("recorded %+v, want %s's 9 and 3", u, provider.MiMoID)
	}
}

// The moved subscription's Token Plan reads as the plugin makes it: the
// window routing weighs (a share, not the text a display alone would give,
// plugins#3650) and the credits it counts by the day, the compact display
// read as millions (TestCreditWindowShapes' MiMo case), read here through
// the plugin host.
func TestMovedMiMoPlanCountsItsCredits(t *testing.T) {
	bun := testenv.Bun(t)
	fresh(t)
	t.Setenv("MAGPIE_BUN", bun)
	t.Setenv("FAKE_ID", provider.MiMoID)
	windows := filepath.Join(t.TempDir(), "windows.json")
	t.Setenv("FAKE_WINDOWS", windows)
	t.Cleanup(plugin.Settle)
	testenv.FakeVendor(t, http.NotFoundHandler())
	old := allowances
	allowances = provider.Allowances
	t.Cleanup(func() { allowances = old })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	abs, _ := filepath.Abs("../plugin/testdata/fake/index.js")
	if _, err := plugin.Add(ctx, abs); err != nil {
		t.Fatal(err)
	}
	if _, err := plugin.Providers(ctx); err != nil {
		t.Fatal(err)
	}
	st, err := provider.StartPluginSignIn(provider.MiMoID, 1, map[string]string{"where": "home"})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.SubmitSignInCallback(st.ID, "good"); err != nil {
		t.Fatal(err)
	}
	if _, err := plugin.Providers(ctx); err != nil {
		t.Fatal(err)
	}
	if p, err := provider.Find(provider.MiMoID); err != nil || !p.IsPlugin() {
		t.Fatalf("Find(%s) = %+v, %v", provider.MiMoID, p, err)
	}
	// the window as the plugin makes it: a share and a compact display
	// alone (credits_daily_shapes_test.go's MiMo case)
	write := func(used float64) {
		t.Helper()
		w := []map[string]any{{"name": "Token Plan", "used": used, "display": jsonOf(used) + "M / 100M credits"}}
		raw, _ := json.Marshal(map[string]any{"me@fake": w})
		if err := os.WriteFile(windows+".tmp", raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(windows+".tmp", windows); err != nil {
			t.Fatal(err)
		}
	}
	agent := "plugin:" + provider.MiMoID
	write(12.3)
	var plan provider.Allowance
	deadline := time.Now().Add(time.Minute)
	for {
		provider.StaleAllowance(agent, "me@fake")
		got := provider.Allowances(agent)
		_, failed, ok := provider.UsageRead(agent, "me@fake")
		if ok && !failed && len(got["me@fake"]) == 1 {
			plan = got["me@fake"]
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("routing never read the plan (failed %v): %+v", failed, got)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if plan[0].Name != "Token Plan" || plan[0].Used != 12.3 {
		t.Fatalf("routing read %+v, want the Token Plan at 12.3", plan)
	}
	// the plan counts as credits: the first read stands, the second's drop
	// is the day's, in credits the display's millions name
	provider.RefreshUsage(ctx, provider.MiMoID, "me@fake")
	write(20.5)
	deadline = time.Now().Add(time.Minute)
	for {
		provider.RefreshUsage(ctx, provider.MiMoID, "me@fake")
		for _, q := range provider.SubscriptionUsage(ctx) {
			if q.Provider != provider.MiMoID || len(q.Windows) != 1 || q.Windows[0].Name != "Token Plan" {
				continue
			}
			d := q.Daily
			if d == nil || d.Window != "Token Plan" || d.Unit != "" || len(d.Days) != 1 || d.Days[0].Used != 8.2e6 {
				t.Fatalf("the Token Plan's credits %+v (window %+v), want a day of 8.2M credits", d, q.Windows[0])
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no MiMo card with its credits read again")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
