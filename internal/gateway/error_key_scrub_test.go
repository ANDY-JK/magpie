package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
)

// keyEchoUpstream answers every ask with a vendor's refusal whose message,
// code and param repeat the key the caller was saved with, and another
// channel's key that isn't this provider's.
type keyEchoUpstream struct {
	status int
	body   string
	asks   atomic.Int64
}

func (u *keyEchoUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	io.Copy(io.Discard, r.Body)
	u.asks.Add(1)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(u.status)
	io.WriteString(w, u.body)
}

// A vendor that refuses the key and prints it back in its error text must
// not hand the key to the client or app.log: both read the same message,
// and both must say only its masked form. The status, type and code stay as
// they were, and another channel's key in the text is left alone.
func TestVendorErrorNeverEchoesTheChannelKey(t *testing.T) {
	fresh(t)
	key := "sk-live-1234567890abcdef"
	other := "sk-live-other-0987654321"
	up := &keyEchoUpstream{status: http.StatusUnauthorized, body: `{"error":{"type":"invalid_api_key","code":"invalid_api_key","message":"Incorrect API key provided: ` + key + ` . Authorization: Bearer ` + key + `; this relay also serves ` + other + `"}}`}
	srv := httptest.NewServer(up)
	t.Cleanup(srv.Close)
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Key: key, Chat: srv.URL + "/v1", Models: []string{"m"}}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive("relay", srv.URL+"/v1", []catalog.Model{{ID: "m"}}); err != nil {
		t.Fatal(err)
	}

	s := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions",
		strings.NewReader(`{"model":"relay/m","messages":[{"role":"user","content":"hi"}]}`))
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", rec.Code, rec.Body.String())
	}
	if up.asks.Load() == 0 {
		t.Fatal("the upstream was never asked")
	}
	var got struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("the reply is not the client's error JSON: %v: %s", err, rec.Body.String())
	}
	if got.Error.Type != "authentication_error" || got.Error.Code != "invalid_api_key" {
		t.Errorf("error type/code = %q/%q, want authentication_error/invalid_api_key", got.Error.Type, got.Error.Code)
	}
	if strings.Contains(rec.Body.String(), key) {
		t.Errorf("the channel's key reached the client: %s", rec.Body.String())
	}
	if !strings.Contains(got.Error.Message, provider.Mask(key)) {
		t.Errorf("the message does not hold the masked key %q: %q", provider.Mask(key), got.Error.Message)
	}
	if !strings.Contains(got.Error.Message, other) {
		t.Errorf("another channel's key was replaced in the message: %q", got.Error.Message)
	}

	// app.log's line is built from the recorded call's error, which is the
	// same text the client got.
	calls := s.Recent()
	var call *Call
	for i := range calls {
		if calls[i].Status == http.StatusUnauthorized {
			call = &calls[i]
			break
		}
	}
	if call == nil {
		t.Fatalf("the failed call was not recorded: %+v", calls)
	}
	if strings.Contains(call.Error, key) {
		t.Errorf("app.log's error gives the key away: %q", call.Error)
	}
	if !strings.Contains(call.Error, provider.Mask(key)) {
		t.Errorf("app.log's error does not hold the masked key: %q", call.Error)
	}
	if line, ok := failedCallLine(*call, time.Now()); ok && strings.Contains(line, key) {
		t.Errorf("app.log's line gives the key away: %s", line)
	}
}
