package gateway

import (
	"testing"
	"time"
)

// RestsUnder lists the rests in force under a key prefix — a
// subscription's accounts by "<provider>@", each its own and each of its
// models' (provider@user/model) — soonest back first, each with Key and
// Until filled from the resting map, not the note. The Providers page
// reads them there: restOf alone finds one exact key and misses a model's
// rest. A rest gone by, another provider's, and a key's (no "@") are not
// listed.
func TestRestsUnderListsWhatRestsUnderAPrefix(t *testing.T) {
	until := time.Now().Add(time.Hour).Truncate(time.Second)
	restingUntil.Lock()
	wasM, wasNote := restingUntil.m, restingUntil.note
	restingUntil.m = map[string]time.Time{
		"codex@max@example.com":       until,
		"codex@max@example.com/gpt-6": until.Add(-20 * time.Minute),
		"codex@gone@example.com":      time.Now().Add(-time.Second),
		"cursor@max@example.com":      until,
		"codex#keyid":                 until,
	}
	restingUntil.note = map[string]Rest{
		"codex@max@example.com":       {Why: "quota", By: "quota"},
		"codex@max@example.com/gpt-6": {Why: "quota", By: "resets", Model: "gpt-6"},
		"codex@gone@example.com":      {Why: "quota"},
		"cursor@max@example.com":      {Why: "rate"},
		"codex#keyid":                 {Why: "rate"},
	}
	restingUntil.Unlock()
	defer func() {
		restingUntil.Lock()
		restingUntil.m, restingUntil.note = wasM, wasNote
		restingUntil.Unlock()
	}()

	got := RestsUnder("codex@")
	if len(got) != 2 {
		t.Fatalf("listed %d rests, want the account's and its model's alone: %+v", len(got), got)
	}
	// soonest back first: the model's rest ends before the account's
	if got[0].Key != "codex@max@example.com/gpt-6" || got[0].Why != "quota" || got[0].By != "resets" || got[0].Model != "gpt-6" || !got[0].Until.Equal(until.Add(-20*time.Minute)) {
		t.Fatalf("first: %+v, want the model's rest", got[0])
	}
	if got[1].Key != "codex@max@example.com" || got[1].Why != "quota" || got[1].By != "quota" || !got[1].Until.Equal(until) {
		t.Fatalf("second: %+v, want the account's rest", got[1])
	}
	// one account's own prefix lists its two, and never another's
	if all := RestsUnder("codex@max@example.com"); len(all) != 2 {
		t.Fatalf("the account's prefix listed %d rests: %+v", len(all), all)
	}
}
