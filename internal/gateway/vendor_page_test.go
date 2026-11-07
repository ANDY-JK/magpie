package gateway

import (
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// vendorPage is what fiimeopc served on 11-07 to every key and model: a
// page where its API should be, so the 404 was the endpoint's.
var vendorPage = []byte(`<!DOCTYPE html><html><head><title>OPC 一站式 AI 聚合平台</title></head><body><h1>404 Not Found</h1></body></html>`)

func keyedCandidate(id, key, model string) candidate {
	return candidate{
		p:     provider.Provider{ID: id, Chat: "http://127.0.0.1:1/v1", Key: key},
		model: model,
		rest:  id + "#" + key,
	}
}

// A keyed provider whose endpoint answers with a page where its API should
// be rests as a whole: every key of it is a key of that one endpoint, so
// there is nothing for a second key to find out. Before, each of
// fiimeopc's six keys rested a minute on its own and every group request
// asked six of them for one model, then six more for another (11-07).
func TestVendorPageRestsWholeProvider(t *testing.T) {
	fresh(t)
	s := New()
	a, b := keyedCandidate("fiimeopc", "ka", "deepseek-v4-1-flash-1017"), keyedCandidate("fiimeopc", "kb", "kimi-k3")
	r := s.restAfter(a, http.StatusNotFound, http.Header{}, vendorPage)
	if r.By != "vendor" || r.Failures != 0 || r.Key != "fiimeopc*" {
		t.Fatalf("the page's rest: %+v, want by vendor, no failures, resting as the provider", r)
	}
	if left := time.Until(r.Until); left < vendorRest-time.Second || left > vendorRest+time.Second {
		t.Fatalf("the page rests %v, want about %v", left.Round(time.Second), vendorRest)
	}
	// the provider's other key, asked for another model, is held too
	if _, ok := restOfAny(b); !ok {
		t.Fatal("the provider's other key is asked while its endpoint serves a page")
	}
	// refused again on the other key: the same rest, nothing counted
	r2 := s.restAfter(b, http.StatusNotFound, http.Header{}, vendorPage)
	if r2.By != "vendor" || r2.Failures != 0 || r2.Key != "fiimeopc*" {
		t.Fatalf("the second key's page: %+v, want the provider's rest", r2)
	}
	// an answer from any key lifts it
	if !s.Unrest(a.restKey()) {
		t.Fatal("Try now on the page's key lifted nothing")
	}
	if _, ok := restOfAny(b); ok {
		t.Fatal("Try now on one key left the provider resting")
	}
}

// A vendor's own JSON refusal is its answer, not a page: it stays the
// key's, and a signed-in account's page may be its plan's (zcodeTrialBlocked
// reads those), so it stays the account's.
func TestVendorPageOnlyForAKeyedProvidersEnd(t *testing.T) {
	fresh(t)
	s := New()
	json404 := []byte(`{"error":{"message":"model not found"}}`)
	a := keyedCandidate("elsewhere", "ka", "m")
	if r := s.restAfter(a, http.StatusNotFound, http.Header{}, json404); r.By == "vendor" || r.Key == "elsewhere*" {
		t.Fatalf("a JSON 404 rested the provider: %+v", r)
	}
	acct := candidate{
		p:     provider.Provider{ID: "sub", Chat: "http://127.0.0.1:1/v1", Account: &provider.Account{Agent: "plugin", User: "u1"}},
		model: "m",
		rest:  "sub#u1",
	}
	if r := s.restAfter(acct, http.StatusNotFound, http.Header{}, vendorPage); r.By == "vendor" || r.Key == "sub*" {
		t.Fatalf("an account's page rested the provider: %+v", r)
	}
	if _, ok := restOfAny(keyedCandidate("sub", "ka", "m")); ok {
		t.Fatal("an account's page rested the provider's keys")
	}
}

// pageEndpoint answers every request with the same page, counting them.
type pageEndpoint struct{ asked atomic.Int32 }

func (p *pageEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.asked.Add(1)
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusNotFound)
	w.Write(vendorPage)
}

// One key of a page-serving provider is asked, then the group's next
// member: the second key was planned before the first answered, and is
// left to the provider's rest while another member can be asked.
func TestVendorPageAsksOneKeyThenTheNextMember(t *testing.T) {
	fresh(t)
	vp := &pageEndpoint{}
	serveOn(t, "vp", "ka", []string{"m"}, vp, "kb")
	ok := &scripted{replies: []reply{{200, "", chatOK}}}
	scriptedOn(t, "ok", provider.Chat, ok)
	if err := provider.SaveGroup(provider.Group{Name: "G", Members: []string{"vp/m", "ok/m"}, Routing: provider.Ordered}); err != nil {
		t.Fatal(err)
	}
	s := New()
	code, body := postAs(t, s, "", `{"model":"group/g","messages":[{"role":"user","content":"hi"}]}`)
	if code != http.StatusOK {
		t.Fatalf("group request: %d %s", code, body)
	}
	if n := vp.asked.Load(); n != 1 {
		for _, tr := range s.trace.routes[len(s.trace.routes)-1].Tries {
			t.Logf("try %s %d %s rest=%+v", tr.ID, tr.Status, tr.Error, tr.Rest)
		}
		t.Fatalf("the page's endpoint was asked %d times, want 1", n)
	}
	if _, ok := restOfAny(keyedCandidate("vp", "kb", "m")); !ok {
		t.Fatal("the provider's second key isn't resting after the first met the page")
	}
}
