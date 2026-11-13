package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
)

// A key the vendor refuses outright (401, By "key") says so on its row:
// what set the rest (By), what the vendor said (Error). The page reads By
// to mark the key dead instead of resting, and Error to say the vendor's
// words where only "failed · resting" stood (app.js's keyRestWords and
// keyDead, which were written for this and read nothing until now).
func TestKeyRestSaysWhoAndWhatVendorSaid(t *testing.T) {
	sandboxHome(t)
	until := time.Now().Add(20 * time.Hour).Truncate(time.Second)
	const said = "Incorrect API key provided"
	was := keyRestOf
	keyRestOf = func(key string) (gateway.Rest, bool) {
		if key == "relay" {
			return gateway.Rest{Why: "other", Status: 401, Until: until, By: "key", Key: key, Error: said}, true
		}
		return gateway.Rest{}, false
	}
	defer func() { keyRestOf = was }()
	p := provider.Provider{ID: "relay", Name: "Relay", Chat: "https://example.invalid/v1", Key: "sk-test"}
	out := providerInfoWith(p, provider.Listing{}, nil, nil)
	if len(out.KeyList) != 1 {
		t.Fatalf("keys: %+v", out.KeyList)
	}
	row := out.KeyList[0].Rest
	if row == nil {
		t.Fatal("the refused key's row says nothing")
	}
	if row.By != "key" || row.Error != said || row.Why != "other" || row.Status != 401 || !row.Until.Equal(until) {
		t.Fatalf("row says %+v, want by key with the vendor's words, until %v", row, until)
	}
	// no whole-provider rest: nothing rests by relay* here
	if out.Rest != nil {
		t.Fatalf("the provider rests: %+v", out.Rest)
	}
}

// A subscription account's row says the same of its rest: what set it
// (By), the model resting alone (Model) and the vendor's words (Error).
func TestAccountRestSaysWhoAndWhatVendorSaid(t *testing.T) {
	h := sandboxHome(t)
	cfg := filepath.Dir(provider.Path())
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		t.Fatal(err)
	}
	logins := `[{"agent":"codex","user":"broke@example.com","plan":"PLUS",` +
		`"seen":"` + time.Now().UTC().Format(time.RFC3339) + `","on":true,"auth":{}}]`
	if err := os.WriteFile(filepath.Join(cfg, "logins.json"), []byte(logins), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = h
	until := time.Now().Add(20 * time.Hour).Truncate(time.Second)
	const said = "您的使用量已超出频率限制，将在 23:57 重置"
	was := keyRestOf
	keyRestOf = func(key string) (gateway.Rest, bool) {
		if key == "codex@broke@example.com" {
			return gateway.Rest{Why: "quota", Status: 429, Until: until, By: "resets", Key: key, Model: "gpt-6-astra", Error: said}, true
		}
		return gateway.Rest{}, false
	}
	defer func() { keyRestOf = was }()
	p := provider.Provider{ID: "codex", Name: "Codex", Account: &provider.Account{Agent: "codex", User: "broke@example.com"}}
	out := providerInfoWith(p, provider.Listing{}, nil, nil)
	if out.Account == nil || len(out.Account.Logins) == 0 {
		t.Fatalf("accounts: %+v", out.Account)
	}
	var row *provider.KeyRest
	for _, l := range out.Account.Logins {
		if l.User == "broke@example.com" {
			row = l.Rest
		}
	}
	if row == nil {
		t.Fatalf("the resting account's row says nothing: %+v", out.Account.Logins)
	}
	if row.By != "resets" || row.Model != "gpt-6-astra" || row.Error != said {
		t.Fatalf("row says %+v, want by resets, model gpt-6-astra, the vendor's words", row)
	}
}

// A provider the vendor refused as a whole — its firewall blocking this
// computer's address, its endpoint answering its web page (gateway's
// blockedKey, id+"*") — rests by no key's or account's own name, so none
// of their rows said anything and the whole provider sat out unexplained.
// The provider's state carries it now, and a provider with nothing
// against it, and another provider's block, name nobody else.
func TestProviderBlockedAsWholeInProviderState(t *testing.T) {
	sandboxHome(t)
	until := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	const said = "Just a moment..." // the challenge page where its API should be
	was := keyRestOf
	keyRestOf = func(key string) (gateway.Rest, bool) {
		if key == "relay*" {
			return gateway.Rest{Why: "other", Status: 403, Until: until, By: "blocked", Key: key, Error: said}, true
		}
		return gateway.Rest{}, false
	}
	defer func() { keyRestOf = was }()
	p := provider.Provider{ID: "relay", Name: "Relay", Chat: "https://example.invalid/v1", Key: "sk-test"}
	out := providerInfoWith(p, provider.Listing{}, nil, nil)
	if out.Rest == nil {
		t.Fatal("the blocked provider's row says nothing")
	}
	if out.Rest.Key != "relay*" || out.Rest.By != "blocked" || out.Rest.Error != said || !out.Rest.Until.Equal(until) {
		t.Fatalf("rest says %+v, want by blocked until %v with the page's words", out.Rest, until)
	}
	// no key rests by its own name: the block is the address's
	for _, k := range out.KeyList {
		if k.Rest != nil {
			t.Fatalf("key %+v rests by its own name under a whole-provider block", k)
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Rest *provider.KeyRest `json:"rest"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Rest == nil || got.Rest.Key != "relay*" || got.Rest.By != "blocked" {
		t.Fatalf("the provider's state: %s", b)
	}
}

// A subscription account's rests in force, its own and its models', are
// in the provider's state by the account's user in lower case, the model
// filled in from the rest's key when the note has none; a key provider is
// asked by no "@" prefix and carries none.
func TestAccountRestsInProviderState(t *testing.T) {
	sandboxHome(t)
	until := time.Now().Add(3 * time.Hour).Truncate(time.Second)
	said := "您的使用量已超出频率限制，将在 2026-11-03 23:57:19 UTC+8 重置，您也可以切换其他模型继续使用。"
	was := restsUnder
	asked := []string{}
	restsUnder = func(prefix string) []gateway.Rest {
		asked = append(asked, prefix)
		if prefix != "codex@" {
			return nil
		}
		return []gateway.Rest{
			{Why: "quota", Status: 429, Until: until, By: "resets", Key: "codex@max.lee@example.com/gpt-6-astra", Error: said},
			{Why: "rate", Status: 429, Until: until, By: "cooldown", Key: "codex@other@example.com", Model: "deepseek-v4.1"},
		}
	}
	defer func() { restsUnder = was }()

	p := provider.Provider{ID: "codex", Name: "Codex", Account: &provider.Account{Agent: "codex", User: "Max.Lee@Example.com"}}
	out := providerInfoWith(p, provider.Listing{}, nil, nil)
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		AccountRests map[string][]provider.KeyRest `json:"accountRests"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	m := got.AccountRests["max.lee@example.com"]
	if len(m) != 1 || m[0].Model != "gpt-6-astra" || m[0].Key != "codex@max.lee@example.com/gpt-6-astra" || !m[0].Until.Equal(until) || m[0].Status != 429 || m[0].By != "resets" || m[0].Error != said {
		t.Fatalf("max.lee: %+v (%s)", m, b)
	}
	// the note names the model here: it stands
	if o := got.AccountRests["other@example.com"]; len(o) != 1 || o[0].Model != "deepseek-v4.1" || o[0].Key != "codex@other@example.com" || o[0].Why != "rate" {
		t.Fatalf("other: %+v", o)
	}

	// a provider with keys, none resting: nothing, and not asked by "@"
	asked = asked[:0]
	k := providerInfoWith(provider.Provider{ID: "relay", Name: "Relay", Chat: "https://example.invalid/v1", Key: "k"}, provider.Listing{}, nil, nil)
	if k.AccountRests != nil || len(asked) != 0 {
		t.Fatalf("a key provider: %+v, asked %v", k.AccountRests, asked)
	}
}
