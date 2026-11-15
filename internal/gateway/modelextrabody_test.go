package gateway

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// said is the raw JSON at a path of what the vendor was sent.
func said(body []byte, path string) string {
	return gjson.GetBytes(body, path).Raw
}

// giveModelExtra keeps one model's own extra body fields (settings'
// ModelExtraBody, the editor's Names & levels) for the test's provider, as
// its editor's Save would.
func giveModelExtra(t *testing.T, pid, model, extra string) {
	t.Helper()
	eb := provider.ExtraBody{}
	if json.Unmarshal([]byte(extra), &eb) != nil {
		t.Fatal(extra)
	}
	if err := provider.SetModelPrefs(pid, map[string]provider.ModelPref{model: {ExtraBody: &eb}}); err != nil {
		t.Fatal(err)
	}
}

// A model's own extra body fields ride its requests over the provider's,
// filling in what the request lacks: its own names, and its value for one
// its provider gives too.
func TestModelExtraBodyModelTierRidesTheRequest(t *testing.T) {
	f := &fake{t: t, reply: responsesReply, ctype: "application/json"}
	setup(t, provider.Responses, f)
	withExtraBody(t, "fake", `{"tier":"key"}`)
	giveModelExtra(t, "fake", "m1", `{"who":"model-1"}`)
	code, body := postWith(t, "/v1/responses", `{"model":"m1","input":"hi"}`, map[string]string{"session_id": "s-1"})
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	if said(f.got, "who") != `"model-1"` || said(f.got, "tier") != `"key"` {
		t.Errorf("the model's fields in %s", f.got)
	}
}

// The provider's and the model's fields are one merge: a name both levels
// give goes out as the model's says it, the rest of each level's standing.
func TestModelExtraBodyModelTierOverTheProviderTier(t *testing.T) {
	f := &fake{t: t, reply: responsesReply, ctype: "application/json"}
	setup(t, provider.Responses, f)
	withExtraBody(t, "fake", `{"who":"provider","tier":"key"}`)
	giveModelExtra(t, "fake", "m1", `{"who":"model-1","extra":"m"}`)
	code, body := postWith(t, "/v1/responses", `{"model":"m1","input":"hi"}`, nil)
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	if said(f.got, "who") != `"model-1"` || said(f.got, "tier") != `"key"` || said(f.got, "extra") != `"m"` {
		t.Errorf("the model's over the provider's in %s", f.got)
	}
}

// The "!" a model writes forces its field over the request's own, as a
// provider's does; its plain one keeps the request's, as it keeps the
// provider's.
func TestModelExtraBodyForcedNameOverTheBody(t *testing.T) {
	f := &fake{t: t, reply: responsesReply, ctype: "application/json"}
	setup(t, provider.Responses, f)
	withExtraBody(t, "fake", `{"service_tier":"key"}`)
	giveModelExtra(t, "fake", "m1", `{"!service_tier":"flex"}`)
	code, body := postWith(t, "/v1/responses", `{"model":"m1","input":"hi","service_tier":"auto"}`, nil)
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	if said(f.got, "service_tier") != `"flex"` {
		t.Errorf("the model's forced service_tier in %s", f.got)
	}
}

// A model's fields keyed by the id magpie knows it by are in force under
// the name the request goes out with too (a wire name, SetUpstreamName):
// the body alone says the vendor's by then.
func TestModelExtraBodyUnderAWireName(t *testing.T) {
	f := &fake{t: t, reply: responsesReply, ctype: "application/json"}
	setup(t, provider.Responses, f)
	withExtraBody(t, "fake", `{"who":"provider"}`)
	giveModelExtra(t, "fake", "m1", `{"who":"model-1"}`)
	if err := provider.SetUpstreamName("fake/m1", "vendor-m1"); err != nil {
		t.Fatal(err)
	}
	code, body := postWith(t, "/v1/responses", `{"model":"m1","input":"hi"}`, nil)
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	if said(f.got, "model") != `"vendor-m1"` || said(f.got, "who") != `"model-1"` {
		t.Errorf("under the wire name %s", f.got)
	}
}

// A model's fields written into the settings file by hand naming what
// magpie decides itself are left out, and the request goes as it would
// without them (cleanExtraBody keeps the provider's own file so; the
// settings' merge skips them).
func TestModelExtraBodyReservedLeftOut(t *testing.T) {
	f := &fake{t: t, reply: responsesReply, ctype: "application/json"}
	setup(t, provider.Responses, f)
	s := settings.Load()
	s.ModelExtraBody = map[string]settings.ExtraBody{"fake/m1": {
		"stream": json.RawMessage(`true`),
		"model":  json.RawMessage(`"other"`),
	}}
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	code, body := postWith(t, "/v1/responses", `{"model":"m1","input":"hi"}`, nil)
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	if said(f.got, "model") != `"m1"` || gjson.GetBytes(f.got, "stream").Exists() {
		t.Errorf("a reserved name sent: %s", f.got)
	}
}
