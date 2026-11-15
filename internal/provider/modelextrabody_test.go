package provider

import (
	"encoding/json"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/settings"
)

// modelExtraHome is one relay of the user's own serving two models, whose
// requests carry an extra body of the provider's own to fall under.
func modelExtraHome(t *testing.T) {
	t.Helper()
	azureHome(t)
	base := "https://relay.example.invalid/v1"
	if err := Save(Provider{ID: "relay", Name: "Relay", Key: "sk-relay", Chat: base,
		ExtraBody: ExtraBody{"tier": json.RawMessage(`"key"`), "!prompt_cache_key": json.RawMessage(`"key-cache"`)}}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive("relay", base, []catalog.Model{{ID: "m"}, {ID: "free"}}); err != nil {
		t.Fatal(err)
	}
}

// giveExtra stages one model's extra body fields with the editor's Save
// (SetModelPrefs), as the Names & levels row does; an empty object stages
// an emptied box, which takes the model's own away.
func giveExtra(t *testing.T, model string, eb *ExtraBody) {
	t.Helper()
	if err := SetModelPrefs("relay", map[string]ModelPref{model: {ExtraBody: eb}}); err != nil {
		t.Fatal(err)
	}
}

func rawExtra(t *testing.T, s string) *ExtraBody {
	t.Helper()
	e := ExtraBody{}
	if json.Unmarshal([]byte(s), &e) != nil {
		t.Fatalf("%s is not an object", s)
	}
	return &e
}

// bodyOf is what the relay sends for a request on model, read back as an
// object; no model for the provider's fields alone.
func bodyOf(t *testing.T, p Provider, model, body string, c Conversation) map[string]json.RawMessage {
	t.Helper()
	var out []byte
	if model == "" {
		out = p.WithExtraBody([]byte(body), c)
	} else {
		out = p.WithModelExtraBody(model, []byte(body), c)
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(out, &m) != nil {
		t.Fatalf("%s: %s", body, out)
	}
	return m
}

func wantRaw(t *testing.T, body map[string]json.RawMessage, name, want string) {
	t.Helper()
	if got, ok := body[name]; !ok || string(got) != want {
		t.Fatalf("%s is %q (have %v), want %s", name, got, body, want)
	}
}

// The editor's Names & levels sends a model's extra body fields with its
// Save: they are kept in settings' ModelExtraBody under the provider id and
// the model's own, an emptied box takes them away again, and one naming a
// field magpie sets itself, naming one twice, or set for a model the
// provider does not serve is refused, leaving what was kept as it was.
func TestModelExtraBodySaveRoundTrip(t *testing.T) {
	modelExtraHome(t)
	if _, held := settings.Load().ModelExtraBody["relay/free"]; held {
		t.Fatal("nothing kept yet")
	}
	giveExtra(t, "free", rawExtra(t, `{"who":"free-1","prompt_cache_key":"${sessionId}"}`))
	kept, held := settings.Load().ModelExtraBody["relay/free"]
	if !held || string(kept["who"]) != `"free-1"` {
		t.Fatalf("kept %v, want who as given", kept)
	}
	p := findProvider(t, "relay")
	// the model's fields ride its requests over the provider's own
	body := bodyOf(t, p, "free", `{"model":"free"}`, Conversation{Session: "s1"})
	wantRaw(t, body, "who", `"free-1"`)
	wantRaw(t, body, "prompt_cache_key", `"s1"`)
	// the provider's fields alone reach its other model
	body = bodyOf(t, p, "", `{"model":"m"}`, Conversation{Session: "s1"})
	wantRaw(t, body, "prompt_cache_key", `"key-cache"`)
	if _, ok := body["who"]; ok {
		t.Fatalf("the other model carries the free model's fields: %v", body)
	}
	// an emptied box takes the model's own away, the provider's staying
	giveExtra(t, "free", &ExtraBody{})
	if _, held := settings.Load().ModelExtraBody["relay/free"]; held {
		t.Fatal("taken away, still kept")
	}
	// a field magpie sets itself is refused, as one named twice is, and so
	// is a set for a model the provider does not serve
	for _, bad := range []struct{ model, eb string }{{"free", `{"model":"other"}`}, {"free", `{"!tier":1,"tier":2}`}, {"nope", `{"who":"x"}`}} {
		if err := SetModelPrefs("relay", map[string]ModelPref{bad.model: {ExtraBody: rawExtra(t, bad.eb)}}); err == nil {
			t.Fatalf("%s for %s was kept", bad.eb, bad.model)
		}
	}
	if _, held := settings.Load().ModelExtraBody["relay/free"]; held {
		t.Fatal("a refused set was kept")
	}
}

// The model's own fields go with it to the id it is listed by (SetModelID),
// as every per-model setting does.
func TestModelExtraBodyKeyMovesWithID(t *testing.T) {
	modelExtraHome(t)
	giveExtra(t, "free", rawExtra(t, `{"who":"free-1"}`))
	setID(t, "relay/free", "free-2")
	s := settings.Load()
	if _, held := s.ModelExtraBody["relay/free"]; held {
		t.Fatal("the key under the old id is still there")
	}
	kept, held := s.ModelExtraBody["relay/free-2"]
	if !held || string(kept["who"]) != `"free-1"` {
		t.Fatalf("relay/free-2 holds %v, want who as given", kept)
	}
	body := bodyOf(t, findProvider(t, "relay"), "free-2", `{"model":"free-2"}`, Conversation{})
	wantRaw(t, body, "who", `"free-1"`)
	wantRaw(t, body, "prompt_cache_key", `"key-cache"`) // the provider's, still under it
}

// A name the model and its provider both give goes out as the model's says
// it, the "!" it wrote or left off with it, and a name only one of them
// gives is that one's alone; the request's own field is kept unless the
// model forces it, as it is against the provider's.
func TestModelExtraBodyModelOverTheProviderTier(t *testing.T) {
	modelExtraHome(t)
	giveExtra(t, "free", rawExtra(t, `{"tier":"model","prompt_cache_key":"${sessionId}","!max_output_tokens":99}`))
	p := findProvider(t, "relay")
	body := bodyOf(t, p, "free", `{"model":"free"}`, Conversation{Session: "s1"})
	// the model's tier over the provider's
	wantRaw(t, body, "tier", `"model"`)
	// the model's cache key is its session's, over the provider's forced one
	wantRaw(t, body, "prompt_cache_key", `"s1"`)
	// the model's forced field, only it giving it, stands
	wantRaw(t, body, "max_output_tokens", `99`)
	// the request's own field is kept against the model's plain one, as it
	// is against the provider's, and the model's "!" replaces it
	body = bodyOf(t, p, "free", `{"model":"free","tier":"client"}`, Conversation{Session: "s1"})
	wantRaw(t, body, "tier", `"client"`)
	body = bodyOf(t, p, "free", `{"model":"free","max_output_tokens":10}`, Conversation{Session: "s1"})
	wantRaw(t, body, "max_output_tokens", `99`)
}
