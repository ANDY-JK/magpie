package gateway

import (
	"testing"
	"time"
)

// SunkKeys says what sank, by its restKey or one of its models' (restID,
// key/model), and stops saying it once the sink is lifted (unsinkAll):
// the page asks it with the keys rest rows carry, to say a rate limited
// one is held at the back of the order rather than resting. An account
// that never sank is not said sunk.
func TestSunkKeysSaysKeyAndItsModels(t *testing.T) {
	defer unsinkAll("codex@max@example.com")
	now := time.Now()
	sink("codex@max@example.com", now)
	sink("codex@max@example.com/gpt-6", now)
	if !SunkKeys("codex@max@example.com") {
		t.Fatal("the sunk account is not said sunk")
	}
	if !SunkKeys("codex@max@example.com/gpt-6") {
		t.Fatal("the sunk model is not said sunk by its own rest key")
	}
	if SunkKeys("codex@spare@example.com") {
		t.Fatal("an account that never sank is said sunk")
	}
	unsinkAll("codex@max@example.com")
	if SunkKeys("codex@max@example.com") || SunkKeys("codex@max@example.com/gpt-6") {
		t.Fatal("the lift left SunkKeys saying sunk")
	}
}
