package gateway

import (
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// L-02: the gateway's access lines — every call logged as it finishes, where
// only failures were — are Settings' Log every call (debugLog), read as each
// call ends so turning it on or off needs no restart; MAGPIE_DEBUG still
// turns them on as before. A failed call is logged either way.
func TestDebugLogSettingLogsEveryCall(t *testing.T) {
	fresh(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("MAGPIE_DEBUG", "")
	l := catchLog(t)
	setup(t, provider.Chat, &fake{t: t, reply: sse(
		`data: {"id":"c1","model":"m1","choices":[{"delta":{"role":"assistant","content":"hi back"}}]}`,
		`data: {"id":"c1","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`,
		`data: [DONE]`)})

	// off, and no env: a successful call is not logged
	if code, body := post(t, "/v1/chat/completions", `{"model":"m1","messages":[{"role":"user","content":"hi"}],"stream":false}`); code != 200 || body == "" {
		t.Fatalf("the call did not answer: %d %.200s", code, body)
	}
	for _, line := range strings.Split(l.String(), "\n") {
		if strings.Contains(line, "m1 ") && strings.Contains(line, "200") {
			t.Fatalf("an access line with the setting off: %s", line)
		}
	}

	// on: the same call's access line is there
	if err := settings.Save(settings.Settings{DebugLog: true}); err != nil {
		t.Fatal(err)
	}
	if code, _ := post(t, "/v1/chat/completions", `{"model":"m1","messages":[{"role":"user","content":"hi"}],"stream":false}`); code != 200 {
		t.Fatal("the call did not answer again")
	}
	waitLogged(t, l, "m1", "200")

	// off again, with no restart: the line is gone again
	if err := settings.Save(settings.Settings{}); err != nil {
		t.Fatal(err)
	}
	before := l.String()
	if code, _ := post(t, "/v1/chat/completions", `{"model":"m1","messages":[{"role":"user","content":"hi"}],"stream":false}`); code != 200 {
		t.Fatal("the third call did not answer")
	}
	for _, line := range strings.Split(strings.TrimPrefix(l.String(), before), "\n") {
		if strings.Contains(line, "m1 ") && strings.Contains(line, "200") {
			t.Fatalf("an access line after the setting went off: %s", line)
		}
	}
}
