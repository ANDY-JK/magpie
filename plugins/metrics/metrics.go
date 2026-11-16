// Package metrics is the Metrics page: what the gateway and this process
// hold and spend right now — the requests in flight and the turns waiting
// on their tool results, with each request listed (agent, model, provider,
// phase, how long); the process's memory, CPU and goroutines, with the last
// four hours of them; and calls by hour, with their failures. It reads the
// three endpoints (/api/busy, /api/process, /api/usage/hourly) and changes
// nothing by itself; what other pages already show, it links to instead of
// drawing again.
package metrics

import "github.com/yetone/magpie/internal/pluginrt"

func init() {
	pluginrt.Register(pluginrt.Manifest{
		ID: "metrics", API: "^1", Requires: []string{"window"},
		Name: "Metrics", Sub: "Requests in flight, this process's memory and CPU, and calls by hour",
		Parts:  []pluginrt.Part{{Slot: "nav.page", Key: "metrics"}},
		Client: []string{"i18n.js", "client.js"},
	})
}
