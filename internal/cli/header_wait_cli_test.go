package cli

import (
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// F-02: magpie provider header-wait <id> [minutes|default] sets how long a
// provider's vendors are given to begin answering; default puts back the
// gateway's own 10 minutes, and 0 or 1441 are refused.
func TestHeaderWaitCmd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("DSH_HOME", "")
	if err := provider.Save(provider.Provider{ID: "zc", Name: "ZC", Key: "k1", Chat: "http://127.0.0.1:1/v1"}); err != nil {
		t.Fatal(err)
	}
	set := func() *int {
		p, err := provider.Find("zc")
		if err != nil {
			t.Fatal(err)
		}
		return p.HeaderWait
	}
	if err := providerCmd([]string{"provider", "header-wait", "zc", "30"}); err != nil {
		t.Fatal(err)
	}
	if n := set(); n == nil || *n != 30 {
		t.Fatalf("set: %v, want 30", n)
	}
	if err := providerCmd([]string{"provider", "header-wait", "zc"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{{"0"}, {"1441"}, {"-5"}, {"x"}, {"2", "3"}} {
		if err := providerCmd(append([]string{"provider", "header-wait", "zc"}, bad...)); err == nil {
			t.Fatalf("%v taken", bad)
		}
	}
	if n := set(); n == nil || *n != 30 {
		t.Fatalf("after refused ones: %v, want 30", n)
	}
	if err := providerCmd([]string{"provider", "header-wait", "zc", "default"}); err != nil {
		t.Fatal(err)
	}
	if n := set(); n != nil {
		t.Fatalf("default: %d left set", *n)
	}
}
