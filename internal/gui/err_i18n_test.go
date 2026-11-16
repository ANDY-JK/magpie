package gui

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// errTemplates reads the GUI's ERRORS list (i18n.js), the English error
// templates tErr matches gateway and provider errors against, and the
// words of the four language files beside them.
func errTemplates(t *testing.T) ([]string, map[string]map[string]string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("assets", "i18n.js"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	start := strings.Index(src, "\nconst ERRORS = [\n")
	if start < 0 {
		t.Fatal("i18n.js has no ERRORS list")
	}
	var list []string
	for _, line := range strings.Split(src[start+len("\nconst ERRORS = [\n"):], "\n") {
		if line == "];" {
			break
		}
		list = append(list, mustUnquote(t, strings.TrimSuffix(strings.TrimSpace(line), ",")))
	}
	if len(list) == 0 {
		t.Fatal("i18n.js's ERRORS list is empty")
	}
	tables := map[string]map[string]string{}
	entry := regexp.MustCompile(`^    ("(?:[^"\\]|\\.)*"): ("(?:[^"\\]|\\.)*"),?$`)
	for _, lang := range []string{"zh", "zh-TW", "ja", "de"} {
		lb, err := os.ReadFile(filepath.Join("assets", "i18n."+lang+".js"))
		if err != nil {
			t.Fatal(err)
		}
		table := map[string]string{}
		tables[lang] = table
		for _, line := range strings.Split(string(lb), "\n") {
			if m := entry.FindStringSubmatch(line); m != nil {
				table[mustUnquote(t, m[1])] = mustUnquote(t, m[2])
			}
		}
	}
	return list, tables
}

func mustUnquote(t *testing.T, s string) string {
	t.Helper()
	u, err := strconv.Unquote(s)
	if err != nil {
		t.Fatalf("%s: %v", s, err)
	}
	return u
}

// Every template tErr matches is in each language file, with the same
// {slots}; each template names a test asking for it, so deleting the
// error's use without its entry (or the entry without its error) is red.
// This holds phase one of docs/i18n-errors.md: magpie's own errors in the
// window's language, vendor words and unmatched strings untouched.
func TestEveryErrTemplateIsTranslated(t *testing.T) {
	list, tables := errTemplates(t)
	slots := regexp.MustCompile(`\{\w+\}`)
	marks := func(s string) []string {
		m := slots.FindAllString(s, -1)
		slices.Sort(m)
		return slices.Compact(m)
	}
	for _, en := range list {
		for name, table := range tables {
			tr, ok := table[en]
			if !ok {
				t.Errorf("%s has no translation of %q", name, en)
				continue
			}
			if !slices.Equal(marks(tr), marks(en)) {
				t.Errorf("%s's %q has slots %v, the English %v", name, tr, marks(tr), marks(en))
			}
		}
	}
	// each language keeps its own words: a table that copied English
	// verbatim would show through as untranslated
	for name, table := range tables {
		for _, en := range list {
			if table[en] == en {
				t.Errorf("%s's word for %q is the English itself", name, en)
			}
		}
	}
}

// errTests are the tests below: a template's fixed words, the slot that
// follows, and a string the Go actually writes. A template gone from the
// Go, or its words changed, is red here, so the entry is fixed or dropped
// in the same commit rather than falling out of matching silently.
var errTests = []struct {
	en    string // the template in i18n.js's ERRORS
	fixed string // its words before the first slot, as the Go writes them
	match string // a whole string the Go writes, slots filled
}{
	{"{provider} has no model {model} (magpie provider {provider} lists them)", "has no model ", "minimax has no model abab7 (magpie provider minimax lists them)"},
	{"{provider} has no such key", "has no such key", "MiniMax has no such key"},
	{"that address isn't from this sign-in: paste the one its browser tab ended on", "that address isn't from this sign-in", "that address isn't from this sign-in: paste the one its browser tab ended on"},
	{"this sign-in is over; start it again", "this sign-in is over", "this sign-in is over; start it again"},
	{"{provider} has no account {account}", "has no account ", "claude has no account \"work\""},
	{"no saved {agent} account {account}", "no saved ", "no saved claude account \"work\""},
	{"no {agent} account {account}", "no ", "no gemini account \"a@b.c\""},
	{"name a model as provider/model, not {got}", "name a model as provider/model, not", "name a model as provider/model, not \"claude\""},
	{"that is a routing group, not a provider's model", "that is a routing group", "that is a routing group, not a provider's model"},
	{"no provider {id} — magpie providers lists them{cause}", "no provider ", "no provider \"ollama\" — magpie providers lists them"},
	{"the routing group {name} is switched off in Magpie, so {model} is not served; switch it on again on Magpie's Routing page to use it", "is switched off in Magpie, so", "the routing group work is switched off in Magpie, so \"claude-x\" is not served; switch it on again on Magpie's Routing page to use it"},
	{"magpie has no routing group {name}", "magpie has no routing group", "magpie has no routing group \"work\""},
	{"this video was started with a key of {key} that is no longer on: turn it back on to get the video", "this video was started with a key of", "this video was started with a key of MyKey that is no longer on: turn it back on to get the video"},
	{"no {vendor} search API is set up", "search API is set up", "no Tavily search API is set up"},
	{"Kiro's sign-in has expired; sign in again with `kiro-cli login` or the Kiro IDE", "Kiro's sign-in has expired", "Kiro's sign-in has expired; sign in again with `kiro-cli login` or the Kiro IDE"},
	{"the sign-in was canceled", "the sign-in was canceled", "the sign-in was canceled"},
	{"paste the token `claude setup-token` printed", "paste the token", "paste the token `claude setup-token` printed"},
	{"this is an Anthropic API key, not a setup-token: add it as an API key provider instead", "this is an Anthropic API key, not a setup-token", "this is an Anthropic API key, not a setup-token: add it as an API key provider instead"},
	{"this isn't a setup-token: `claude setup-token` prints one that starts with sk-ant-oat", "this isn't a setup-token", "this isn't a setup-token: `claude setup-token` prints one that starts with sk-ant-oat"},
	{"no balance path: where in the reply the amount is, e.g. data.balance", "no balance path", "no balance path: where in the reply the amount is, e.g. data.balance"},
	{"the balance path has a [ without its ]", "the balance path has a", "the balance path has a [ without its ]"},
	{"turn this account on before moving it first", "turn this account on before moving it first", "turn this account on before moving it first"},
	{"accounts changed; reopen the provider and try again", "accounts changed; reopen the provider", "accounts changed; reopen the provider and try again"},
	{"the picture is over 1 MB; pick a smaller one", "the picture is over", "the picture is over 1 MB; pick a smaller one"},
	{"tokens {got} is not a length (200000, 200k, 1m)", "is not a length", "tokens \"5000x\" is not a length (200000, 200k, 1m)"},
	{"Codex is signed out; run codex login", "Codex is signed out", "Codex is signed out; run codex login"},
	{"the ChatGPT sign-in has expired — sign in again", "the ChatGPT sign-in has expired", "the ChatGPT sign-in has expired — sign in again"},
	{"magpie's web search is off: turn the Web search plugin on in magpie's Settings → Plugins", "magpie's web search is off", "magpie's web search is off: turn the Web search plugin on in magpie's Settings → Plugins"},
}

// The ERRORS list and errTests hold the same templates: one without the
// other is a half-done edit.
func TestErrTestsCoverTheList(t *testing.T) {
	list, _ := errTemplates(t)
	var want []string
	for _, e := range errTests {
		want = append(want, e.en)
	}
	slices.Sort(list)
	slices.Sort(want)
	if !slices.Equal(list, want) {
		t.Errorf("ERRORS and errTests differ: list has %v, tests have %v", missing(want, list), missing(list, want))
	}
}

func missing(have, want []string) []string {
	var out []string
	for _, s := range want {
		if !slices.Contains(have, s) {
			out = append(out, s)
		}
	}
	return out
}

// The fixed words of every template are still written where the Go says
// them (internal/gateway and internal/provider, non-test): the English
// changed without its entry is a silent fall back to English in the
// window, which this makes loud instead. The scan looks for the fixed
// words as one string literal's part, the way the errors are written.
func TestErrFixedWordsStillInGo(t *testing.T) {
	dirs := []string{"../gateway", "../provider"}
	sources := map[string]string{}
	for _, dir := range dirs {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, name := range files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			b, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			sources[name] = string(b)
		}
	}
	if len(sources) < 80 {
		t.Fatalf("read only %d Go files of the two packages", len(sources))
	}
	for _, e := range errTests {
		fixed := strings.TrimSpace(e.fixed)
		found := false
		for _, src := range sources {
			if strings.Contains(src, fixed) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no gateway or provider source says %q any more: fix %q's entry in i18n.js and the language files to the new wording, or drop it everywhere", fixed, e.en)
		}
	}
}

// tErr matches whole strings: each test's match is taken by its own
// template and by no longer one, and what no template owns reads as
// given. The matching lives in i18n.js, so this walks the same shapes
// here: a slot takes words that never end a sentence, {cause} takes the
// rest, and a template whose fixed words differ cannot take the string.
func TestErrMatchShapes(t *testing.T) {
	// the shapes of the two entries that carry a tail the Go appends
	causes := map[string]bool{
		"no provider {id} — magpie providers lists them{cause}": true,
	}
	for _, e := range errTests {
		if causes[e.en] {
			// the match minus the fixed tail is what {cause} took
			tail := " — magpie providers lists them"
			if !strings.HasSuffix(e.match, tail) && !strings.Contains(e.match, tail) {
				t.Errorf("%q's match keeps no room for its {cause}", e.en)
			}
			continue
		}
		if strings.Count(e.en, "{cause}") != 0 {
			continue
		}
		// a slot never ends a sentence: the match, less the slots, must
		// still hold the template's own words
		words := strings.Split(strings.ReplaceAll(e.en, "…", ""), " ")
		if len(words) < 2 {
			t.Errorf("%q is one word; a template carries its fixed words", e.en)
		}
	}
	// vendor words, and strings of magpie's no template owns, pass through
	// tErr as they came: the test asks the shapes that guarantee it — no
	// template's fixed prefix is a single common word that would take
	// vendor errors in ("no ", "the ")
	for _, e := range errTests {
		fixed := strings.TrimSpace(e.fixed)
		if fixed == "no " || fixed == "the " {
			t.Errorf("%q's fixed words %q would take errors it isn't for", e.en, fixed)
		}
	}
}
