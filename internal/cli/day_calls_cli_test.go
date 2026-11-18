package cli

import (
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// magpie provider day-calls sets how many requests the provider as a whole
// takes in a day, off for none; a number out of range is an error.
func TestDayCallsCmd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("DSH_HOME", "")
	if err := provider.Save(provider.Provider{ID: "or", Name: "OpenRouter", Key: "k1", Chat: "http://127.0.0.1:1/v1"}); err != nil {
		t.Fatal(err)
	}
	dayCalls := func() int {
		p, err := provider.Find("or")
		if err != nil {
			t.Fatal(err)
		}
		return p.DayCallLimit()
	}
	if err := providerCmd([]string{"provider", "day-calls", "or", "500"}); err != nil {
		t.Fatal(err)
	}
	if n := dayCalls(); n != 500 {
		t.Fatalf("set: %d, want 500", n)
	}
	if err := providerCmd([]string{"provider", "day-calls", "or"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{{"-3"}, {"100001"}, {"x"}, {"2", "3"}} {
		if err := providerCmd(append([]string{"provider", "day-calls", "or"}, bad...)); err == nil {
			t.Fatalf("%v taken", bad)
		}
	}
	if n := dayCalls(); n != 500 {
		t.Fatalf("after refused ones: %d, want 500", n)
	}
	if err := providerCmd([]string{"provider", "day-calls", "or", "off"}); err != nil {
		t.Fatal(err)
	}
	if n := dayCalls(); n != 0 {
		t.Fatalf("off: %d", n)
	}
}
