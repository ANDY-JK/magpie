package gateway

import (
	"net/http"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// A member the request already rested is not asked again in the same
// request while another can answer: a key whose 429 rested it was asked
// once for each of the group's models that key serves, and heard the same
// refusal each time. Cloudflare Workers AI's 401 was asked twice a
// request that way on 11-07, once for each of two members' models.
func TestRestedSeatIsNotAskedTwiceInOneRequest(t *testing.T) {
	fresh(t)
	limited := &scripted{replies: []reply{{429, "", `{"error":{"message":"Too many requests"}}`}}}
	serveOn(t, "a", "k1", []string{"m", "n"}, limited)
	ok := &scripted{replies: []reply{{200, "", chatOK}}}
	scriptedOn(t, "b", provider.Chat, ok)
	if err := provider.SaveGroup(provider.Group{Name: "G", Members: []string{"a/m", "a/n", "b/m"}, Routing: provider.Ordered}); err != nil {
		t.Fatal(err)
	}
	s := New()
	code, body := postAs(t, s, "", `{"model":"group/g","messages":[{"role":"user","content":"hi"}]}`)
	if code != http.StatusOK {
		t.Fatalf("group request: %d %s", code, body)
	}
	if limited.n != 1 {
		t.Fatalf("the rested key was asked %d times in one request, want 1", limited.n)
	}
}
