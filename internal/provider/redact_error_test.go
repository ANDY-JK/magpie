package provider

import (
	"strings"
	"testing"
)

// A vendor that echoes a channel's key in its error text must hand on the
// masked form: the client, app.log, the trace and the usage ledger all read
// that text. Another channel's key, and a value too short to be a secret,
// are left exactly as they came.
func TestRedactErrorLeavesOtherChannelsAndShortWords(t *testing.T) {
	key := "sk-live-1234567890abcdef"
	second := "sk-key-account-22223333"
	secret := "secret-access-key-4444"
	balance := "balance-token-5555"
	other := "sk-live-other-0987654321"
	short := "sk-1234"

	p := Provider{
		Key:             key,
		Keys:            []KeyAccount{{Key: second}},
		SecretAccessKey: secret,
		BalanceToken:    balance,
	}
	said := "Incorrect API key provided: " + key +
		"; also " + second + ", " + secret + ", " + balance +
		"; another channel's " + other + "; a short word " + short
	got := p.RedactError(said)

	for _, k := range []string{key, second, secret, balance} {
		if strings.Contains(got, k) {
			t.Errorf("the provider's own key %q is still in %q", k, got)
		}
		if !strings.Contains(got, Mask(k)) {
			t.Errorf("the provider's own key isn't masked as %q in %q", Mask(k), got)
		}
	}
	if !strings.Contains(got, other) {
		t.Errorf("another channel's key was replaced: %q", got)
	}
	if !strings.Contains(got, short) {
		t.Errorf("a value shorter than 8 characters was replaced: %q", got)
	}

	if plain := (Provider{}).RedactError("nothing to hide"); plain != "nothing to hide" {
		t.Errorf("a provider with no keys changed %q", plain)
	}
}
