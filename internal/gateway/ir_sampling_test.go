package gateway

import (
	"testing"

	"github.com/tidwall/gjson"
)

// A Chat request's sampling fields reach each API as far as that API takes
// them, and no further: Chat keeps seed, the two penalties and logit_bias
// (top_k is no Chat field), Messages takes top_k alone, Responses none of
// the five, and Gemini's generationConfig takes seed, the two penalties and
// topK. The fields the request shares with every API — temperature, top_p,
// the output cap and the stop words — are untouched wherever they are not
// the field in question.
func TestSamplingFieldsOnTranslation(t *testing.T) {
	r, err := parseChat([]byte(`{"model":"m","max_tokens":128,"temperature":0.4,"top_p":0.9,` +
		`"seed":42,"frequency_penalty":0.25,"presence_penalty":0.5,"top_k":7,` +
		`"logit_bias":{"123":-4},"stop":["</end>"],"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if r.Seed == nil || *r.Seed != 42 || r.FrequencyPenalty == nil || *r.FrequencyPenalty != 0.25 ||
		r.PresencePenalty == nil || *r.PresencePenalty != 0.5 || r.TopK == nil || *r.TopK != 7 ||
		r.LogitBias["123"] != -4 {
		t.Fatalf("sampling not read: %+v", r)
	}

	t.Run("chat", func(t *testing.T) {
		b := buildChat(r, "m", "", false)
		if gjson.GetBytes(b, "seed").Int() != 42 || gjson.GetBytes(b, "frequency_penalty").Float() != 0.25 ||
			gjson.GetBytes(b, "presence_penalty").Float() != 0.5 || gjson.GetBytes(b, "logit_bias.123").Float() != -4 {
			t.Errorf("sampling not sent: %s", b)
		}
		if gjson.GetBytes(b, "top_k").Exists() {
			t.Errorf("top_k sent to a Chat upstream: %s", b)
		}
		if gjson.GetBytes(b, "temperature").Float() != 0.4 || gjson.GetBytes(b, "top_p").Float() != 0.9 ||
			gjson.GetBytes(b, "max_tokens").Int() != 128 || gjson.GetBytes(b, "stop.0").String() != "</end>" {
			t.Errorf("other fields changed: %s", b)
		}
	})

	t.Run("anthropic", func(t *testing.T) {
		b := buildAnthropic(r, "claude-sonnet-4-5")
		if gjson.GetBytes(b, "top_k").Int() != 7 {
			t.Errorf("top_k not sent: %s", b)
		}
		for _, k := range []string{"seed", "frequency_penalty", "presence_penalty", "logit_bias"} {
			if gjson.GetBytes(b, k).Exists() {
				t.Errorf("%s sent to a Messages upstream: %s", k, b)
			}
		}
		if gjson.GetBytes(b, "temperature").Float() != 0.4 || gjson.GetBytes(b, "max_tokens").Int() != 128 ||
			gjson.GetBytes(b, "stop_sequences.0").String() != "</end>" {
			t.Errorf("other fields changed: %s", b)
		}
	})

	t.Run("responses", func(t *testing.T) {
		b := buildResponses(r, "gpt-5.5", "api.openai.com", false)
		for _, k := range []string{"seed", "frequency_penalty", "presence_penalty", "top_k", "logit_bias"} {
			if gjson.GetBytes(b, k).Exists() {
				t.Errorf("%s sent to a Responses upstream: %s", k, b)
			}
		}
		if gjson.GetBytes(b, "temperature").Float() != 0.4 || gjson.GetBytes(b, "top_p").Float() != 0.9 ||
			gjson.GetBytes(b, "max_output_tokens").Int() != 128 {
			t.Errorf("other fields changed: %s", b)
		}
	})

	t.Run("gemini", func(t *testing.T) {
		b, err := buildGemini(r, "gemini-2.5-pro")
		if err != nil {
			t.Fatal(err)
		}
		for k, want := range map[string]float64{"topK": 7, "seed": 42, "presencePenalty": 0.5, "frequencyPenalty": 0.25} {
			if got := gjson.GetBytes(b, "generationConfig."+k).Float(); got != want {
				t.Errorf("generationConfig.%s = %v, want %v: %s", k, got, want, b)
			}
		}
		if gjson.GetBytes(b, "generationConfig.logit_bias").Exists() || gjson.GetBytes(b, "generationConfig.logitBias").Exists() {
			t.Errorf("logit_bias sent to a Gemini upstream: %s", b)
		}
		if gjson.GetBytes(b, "generationConfig.maxOutputTokens").Int() != 128 ||
			gjson.GetBytes(b, "generationConfig.temperature").Float() != 0.4 ||
			gjson.GetBytes(b, "generationConfig.topP").Float() != 0.9 ||
			gjson.GetBytes(b, "generationConfig.stopSequences.0").String() != "</end>" {
			t.Errorf("other fields changed: %s", b)
		}
	})
}

// An Anthropic request's top_k is carried by the IR: a Gemini upstream is
// asked for it in generationConfig.topK, and a Chat upstream, which has no
// top_k, is sent everything else and no top_k.
func TestAnthropicTopKOnTranslation(t *testing.T) {
	r, err := parseAnthropic([]byte(`{"model":"claude-sonnet-4-5","max_tokens":64,"top_k":5,"top_p":0.7,` +
		`"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil || r.TopK == nil || *r.TopK != 5 {
		t.Fatalf("top_k not read: %v %+v", err, r)
	}
	b := buildChat(r, "gpt-5.5", "api.openai.com", false)
	if gjson.GetBytes(b, "top_k").Exists() {
		t.Errorf("top_k sent to a Chat upstream: %s", b)
	}
	if gjson.GetBytes(b, "top_p").Float() != 0.7 || gjson.GetBytes(b, "max_completion_tokens").Int() != 64 {
		t.Errorf("other fields changed: %s", b)
	}
	g, err := buildGemini(r, "gemini-2.5-pro")
	if err != nil {
		t.Fatal(err)
	}
	if gjson.GetBytes(g, "generationConfig.topK").Int() != 5 {
		t.Errorf("top_k not sent as generationConfig.topK: %s", g)
	}
}

// A Gemini request's generationConfig sampling fields are read into the IR
// as well, in Google's camelCase and in the snake_case clients send, and
// reach a Chat upstream under Chat's own names.
func TestGeminiSamplingFieldsRead(t *testing.T) {
	r, err := parseGemini([]byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}],` +
		`"generationConfig":{"maxOutputTokens":32,"seed":9,"presence_penalty":0.3,"frequency_penalty":0.2,"top_k":11}}`))
	if err != nil || r.Seed == nil || *r.Seed != 9 || r.PresencePenalty == nil || *r.PresencePenalty != 0.3 ||
		r.FrequencyPenalty == nil || *r.FrequencyPenalty != 0.2 || r.TopK == nil || *r.TopK != 11 {
		t.Fatalf("sampling not read: %v %+v", err, r)
	}
	b := buildChat(r, "gpt-5.5", "", false)
	if gjson.GetBytes(b, "seed").Int() != 9 || gjson.GetBytes(b, "presence_penalty").Float() != 0.3 ||
		gjson.GetBytes(b, "frequency_penalty").Float() != 0.2 {
		t.Errorf("sampling not sent to Chat: %s", b)
	}
	if gjson.GetBytes(b, "top_k").Exists() || gjson.GetBytes(b, "max_tokens").Int() != 32 {
		t.Errorf("Chat sent a top_k or lost the output cap: %s", b)
	}
}
