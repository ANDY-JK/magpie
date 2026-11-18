//go:build !nogui

package gui

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// The all-down notice, in the page's language: the model when one is named.
// It names no reason (resting keys and accounts, or the proxy/network down)
// and no time to wait, as the notice is the outage's, not the recovery's.
func TestAllDownText(t *testing.T) {
	for _, c := range []struct {
		lang, model, title, body string
	}{
		{"en", "relay/m", "No channel can answer", "No channel can answer relay/m right now"},
		{"en", "", "No channel can answer", "No channel can answer right now"},
		{"zh", "relay/m", "渠道全部不可用", "「relay/m」暂无渠道可应答"},
		{"zh-TW", "relay/m", "渠道全部不可用", "「relay/m」暫無渠道可應答"},
		{"ja", "relay/m", "全チャネル応答不能", "relay/m に応答できるチャネルがありません"},
		{"de", "relay/m", "Alle Kanäle außer Betrieb", "Kein Kanal kann relay/m gerade beantworten"},
	} {
		if title, body := allDownText(c.lang, c.model); title != c.title || body != c.body {
			t.Errorf("%s %s: %q / %q, want %q / %q", c.lang, c.model, title, body, c.title, c.body)
		}
	}
}

// A click on an all-down notice opens the Providers page, not Usage: what
// rested is a channel, and its rest is there. A Linux notice says nothing
// but a click.
func TestAllDownClickOpensProviders(t *testing.T) {
	id := downNotice + "1"
	for _, c := range []struct {
		goos, action, want string
	}{
		{"windows", notifications.DefaultActionIdentifier, "providers"},
		{"darwin", notifications.DefaultActionIdentifier, "providers"},
		{"linux", linuxClick, "providers"},
		{"linux", notifications.DefaultActionIdentifier, ""}, // dismissed
	} {
		r := notifications.NotificationResponse{ID: id, ActionIdentifier: c.action}
		if got := clickView(c.goos, r); got != c.want {
			t.Errorf("clickView(%s, %s %s) = %q, want %q", c.goos, id, c.action, got, c.want)
		}
	}
}
