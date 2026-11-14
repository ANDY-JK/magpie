package gateway

// A reply the vendor stopped at its output limit — Anthropic's stop_reason
// "max_tokens", the OpenAI family's (DeepSeek's, Kimi's) finish_reason
// "length", a Responses reply's incomplete "max_output_tokens" — ends the
// reply where the limit stopped it, as it always has, unless the user
// turned settings.AutoContinue on: it is then asked to go on from what it
// has said, at most autoContinues times, in each protocol's prefill shape
// (continuation.go), the tries' usage summed into the reply's. A streamed
// client reads the whole of it as one reply, and so does one handed whole.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// limitUpstream is an upstream that answers call n with answers[n]'s events,
// the stream whole each time: the vendor stopped at its output limit ends
// its reply itself, no cut. Paths journals what path each call was asked
// on, bodies what each was asked with.
type limitUpstream struct {
	mu      sync.Mutex
	bodies  []map[string]any
	paths   []string
	answers [][]string // each call's events, the stream whole
	tail    string     // after the events: a Chat stream's [DONE]
}

func (f *limitUpstream) serve(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	var body map[string]any
	b, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(b, &body); err != nil {
		t.Errorf("upstream request isn't JSON: %s", b)
	}
	f.mu.Lock()
	call := len(f.bodies)
	f.bodies = append(f.bodies, body)
	f.paths = append(f.paths, r.URL.Path)
	var events []string
	if call < len(f.answers) {
		events = f.answers[call]
	}
	tail := f.tail
	f.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	for _, ev := range events {
		io.WriteString(w, "data: "+ev+"\n\n")
	}
	io.WriteString(w, tail)
}

func (f *limitUpstream) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bodies)
}

// path is the path call was asked on.
func (f *limitUpstream) path(call int) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.paths[call]
}

// lastAssistant is the last message a Chat call was asked with, when it's
// the assistant's: the reply so far, sent back to go on from.
func (f *limitUpstream) lastAssistant(call int) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	msgs, _ := f.bodies[call]["messages"].([]any)
	last, _ := msgs[len(msgs)-1].(map[string]any)
	if last["role"] != "assistant" {
		return nil
	}
	return last
}

// lastAssistantText is the text of the last message an Anthropic call was
// asked with, when it's the assistant's.
func (f *limitUpstream) lastAssistantText(call int) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	msgs, _ := f.bodies[call]["messages"].([]any)
	last, _ := msgs[len(msgs)-1].(map[string]any)
	if last["role"] != "assistant" {
		return "", false
	}
	var sb strings.Builder
	for _, b := range last["content"].([]any) {
		block, _ := b.(map[string]any)
		if s, _ := block["text"].(string); s != "" {
			sb.WriteString(s)
		}
	}
	return sb.String(), true
}

// autoContinueOn turns settings.AutoContinue on for the test: the
// behaviour is the user's own to switch on.
func autoContinueOn(t *testing.T) {
	t.Helper()
	if err := settings.Save(settings.Settings{AutoContinue: true}); err != nil {
		t.Fatal(err)
	}
}

// limitUpstreamServer points a provider whose only API is Chat at f, and
// answers a Messages request for its model, streamed or not: the reply is
// translated.
func limitUpstreamServer(t *testing.T, model string, f *limitUpstream, stream bool) string {
	t.Helper()
	fresh(t)
	autoContinueOn(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { f.serve(t, w, r) }))
	t.Cleanup(up.Close)
	p := provider.Provider{ID: "up", Name: "UP", Key: "k", Models: []string{model}, Chat: up.URL + "/v1"}
	if err := provider.Save(p); err != nil {
		t.Fatal(err)
	}
	return postTranslated(t, model, stream)
}

// limitAnthropicServer points a provider whose only API is Anthropic's at
// f, and answers a Chat request for its model, streamed or not.
func limitAnthropicServer(t *testing.T, model string, f *limitUpstream, stream bool) string {
	t.Helper()
	fresh(t)
	autoContinueOn(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { f.serve(t, w, r) }))
	t.Cleanup(up.Close)
	p := provider.Provider{ID: "up", Name: "UP", Key: "k", Models: []string{model}, Anthropic: up.URL}
	if err := provider.Save(p); err != nil {
		t.Fatal(err)
	}
	if stream {
		code, body := post(t, "/v1/chat/completions", `{"model":"up/`+model+`","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
		if code != 200 {
			t.Fatalf("status %d: %s", code, body)
		}
		return body
	}
	code, body := post(t, "/v1/chat/completions", `{"model":"up/`+model+`","messages":[{"role":"user","content":"hi"}]}`)
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	return body
}

// postTranslated posts a Messages request for up/model, streamed or not.
func postTranslated(t *testing.T, model string, stream bool) string {
	t.Helper()
	body := `{"model":"up/` + model + `","max_tokens":32000,`
	if stream {
		body += `"stream":true,`
	}
	body += `"messages":[{"role":"user","content":"hi"}]}`
	code, body := post(t, "/v1/messages", body)
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	return body
}

// anthLimitStop is the message_delta a reply stopped at its output limit
// ends with, anthWhole its message_stop.
func anthLimitStop(output int) string {
	return fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":%d}}`, output)
}

func anthWhole() string {
	return `{"type":"message_stop"}`
}

// chatLimitStop is the chunk a Chat reply stopped at its output limit ends
// with: finish_reason "length", the usage counted.
func chatLimitStop(model string) string {
	return fmt.Sprintf(`{"id":"c1","model":%q,"choices":[{"index":0,"delta":{},"finish_reason":"length"}],"usage":{"prompt_tokens":9,"completion_tokens":4}}`, model)
}

// stopOf is the stop_reason a Messages stream's message_delta says, wholeStopOf
// one a whole Messages reply says.
func stopOf(body string) string {
	for _, ev := range events(body) {
		if ev["type"] == "message_delta" {
			if d, ok := ev["delta"].(map[string]any); ok {
				s, _ := d["stop_reason"].(string)
				return s
			}
		}
	}
	return ""
}

func wholeStopOf(t *testing.T, body string) string {
	t.Helper()
	var v struct {
		StopReason string `json:"stop_reason"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("whole reply isn't JSON: %s", body)
	}
	return v.StopReason
}

// chatStopOf is the finish_reason a whole Chat reply says, chatContent its
// content, chatWholeUsage its usage.
func chatStopOf(t *testing.T, body string) string {
	t.Helper()
	var v struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("whole reply isn't JSON: %s", body)
	}
	if len(v.Choices) == 0 {
		t.Fatalf("no choices in the reply: %s", body)
	}
	return v.Choices[0].FinishReason
}

func chatContentOf(t *testing.T, body string) string {
	t.Helper()
	var v struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("whole reply isn't JSON: %s", body)
	}
	if len(v.Choices) == 0 {
		t.Fatalf("no choices in the reply: %s", body)
	}
	return v.Choices[0].Message.Content
}

func chatWholeUsage(t *testing.T, body string) map[string]any {
	t.Helper()
	var v struct {
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("whole reply isn't JSON: %s", body)
	}
	return v.Usage
}

// wholeUsageOf is the usage a whole Messages reply counts.
func wholeUsageOf(t *testing.T, body string) map[string]any {
	t.Helper()
	var v struct {
		Usage struct {
			Input  float64 `json:"input_tokens"`
			Output float64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("whole reply isn't JSON: %s", body)
	}
	return map[string]any{"input_tokens": v.Usage.Input, "output_tokens": v.Usage.Output}
}

// chatEndsOf counts a Chat stream's finish_reason chunks and its error
// chunks: one reply says its end once and fails with none.
func chatEndsOf(body string) (finishes, errs int) {
	for _, ev := range events(body) {
		if _, ok := ev["error"]; ok {
			errs++
		}
		for _, c := range choices(ev) {
			if fr, ok := c["finish_reason"]; ok && fr != nil {
				finishes++
			}
		}
	}
	return
}

// chatFinishOf is the finish_reason a Chat stream says its reply ended with.
func chatFinishOf(body string) string {
	fin := ""
	for _, ev := range events(body) {
		for _, c := range choices(ev) {
			if fr, ok := c["finish_reason"]; ok && fr != nil {
				fin, _ = fr.(string)
			}
		}
	}
	return fin
}

// quickPauses leaves the pauses between the asks of a reply nothing, so
// the tests count asks, not seconds.
func quickPauses(t *testing.T) {
	t.Helper()
	old := retryPause
	retryPause = time.Millisecond
	t.Cleanup(func() { retryPause = old })
}

// A reply the Anthropic upstream stopped at its output limit goes on: the
// same conversation is asked again with what the client has as the last,
// assistant message, and the client reads one whole reply, its usage the
// tries' summed.
func TestAutoContinueAnthropicLimitContinues(t *testing.T) {
	quickPauses(t)
	f := &limitUpstream{
		answers: [][]string{
			{anthStart("m", 7), anthText("hel"), anthLimitStop(4), anthWhole()},
			{anthStart("m", 11), anthText("lo"), anthStop(4), anthWhole()},
		},
	}
	body := limitAnthropicServer(t, "m", f, true)
	if f.calls() != 2 {
		t.Fatalf("the reply stopped at the limit wasn't asked to go on: %d calls", f.calls())
	}
	prefill, ok := f.lastAssistantText(1)
	if !ok || prefill != "hel" {
		t.Fatalf("the second ask didn't carry what the client has to go on from: %q, %v", prefill, ok)
	}
	if got := chatTextOf(body); got != "hello" {
		t.Fatalf("client's text = %q, want %q", got, "hello")
	}
	if finishes, errs := chatEndsOf(body); finishes != 1 || errs != 0 {
		t.Fatalf("finish_reasons %d, errors %d, want 1/0: %s", finishes, errs, body)
	}
	if fin := chatFinishOf(body); fin != "stop" {
		t.Fatalf("the reply's finish_reason = %q, want the going-on's stop", fin)
	}
	u := chatUsageOf(body)
	if u == nil || u["prompt_tokens"] != 18.0 || u["completion_tokens"] != 8.0 {
		t.Fatalf("usage = %v, want the tries summed: prompt 7+11=18, output 4+4=8", u)
	}
}

// A reply the OpenAI family stopped at its output limit (finish_reason
// "length") goes on in the vendor's own prefill shape: Kimi's partial mode
// carries what the client has back, and the client reads one whole reply.
func TestAutoContinueKimiLengthContinues(t *testing.T) {
	quickPauses(t)
	f := &limitUpstream{
		tail: "data: [DONE]\n\n",
		answers: [][]string{
			{chatText("kimi-k3", "hel"), chatLimitStop("kimi-k3")},
			{chatText("kimi-k3", "lo"), chatStop("kimi-k3")},
		},
	}
	body := limitUpstreamServer(t, "kimi-k3", f, true)
	if f.calls() != 2 {
		t.Fatalf("calls = %d, want 2", f.calls())
	}
	m := f.lastAssistant(1)
	if m == nil || m["content"] != "hel" || m["partial"] != true {
		t.Fatalf("the second ask didn't carry what the client has, in Kimi's partial mode: %v", m)
	}
	if got := textOf(body); got != "hello" {
		t.Fatalf("client's text = %q, want %q", got, "hello")
	}
	if starts, stops, errs := messageStops(body); starts != 1 || stops != 1 || errs != 0 {
		t.Fatalf("starts %d, stops %d, errors %d, want 1/1/0: %s", starts, stops, errs, body)
	}
}

// DeepSeek goes on from the reply's part at its output limit in its prefix
// mode: the continuation is asked of the /beta endpoint with "prefix": true
// on the message, and the client reads one whole reply.
func TestAutoContinueDeepSeekLengthContinues(t *testing.T) {
	quickPauses(t)
	f := &limitUpstream{
		tail: "data: [DONE]\n\n",
		answers: [][]string{
			{chatText("deepseek-chat", "hel"), chatLimitStop("deepseek-chat")},
			{chatText("deepseek-chat", "lo"), chatStop("deepseek-chat")},
		},
	}
	body := limitUpstreamServer(t, "deepseek-chat", f, true)
	if f.calls() != 2 {
		t.Fatalf("calls = %d, want 2", f.calls())
	}
	if p := f.path(1); p != "/beta/chat/completions" {
		t.Fatalf("the continuation went to %q, want DeepSeek's prefix-mode /beta path", p)
	}
	m := f.lastAssistant(1)
	if m == nil || m["content"] != "hel" || m["prefix"] != true {
		t.Fatalf("the continuation wasn't DeepSeek's prefix mode: %v", m)
	}
	if got := textOf(body); got != "hello" {
		t.Fatalf("client's text = %q, want %q", got, "hello")
	}
}

// A reply stopped at its output limit again and again goes on at most
// autoContinues times, then ends where the limit stopped it, as it always
// has: its stop_reason says so, and no error is said.
func TestAutoContinueLimitOnlySoLong(t *testing.T) {
	quickPauses(t)
	f := &limitUpstream{
		answers: [][]string{
			{anthStart("m", 7), anthText("hel"), anthLimitStop(4), anthWhole()},
			{anthStart("m", 11), anthText(" mid"), anthLimitStop(4), anthWhole()},
			{anthStart("m", 11), anthText(" end"), anthLimitStop(4), anthWhole()},
			{anthStart("m", 11), anthText(" more"), anthLimitStop(4), anthWhole()},
		},
	}
	body := limitAnthropicServer(t, "m", f, true)
	if f.calls() != 1+autoContinues {
		t.Fatalf("calls = %d, want %d (the reply itself and %d continuations)", f.calls(), 1+autoContinues, autoContinues)
	}
	if got := chatTextOf(body); got != "hel mid end" {
		t.Fatalf("client's text = %q, want %q", got, "hel mid end")
	}
	if finishes, errs := chatEndsOf(body); finishes != 1 || errs != 0 {
		t.Fatalf("finish_reasons %d, errors %d, want 1/0: %s", finishes, errs, body)
	}
	if fin := chatFinishOf(body); fin != "length" {
		t.Fatalf("the reply's finish_reason = %q, want length, the limit it ended at", fin)
	}
}

// With AutoContinue off, the default, a reply stopped at its output limit
// ends there, as it always has: nothing is asked again.
func TestAutoContinueOffEndsAtTheLimit(t *testing.T) {
	quickPauses(t)
	fresh(t)
	f := &limitUpstream{
		answers: [][]string{
			{anthStart("m", 7), anthText("hel"), anthLimitStop(4), anthWhole()},
			{anthStart("m", 11), anthText("lo"), anthStop(4), anthWhole()},
		},
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { f.serve(t, w, r) }))
	t.Cleanup(up.Close)
	p := provider.Provider{ID: "up", Name: "UP", Key: "k", Models: []string{"m"}, Anthropic: up.URL}
	if err := provider.Save(p); err != nil {
		t.Fatal(err)
	}
	body := postTranslated(t, "m", true)
	if f.calls() != 1 {
		t.Fatalf("the reply stopped at the limit was asked again with the switch off: %d calls", f.calls())
	}
	if got := textOf(body); got != "hel" {
		t.Fatalf("client's text = %q, want %q", got, "hel")
	}
	if stop := stopOf(body); stop != "max_tokens" {
		t.Fatalf("the reply's stop_reason = %q, want max_tokens", stop)
	}
}

// A reply handed whole that the Anthropic upstream stopped at its output
// limit goes on too: one JSON body, the tries' text joined, their usage
// summed, the going-on's own end said.
func TestAutoContinueWholeAnthropicLimitContinues(t *testing.T) {
	quickPauses(t)
	f := &limitUpstream{
		answers: [][]string{
			{anthStart("m", 7), anthText("hel"), anthLimitStop(4), anthWhole()},
			{anthStart("m", 11), anthText("lo"), anthStop(4), anthWhole()},
		},
	}
	body := limitAnthropicServer(t, "m", f, false)
	if f.calls() != 2 {
		t.Fatalf("calls = %d, want 2", f.calls())
	}
	prefill, ok := f.lastAssistantText(1)
	if !ok || prefill != "hel" {
		t.Fatalf("the second ask didn't carry what the reply has to go on from: %q, %v", prefill, ok)
	}
	if got := chatContentOf(t, body); got != "hello" {
		t.Fatalf("reply's text = %q, want %q", got, "hello")
	}
	if stop := chatStopOf(t, body); stop != "stop" {
		t.Fatalf("reply's finish_reason = %q, want the going-on's stop", stop)
	}
	u := chatWholeUsage(t, body)
	if u["prompt_tokens"] != 18.0 || u["completion_tokens"] != 8.0 {
		t.Fatalf("usage = %v, want the tries summed: prompt 7+11=18, output 4+4=8", u)
	}
}

// A reply handed whole that the OpenAI family stopped at its output limit
// goes on in Kimi's partial mode: one Messages reply, its text joined and
// its usage the tries' summed.
func TestAutoContinueWholeKimiLengthContinues(t *testing.T) {
	quickPauses(t)
	f := &limitUpstream{
		tail: "data: [DONE]\n\n",
		answers: [][]string{
			{chatText("kimi-k3", "hel"), chatLimitStop("kimi-k3")},
			{chatText("kimi-k3", "lo"), chatStop("kimi-k3")},
		},
	}
	body := limitUpstreamServer(t, "kimi-k3", f, false)
	if f.calls() != 2 {
		t.Fatalf("calls = %d, want 2", f.calls())
	}
	m := f.lastAssistant(1)
	if m == nil || m["content"] != "hel" || m["partial"] != true {
		t.Fatalf("the second ask didn't carry what the reply has, in Kimi's partial mode: %v", m)
	}
	if got := wholeTextOf(t, body); got != "hello" {
		t.Fatalf("reply's text = %q, want %q", got, "hello")
	}
	if stop := wholeStopOf(t, body); stop != "end_turn" {
		t.Fatalf("reply's stop_reason = %q, want the going-on's end_turn", stop)
	}
	u := wholeUsageOf(t, body)
	if u["input_tokens"] != 18.0 || u["output_tokens"] != 8.0 {
		t.Fatalf("usage = %v, want the tries summed: prompt 9+9=18, output 4+4=8", u)
	}
}

// wholeTextOf is the text a whole Messages reply says.
func wholeTextOf(t *testing.T, body string) string {
	t.Helper()
	var v struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("whole reply isn't JSON: %s", body)
	}
	var sb strings.Builder
	for _, b := range v.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}

// Each vendor's shape of "stopped at its output limit" is read as the one
// word the going-on goes by: Anthropic's stop_reason "max_tokens", the
// OpenAI family's finish_reason "length", a Responses reply's status
// "incomplete" for reason "max_output_tokens" — and Gemini's finishReason
// "MAX_TOKENS", the same limit in its own words.
func TestAutoContinueLimitShapesRead(t *testing.T) {
	p := provider.Provider{ID: "up", Name: "UP"}
	for _, c := range []struct {
		proto provider.Protocol
		event string
	}{
		{provider.Anthropic, `{"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":4}}`},
		{provider.Chat, `{"choices":[{"index":0,"delta":{},"finish_reason":"length"}]}`},
		{provider.Responses, `{"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":9,"output_tokens":4}}}`},
		{provider.Gemini, `{"candidates":[{"finishReason":"MAX_TOKENS","content":{"parts":[{"text":"hel"}]}}]}`},
	} {
		var stops []string
		dec := decoderFor(p, c.proto, nil)
		if err := dec(c.event, func(ev Event) {
			if ev.Kind == KStop {
				stops = append(stops, ev.Stop)
			}
		}); err != nil {
			t.Errorf("%s: the event didn't read: %v", c.proto, err)
		}
		if len(stops) != 1 || !limitStop(stops[0]) {
			t.Errorf("%s: %q read as %v, want one stop the going-on goes by", c.proto, c.event, stops)
		}
	}
	// a turn's own end, and a filter's, are no limit: they end the reply.
	for _, c := range []struct {
		proto provider.Protocol
		event string
	}{
		{provider.Anthropic, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":4}}`},
		{provider.Chat, `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`},
	} {
		var stops []string
		dec := decoderFor(p, c.proto, nil)
		if err := dec(c.event, func(ev Event) {
			if ev.Kind == KStop {
				stops = append(stops, ev.Stop)
			}
		}); err != nil {
			t.Errorf("%s: the event didn't read: %v", c.proto, err)
		}
		if len(stops) != 1 || limitStop(stops[0]) {
			t.Errorf("%s: %q read as a limit stop: %v", c.proto, c.event, stops)
		}
	}
}
