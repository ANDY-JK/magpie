package settings

import (
	"fmt"
	"strings"

	"github.com/yetone/magpie/internal/redact"
)

// How many values the masking allowlist may hold, and how long each.
const (
	maxAllow    = 200
	maxAllowLen = 256
)

// redactActions is m with only the actions redact knows, each for a kind
// named as a placeholder names one (API_KEY, a rule's own); masking, the
// default, is left out.
func redactActions(m map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for k, a := range m {
		k, a = strings.TrimSpace(k), strings.ToLower(strings.TrimSpace(a))
		switch a {
		case "", "mask":
			continue
		case redact.Warn, redact.Block:
		default:
			return nil, fmt.Errorf("what is done with a kind of value found must be mask, %s or %s, not %q", redact.Warn, redact.Block, a)
		}
		if !redact.KindName(k) {
			return nil, fmt.Errorf("%q isn't a kind of value masking finds", k)
		}
		out[k] = a
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// ForUI is the settings as the UI's state is told them: what in them holds
// a secret goes out only masked, so a page served over the network reads
// none of it. The page sends the masked forms back when it saves; the
// routes that set these fields take one back as the value it was
// (UnmaskAllow), the proxy's login comes back to the page from its own
// route when it edits the row, and the OTLP headers' values are set
// through theirs alone.
func (s Settings) ForUI() Settings {
	s.GitHubToken, s.LANKey = "", ""
	s.Proxy, s.ProxyCustom = ProxyWithoutLogin(s.Proxy), ProxyWithoutLogin(s.ProxyCustom)
	if len(s.OTel.Headers) > 0 {
		h := make(map[string]string, len(s.OTel.Headers))
		for k := range s.OTel.Headers {
			h[k] = ""
		}
		s.OTel.Headers = h
	}
	if len(s.RedactAllow) > 0 {
		a := make([]string, len(s.RedactAllow))
		for i, v := range s.RedactAllow {
			a[i] = MaskAllow(v)
		}
		s.RedactAllow = a
	}
	return s
}

// ProxyWithoutLogin is a proxy address with its login taken off, as the UI
// is told the proxy (ForUI): the scheme and address kept, the user:pass
// before the last @ dropped, as the page's own split reads it. One with no
// login is as it was.
func ProxyWithoutLogin(p string) string {
	start := 0
	if i := strings.Index(p, "://"); i >= 0 {
		start = i + 3
	}
	end := strings.IndexAny(p[start:], "/?#")
	if end < 0 {
		end = len(p) - start
	}
	at := strings.LastIndexByte(p[start:start+end], '@')
	if at < 0 {
		return p
	}
	return p[:start] + p[start+at+1:]
}

// MaskAllow is a masking-allowlist value as the UI is told it (ForUI) and
// sends it back: its ends with an ellipsis between, all dots when it is
// short. UnmaskAllow takes one back as the value it was.
func MaskAllow(v string) string {
	r := []rune(v)
	if len(r) <= 8 {
		return strings.Repeat("•", len(r))
	}
	return string(r[:4]) + "…" + string(r[len(r)-4:])
}

// UnmaskAllow is the allow list the page sent back, with each value it
// holds only masked (MaskAllow) the value it was, so editing the list as
// shown never writes a masked form over a real one. A value the list holds
// as written stands, then one that is only how a value is shown stands for
// it, each once; the rest are new or edited, as sent, for the save to trim
// and bound.
func UnmaskAllow(had, sent []string) []string {
	out := make([]string, 0, len(sent))
	used := make([]bool, len(had))
	for _, v := range sent {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		kept := -1
		for i, e := range had {
			if !used[i] && e == v {
				kept = i
				break
			}
		}
		for i, e := range had {
			if kept >= 0 {
				break
			}
			if !used[i] && MaskAllow(e) == v {
				kept = i
			}
		}
		if kept < 0 {
			out = append(out, v)
			continue
		}
		used[kept] = true
		out = append(out, had[kept])
	}
	return out
}

// redactAllow is the allowlist trimmed, each value once whatever its case.
func redactAllow(vs []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, v := range vs {
		v = strings.TrimSpace(v)
		if v == "" || seen[strings.ToLower(v)] {
			continue
		}
		if len(v) > maxAllowLen {
			return nil, fmt.Errorf("a value the masking lets go is at most %d characters", maxAllowLen)
		}
		seen[strings.ToLower(v)] = true
		out = append(out, v)
	}
	if len(out) > maxAllow {
		return nil, fmt.Errorf("the masking lets at most %d values go", maxAllow)
	}
	return out, nil
}
