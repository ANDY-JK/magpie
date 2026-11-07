package gui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
)

// An account the gateway passes over says why and until when on its row of
// the provider's accounts, as a key's says it on its own. The page draws
// Account.Logins, so the rest has to be on that list: it was filled before
// the account was built, where there was none, and a subscription resting
// the day (zcode@雷_5505, cursor's accounts) said nothing at all.
func TestProviderAccountRestOnItsRow(t *testing.T) {
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
	until := time.Now().Add(20 * time.Hour)
	was := keyRestOf
	keyRestOf = func(key string) (gateway.Rest, bool) {
		if key == "codex@broke@example.com" {
			return gateway.Rest{Why: "credit", Status: 402, Until: until, By: "credit"}, true
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
	for i, l := range out.Account.Logins {
		if l.User == "broke@example.com" {
			row = out.Account.Logins[i].Rest
		}
	}
	if row == nil {
		t.Fatalf("the resting account's row says nothing: %+v", out.Account.Logins)
	}
	if row.Why != "credit" || !row.Until.Equal(until) {
		t.Fatalf("row says %+v, want credit until %v", row, until)
	}
}
