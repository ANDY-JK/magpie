package gateway

import (
	"net/http"
	"os"

	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/provider"
)

// The gateway's two probes (L-03, the review): GET / and /api/hello answer
// as long as the process listens, so a script could not tell a gateway that
// serves from one whose providers.json can't be read. /healthz says the
// process is up; /readyz says it can serve, and why not when it can't.
// Neither reads a provider's sign-in nor asks any vendor.
func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// readyz is what the gateway needs to route: a providers.json that reads,
// and a models folder it can list. A missing folder is a fresh install, not
// a fault.
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if err := provider.FileError(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "why": err.Error()})
		return
	}
	if _, err := os.ReadDir(appdir.Join(appdir.Cache(), "models")); err != nil && !os.IsNotExist(err) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "why": "the models folder can't be listed: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
