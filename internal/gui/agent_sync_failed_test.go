package gui

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// B-02: an agent's model list the sync couldn't write is said on its row by
// the first failure the sync kept, with how many more distinct ones there
// are; a row whose sync wrote last says nothing.
func TestSyncErrorOf(t *testing.T) {
	if got := syncErrorOf(nil); got != "" {
		t.Fatalf("no failures said %q", got)
	}
	one := errors.New("open /fixture/pi/models.json: permission denied")
	if got := syncErrorOf([]error{one}); got != one.Error() {
		t.Fatalf("one failure said %q, want %q", got, one.Error())
	}
	two := syncErrorOf([]error{one, errors.New("rename: file is open elsewhere")})
	if want := one.Error() + " (+1)"; two != want {
		t.Fatalf("two failures said %q, want %q", two, want)
	}
}

// And the row carries it to the page as syncError, left out when the sync
// wrote last.
func TestAgentJSONCarriesSyncError(t *testing.T) {
	b, err := json.Marshal(agentJSON{SyncError: "permission denied"})
	if err != nil || !strings.Contains(string(b), `"syncError":"permission denied"`) {
		t.Fatalf("a kept failure is not in the row's JSON: %s (%v)", b, err)
	}
	b, err = json.Marshal(agentJSON{})
	if err != nil || strings.Contains(string(b), "syncError") {
		t.Fatalf("a row with no failure names one: %s (%v)", b, err)
	}
}
