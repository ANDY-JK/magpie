package provider

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// capVendor is a fake vendor that answers a model's test and its
// capability probes, and records what it was asked: the decoded body, its
// raw bytes and the headers each request went with.
type capVendor struct {
	mu    sync.Mutex
	asks  []map[string]any
	raw   []string
	heads []http.Header
	srv   *httptest.Server
}

func newCapVendor(t *testing.T, answer func(w http.ResponseWriter, r *http.Request, in map[string]any, raw string)) *capVendor {
	t.Helper()
	v := &capVendor{}
	v.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var in map[string]any
		_ = json.Unmarshal(raw, &in)
		v.mu.Lock()
		v.asks = append(v.asks, in)
		v.raw = append(v.raw, string(raw))
		v.heads = append(v.heads, r.Header.Clone())
		v.mu.Unlock()
		answer(w, r, in, string(raw))
	}))
	t.Cleanup(v.srv.Close)
	return v
}

func (v *capVendor) count() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.asks)
}

func (v *capVendor) asked() (asks []map[string]any, raw []string, heads []http.Header) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.asks, v.raw, v.heads
}

// capsOf is the result's capabilities as the map they should be: a test
// says the exact set, not only that it holds a key.
func capsOf(t *testing.T, r Result, want map[string]string) {
	t.Helper()
	if !maps.Equal(r.Caps, want) {
		t.Fatalf("caps %v, want %v (result %+v)", r.Caps, want, r)
	}
}

// capAsked is the first request a vendor was asked whose raw body holds
// rawNeedle, nil when none did.
func capAsked(v *capVendor, rawNeedle string) map[string]any {
	asks, raw, _ := v.asked()
	for i, r := range raw {
		if strings.Contains(r, rawNeedle) {
			return asks[i]
		}
	}
	return nil
}

func capMapOf(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func capListOf(v any) []any {
	l, _ := v.([]any)
	return l
}

// chatCaps answers a Chat model's test and its probes as a vendor that
// does all three: a tool call for the tool probe, words for the picture,
// and server-sent events for the stream.
func chatCaps(w http.ResponseWriter, in map[string]any, raw string) {
	switch {
	case strings.Contains(raw, "image_url"):
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"white"}}]}`)
	case in["tools"] != nil:
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"magpie_probe","arguments":"{}"}}]}}]}`)
	case in["stream"] == true:
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
	default:
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"hi"}}]}`)
	}
}

// A Chat model that calls tools, reads a picture and streams says all
// three: the three probes follow its test, each the smallest request that
// asks one thing, with the same URL and key.
func TestCapsChat(t *testing.T) {
	oneHome(t)
	v := newCapVendor(t, func(w http.ResponseWriter, _ *http.Request, in map[string]any, raw string) {
		chatCaps(w, in, raw)
	})
	p := Provider{ID: "relay", Name: "Relay", Chat: v.srv.URL + "/v1", Key: "sk-x"}
	rs := p.TestModelsWith(context.Background(), []string{"gpt-x"}, true)
	if len(rs) != 1 || !rs[0].OK {
		t.Fatalf("results %+v", rs)
	}
	capsOf(t, rs[0], map[string]string{"tools": "yes", "vision": "yes", "stream": "yes"})
	if n := v.count(); n != 4 {
		t.Fatalf("asked %d requests, want the test and three probes", n)
	}
	_, _, heads := v.asked()
	for _, h := range heads {
		if h.Get("Authorization") != "Bearer sk-x" {
			t.Fatalf("a probe went out as %q", h.Get("Authorization"))
		}
	}
	tools := capAsked(v, `"tools"`)
	if tools == nil || tools["tool_choice"] != "required" || tools["max_tokens"] != float64(64) {
		t.Fatalf("tool probe %v", tools)
	}
	fn := capMapOf(capListOf(tools["tools"])[0])["function"]
	if name := capMapOf(fn)["name"]; name != capProbeTool {
		t.Fatalf("tool probe offers %v", name)
	}
	_, raws, _ := v.asked()
	sent := false
	for _, r := range raws {
		sent = sent || strings.Contains(r, "data:image/png;base64,")
	}
	if !sent {
		t.Fatal("the vision probe sent no picture")
	}
}

// A model asked for a tool that answers in words has no tool use: the
// answer is "no", not a failed probe.
func TestCapsChatToolsNo(t *testing.T) {
	oneHome(t)
	v := newCapVendor(t, func(w http.ResponseWriter, _ *http.Request, in map[string]any, raw string) {
		switch {
		case in["tools"] != nil:
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"hi"}}]}`)
		default:
			chatCaps(w, in, raw)
		}
	})
	p := Provider{ID: "relay", Name: "Relay", Chat: v.srv.URL + "/v1", Key: "sk-x"}
	rs := p.TestModelsWith(context.Background(), []string{"gpt-x"}, true)
	if len(rs) != 1 || !rs[0].OK {
		t.Fatalf("results %+v", rs)
	}
	capsOf(t, rs[0], map[string]string{"tools": "no", "vision": "yes", "stream": "yes"})
}

// A 400 that names the picture is the vendor saying the model reads none:
// vision is "no".
func TestCapsVisionNo(t *testing.T) {
	oneHome(t)
	v := newCapVendor(t, func(w http.ResponseWriter, _ *http.Request, in map[string]any, raw string) {
		if strings.Contains(raw, "image_url") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"This model does not support image input","type":"invalid_request_error"}}`)
			return
		}
		chatCaps(w, in, raw)
	})
	p := Provider{ID: "relay", Name: "Relay", Chat: v.srv.URL + "/v1", Key: "sk-x"}
	rs := p.TestModelsWith(context.Background(), []string{"gpt-x"}, true)
	if len(rs) != 1 || !rs[0].OK {
		t.Fatalf("results %+v", rs)
	}
	capsOf(t, rs[0], map[string]string{"tools": "yes", "vision": "no", "stream": "yes"})
}

// A 200 answered whole, with no events in it, is a stream the vendor
// doesn't stream: "no".
func TestCapsStreamNo(t *testing.T) {
	oneHome(t)
	v := newCapVendor(t, func(w http.ResponseWriter, _ *http.Request, in map[string]any, raw string) {
		if in["stream"] == true {
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"hi"}}]}`)
			return
		}
		chatCaps(w, in, raw)
	})
	p := Provider{ID: "relay", Name: "Relay", Chat: v.srv.URL + "/v1", Key: "sk-x"}
	rs := p.TestModelsWith(context.Background(), []string{"gpt-x"}, true)
	if len(rs) != 1 || !rs[0].OK {
		t.Fatalf("results %+v", rs)
	}
	capsOf(t, rs[0], map[string]string{"tools": "yes", "vision": "yes", "stream": "no"})
}

// A rate limit is no answer about the model: each refused probe leaves its
// capability out rather than saying the model can't.
func TestCapsRefused(t *testing.T) {
	oneHome(t)
	v := newCapVendor(t, func(w http.ResponseWriter, _ *http.Request, in map[string]any, raw string) {
		if in["tools"] != nil || strings.Contains(raw, "image_url") || in["stream"] == true {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"message":"rate limited"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"hi"}}]}`)
	})
	p := Provider{ID: "relay", Name: "Relay", Chat: v.srv.URL + "/v1", Key: "sk-x"}
	rs := p.TestModelsWith(context.Background(), []string{"gpt-x"}, true)
	if len(rs) != 1 || !rs[0].OK {
		t.Fatalf("results %+v", rs)
	}
	if len(rs[0].Caps) != 0 {
		t.Fatalf("caps %v, want none", rs[0].Caps)
	}
	if n := v.count(); n != 4 {
		t.Fatalf("asked %d requests, want the test and three probes", n)
	}
}

// A model whose own test failed has nothing to probe: caps=true asks no
// capability at all.
func TestCapsNotAskedWhenTheTestFails(t *testing.T) {
	oneHome(t)
	v := newCapVendor(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any, _ string) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"message":"upstream is down"}}`)
	})
	p := Provider{ID: "relay", Name: "Relay", Chat: v.srv.URL + "/v1", Key: "sk-x"}
	rs := p.TestModelsWith(context.Background(), []string{"gpt-x"}, true)
	if len(rs) != 1 || rs[0].OK || rs[0].Caps != nil {
		t.Fatalf("results %+v", rs)
	}
	if n := v.count(); n != 1 {
		t.Fatalf("asked %d requests, want the test alone", n)
	}
}

// caps=false is TestModels: the model is asked once and nothing else.
func TestCapsNotAskedWhenOff(t *testing.T) {
	oneHome(t)
	v := newCapVendor(t, func(w http.ResponseWriter, _ *http.Request, in map[string]any, raw string) {
		chatCaps(w, in, raw)
	})
	p := Provider{ID: "relay", Name: "Relay", Chat: v.srv.URL + "/v1", Key: "sk-x"}
	rs := p.TestModelsWith(context.Background(), []string{"gpt-x"}, false)
	if len(rs) != 1 || !rs[0].OK || rs[0].Caps != nil {
		t.Fatalf("results %+v", rs)
	}
	if n := v.count(); n != 1 {
		t.Fatalf("asked %d requests, want the test alone", n)
	}
}

// Anthropic's Messages API is asked for a tool it must call (tool_choice
// any), and the tool_use in its answer is "yes".
func TestCapsAnthropicTools(t *testing.T) {
	oneHome(t)
	v := newCapVendor(t, func(w http.ResponseWriter, r *http.Request, in map[string]any, raw string) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		switch {
		case in["tools"] != nil:
			_, _ = io.WriteString(w, `{"type":"message","content":[{"type":"tool_use","id":"t1","name":"magpie_probe","input":{}}]}`)
		case strings.Contains(raw, `"type":"image"`):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"image input is not supported"}}`)
		case in["stream"] == true:
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
		default:
			_, _ = io.WriteString(w, `{"type":"message","content":[{"type":"text","text":"hi"}]}`)
		}
	})
	p := Provider{ID: "claude-relay", Name: "Relay", Anthropic: v.srv.URL, Key: "sk-a"}
	rs := p.TestModelsWith(context.Background(), []string{"claude-x"}, true)
	if len(rs) != 1 || !rs[0].OK {
		t.Fatalf("results %+v", rs)
	}
	capsOf(t, rs[0], map[string]string{"tools": "yes", "vision": "no", "stream": "yes"})
	_, _, heads := v.asked()
	for _, h := range heads {
		if h.Get("x-api-key") != "sk-a" || h.Get("anthropic-version") != "2023-06-01" {
			t.Fatalf("a probe went out with %v", h)
		}
	}
	tools := capAsked(v, `"tools"`)
	if tools == nil {
		t.Fatal("no tool probe")
	}
	if kind := capMapOf(tools["tool_choice"])["type"]; kind != "any" {
		t.Fatalf("tool probe asks %v", tools["tool_choice"])
	}
	name := capMapOf(capListOf(tools["tools"])[0])["name"]
	if name != capProbeTool {
		t.Fatalf("tool probe offers %v", name)
	}
}

// Responses' API is asked for a tool it must call, and the function_call in
// its answer is "yes".
func TestCapsResponsesTools(t *testing.T) {
	oneHome(t)
	v := newCapVendor(t, func(w http.ResponseWriter, r *http.Request, in map[string]any, raw string) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		switch {
		case in["tools"] != nil:
			_, _ = io.WriteString(w, `{"output":[{"type":"function_call","name":"magpie_probe","arguments":"{}"}]}`)
		case strings.Contains(raw, "input_image"):
			_, _ = io.WriteString(w, `{"output":[{"type":"message","content":[{"type":"output_text","text":"white"}]}]}`)
		case in["stream"] == true:
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n")
		default:
			_, _ = io.WriteString(w, `{"output":[{"type":"message"}]}`)
		}
	})
	p := Provider{ID: "relay", Name: "Relay", Responses: v.srv.URL + "/v1", Key: "sk-r"}
	rs := p.TestModelsWith(context.Background(), []string{"gpt-x"}, true)
	if len(rs) != 1 || !rs[0].OK {
		t.Fatalf("results %+v", rs)
	}
	if rs[0].Caps["tools"] != "yes" {
		t.Fatalf("caps %v, want tools yes", rs[0].Caps)
	}
	tools := capAsked(v, `"tools"`)
	if tools == nil || tools["tool_choice"] != "required" {
		t.Fatalf("tool probe %v", tools)
	}
	if name := capMapOf(capListOf(tools["tools"])[0])["name"]; name != capProbeTool {
		t.Fatalf("tool probe offers %v", name)
	}
}

// Gemini's generateContent is asked for a function call it must make
// (toolConfig ANY), and the functionCall in its answer is "yes". Its
// stream is never probed: Gemini streams at another method.
func TestCapsGeminiTools(t *testing.T) {
	oneHome(t)
	v := newCapVendor(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any, raw string) {
		if !strings.Contains(r.URL.Path, ":generateContent") {
			http.NotFound(w, r)
			return
		}
		switch {
		case strings.Contains(raw, "functionDeclarations"):
			_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"magpie_probe","args":{}}}]}}]}`)
		case strings.Contains(raw, "inline_data"):
			_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"white"}]}}]}`)
		default:
			_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}`)
		}
	})
	p := Provider{ID: "gem", Name: "Gem", Gemini: v.srv.URL + "/v1beta", Key: "sk-g"}
	rs := p.TestModelsWith(context.Background(), []string{"gemini-x"}, true)
	if len(rs) != 1 || !rs[0].OK {
		t.Fatalf("results %+v", rs)
	}
	capsOf(t, rs[0], map[string]string{"tools": "yes", "vision": "yes"})
	if n := v.count(); n != 3 {
		t.Fatalf("asked %d requests, want the test and two probes", n)
	}
	_, _, heads := v.asked()
	for _, h := range heads {
		if h.Get("x-goog-api-key") != "sk-g" {
			t.Fatalf("a probe went out with %v", h)
		}
	}
	tools := capAsked(v, "functionDeclarations")
	if tools == nil {
		t.Fatal("no tool probe")
	}
	mode := capMapOf(capMapOf(tools["toolConfig"])["functionCallingConfig"])["mode"]
	if mode != "ANY" {
		t.Fatalf("tool probe asks %v", tools["toolConfig"])
	}
	name := capMapOf(capListOf(capMapOf(capListOf(tools["tools"])[0])["functionDeclarations"])[0])["name"]
	if name != capProbeTool {
		t.Fatalf("tool probe offers %v", name)
	}
}
