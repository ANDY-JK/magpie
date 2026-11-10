package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// A model's test can say what it can do besides answer (TestModelsWith):
// whether it calls a tool, reads a picture and streams its answer. Each is
// one more small request to the endpoint the model's test went to, with
// the same URL, headers and key, so a capability is the endpoint's answer
// and nothing magpie configured. Only the endpoints a test goes to
// directly are asked: a sign-in reached through its agent's API, a
// translated one and a decision model have no endpoint of their own to ask
// (ModelTest). A capability the answer didn't settle has no key at all, as
// a refusal for auth or a rate limit says nothing about the model.

// capProbeTool is the tool a capability probe requires the model to call.
const capProbeTool = "magpie_probe"

// capToolCalls are what the four APIs write a tool call as: OpenAI's
// tool_calls, Anthropic's tool_use, and Responses' and Gemini's
// function_call.
var capToolCalls = [][]byte{[]byte("tool_calls"), []byte("tool_use"), []byte("function_call"), []byte("functionCall")}

// capPNG is a 1×1 transparent PNG: the smallest picture a vision probe can
// send.
const capPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4//8/AwAI/AL+p5qgoAAAAABJRU5ErkJggg=="

// capRead is how much of a capability probe's answer is read: a few KB,
// enough for its first events or its error, never a whole stream.
const capRead = 32 << 10

// withCaps is r with what model can do, when r answered on proto's
// endpoint and caps was asked. A test that failed has nothing to probe:
// the endpoint is not answering.
func (p Provider) withCaps(ctx context.Context, r Result, proto Protocol, upstream, client string, caps bool) Result {
	if !caps || !r.OK {
		return r
	}
	r.Caps = p.capsOn(ctx, proto, upstream, client)
	return r
}

// capsOn is what the model answered on proto's endpoint can do, each "yes"
// or "no" where the endpoint answered about it and no key where it was
// refused for another reason. Each probe follows the model's own test, one
// after the other, so a slow endpoint spends at most three more waits.
func (p Provider) capsOn(ctx context.Context, proto Protocol, upstream, client string) map[string]string {
	out := map[string]string{}
	for _, c := range []struct {
		name  string
		probe func(context.Context, Protocol, string, string) string
	}{
		{"tools", p.capTools},
		{"vision", p.capVision},
		{"stream", p.capStream},
	} {
		if proto == Gemini && c.name == "stream" {
			// Google answers generateContent whole; a streamed answer is
			// another method (streamGenerateContent), not this endpoint's
			continue
		}
		if said := c.probe(ctx, proto, upstream, client); said != "" {
			out[c.name] = said
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// capBody is url and body for one capability probe of model on proto's
// endpoint: the smallest request tinyBody sends for it, changed by change,
// then in the shape the backend takes (tinyShape), asked as client asks
// when one is named (TestAs).
func capBody(q Provider, proto Protocol, upstream, client string, change func(map[string]any)) (string, []byte) {
	url, raw := tinyBody(q, proto, upstream)
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return url, []byte(raw)
	}
	change(m)
	b, err := json.Marshal(m)
	if err != nil {
		return url, []byte(raw)
	}
	url, shaped := tinyShape(q, proto, upstream, url, string(b))
	if client != "" {
		shaped = asClient(client, proto, shaped)
	}
	return url, []byte(shaped)
}

// capAnswer sends one capability probe as any probe goes out, through
// probeHeaders, and reports its status with the beginning of its answer.
// stop ends the read once the answer holds what the caller looks for, so
// an open stream is never read to its end; a 2xx answer with no stop is
// not read at all, while an error's words are read whole (they come as one
// small body). A request that couldn't be made or signed is no answer.
func capAnswer(ctx context.Context, p Provider, proto Protocol, url string, body []byte, stop func([]byte) bool) (status int, said []byte) {
	ctx, cancel := context.WithTimeout(ctx, testWait)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil
	}
	if err := probeHeaders(ctx, p, proto, url, req, body); err != nil {
		return 0, nil
	}
	res, err := p.Do(http.DefaultClient, req)
	if err != nil {
		return 0, nil
	}
	defer res.Body.Close()
	status = res.StatusCode
	if status < 200 || status >= 300 {
		said, _ = io.ReadAll(io.LimitReader(res.Body, capRead))
		return status, said
	}
	if stop == nil {
		return status, nil
	}
	said = make([]byte, 0, 4096)
	chunk := make([]byte, 4096)
	for len(said) < capRead {
		n, err := res.Body.Read(chunk)
		said = append(said, chunk[:n]...)
		if n == 0 && err == nil {
			// no progress: a body that answers nothing is not waited on
			break
		}
		if stop(said) || err != nil {
			break
		}
	}
	return status, said
}

// capTools asks model to call one tool, which it must, and says "yes" when
// its answer has the call: a model without tool use answers in words
// instead, which is "no". A refusal of the request itself (400, 422) — what
// a vendor that takes no tool_choice sends — is "no" too; any other
// refusal says nothing about the model.
func (p Provider) capTools(ctx context.Context, proto Protocol, upstream, client string) string {
	if p.OpenCodeFree(upstream) {
		// OpenCode's free tier is asked with its own tools in place of any
		// offered (zenFreeProbe): whether it takes tools isn't this probe's
		// to answer, so it says nothing
		return ""
	}
	url, body := capBody(p, proto, upstream, client, func(m map[string]any) {
		schema := map[string]any{"type": "object", "properties": map[string]any{}}
		const about = "A probe. Call it with no arguments."
		capTokens(m, 64)
		switch proto {
		case Chat:
			m["tools"] = []any{map[string]any{"type": "function", "function": map[string]any{"name": capProbeTool, "description": about, "parameters": schema}}}
			m["tool_choice"] = "required"
		case Responses:
			m["tools"] = []any{map[string]any{"type": "function", "name": capProbeTool, "description": about, "parameters": schema}}
			m["tool_choice"] = "required"
		case Anthropic:
			m["tools"] = []any{map[string]any{"name": capProbeTool, "description": about, "input_schema": schema}}
			m["tool_choice"] = map[string]any{"type": "any"}
		case Gemini:
			m["tools"] = []any{map[string]any{"functionDeclarations": []any{map[string]any{"name": capProbeTool, "description": about, "parameters": schema}}}}
			m["toolConfig"] = map[string]any{"functionCallingConfig": map[string]any{"mode": "ANY"}}
		}
	})
	ctx, b := p.tested(ctx, string(body))
	status, said := capAnswer(ctx, p, proto, url, b, func(b []byte) bool { return capHas(b, capToolCalls) })
	if status >= 200 && status < 300 {
		if capHas(said, capToolCalls) {
			return "yes"
		}
		return "no"
	}
	if status == http.StatusBadRequest || status == http.StatusUnprocessableEntity {
		return "no"
	}
	return ""
}

// capVision asks model what color a 1×1 picture is: a 2xx is "yes", and a
// 400 or 422 that names the picture is "no" — the vendor turning an image
// away for what it is — while a refusal for another reason says nothing.
func (p Provider) capVision(ctx context.Context, proto Protocol, upstream, client string) string {
	const ask = "What color is this image?"
	const data = "data:image/png;base64," + capPNG
	url, body := capBody(p, proto, upstream, client, func(m map[string]any) {
		// a word's room, or a model that thinks first spends it before the
		// picture is looked at
		capTokens(m, 64)
		switch proto {
		case Chat:
			m["messages"] = []any{map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": ask},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": data}},
			}}}
		case Responses:
			m["input"] = []any{map[string]any{"type": "message", "role": "user", "content": []any{
				map[string]any{"type": "input_text", "text": ask},
				map[string]any{"type": "input_image", "image_url": data},
			}}}
		case Anthropic:
			m["messages"] = []any{map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": ask},
				map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": capPNG}},
			}}}
		case Gemini:
			m["contents"] = []any{map[string]any{"role": "user", "parts": []any{
				map[string]any{"text": ask},
				map[string]any{"inline_data": map[string]any{"mime_type": "image/png", "data": capPNG}},
			}}}
		}
	})
	ctx, b := p.tested(ctx, string(body))
	status, said := capAnswer(ctx, p, proto, url, b, nil)
	switch {
	case status >= 200 && status < 300:
		return "yes"
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		if capNamesImage(said) {
			return "no"
		}
	}
	return ""
}

// capNamesImage says a refusal names the picture it was sent (image,
// vision, media, picture): a model that doesn't read pictures is turned
// away for that, where one turned away for another reason leaves vision
// unknown. "Content" alone is too common in refusals that aren't about a
// picture to be counted.
func capNamesImage(b []byte) bool {
	b = bytes.ToLower(b)
	for _, word := range []string{"image", "vision", "media", "picture"} {
		if bytes.Contains(b, []byte(word)) {
			return true
		}
	}
	return false
}

// capTokens gives m room for n tokens, whatever its API calls the field: a
// probe's answer is a tool call or a word, and a model that thinks first
// would spend a request's 16 before it got there.
func capTokens(m map[string]any, n int) {
	for _, k := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
		if _, ok := m[k]; ok {
			m[k] = n
		}
	}
	if gc, ok := m["generationConfig"].(map[string]any); ok {
		gc["maxOutputTokens"] = n
	}
}

// capStream asks model to stream its answer: a 2xx whose events come as
// server-sent events is "yes", a 200 answered whole is "no", and a request
// the vendor turns away (400, 422) — one it takes no stream for — is "no"
// too. Gemini is never asked (capsOn): it streams at another method.
func (p Provider) capStream(ctx context.Context, proto Protocol, upstream, client string) string {
	url, body := capBody(p, proto, upstream, client, func(m map[string]any) { m["stream"] = true })
	ctx, b := p.tested(ctx, string(body))
	isEvent := func(b []byte) bool { return bytes.Contains(b, []byte("data:")) || bytes.Contains(b, []byte("event:")) }
	status, said := capAnswer(ctx, p, proto, url, b, isEvent)
	if status >= 200 && status < 300 {
		if isEvent(said) {
			return "yes"
		}
		return "no"
	}
	if status == http.StatusBadRequest || status == http.StatusUnprocessableEntity {
		return "no"
	}
	return ""
}

// capHas says b holds one of marks, as a vendor writes a tool call.
func capHas(b []byte, marks [][]byte) bool {
	for _, m := range marks {
		if bytes.Contains(b, m) {
			return true
		}
	}
	return false
}
