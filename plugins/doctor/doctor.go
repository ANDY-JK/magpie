// Package doctor is the Doctor page: who is resting, why, and what failed
// lately, with Try now for a resting key or account and a way to the
// provider it belongs to. It reads what the window already has (the
// providers list and the Requests the gateway logged) and changes nothing
// by itself: Try now ends one rest, as the Providers page's button does.
package doctor

import "github.com/yetone/magpie/internal/pluginrt"

func init() {
	pluginrt.Register(pluginrt.Manifest{
		ID: "doctor", API: "^1", Requires: []string{"window"},
		Name: "Doctor", Sub: "Who is resting, why, and what failed lately",
		Parts:  []pluginrt.Part{{Slot: "nav.page", Key: "doctor"}},
		Client: []string{"i18n.js", "client.js"},
	})
}
