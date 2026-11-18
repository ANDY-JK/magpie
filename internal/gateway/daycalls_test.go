package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

const dcPath = "/v1/chat/completions"

const dcBody = `{"model":"dc/m","messages":[{"role":"user","content":"hi"}]}`

// dayCallsGateway is a gateway with a provider "dc" of one key whose
// DayCalls is limit, on a fresh config folder.
func dayCallsGateway(t *testing.T, limit int) (*Server, string) {
	t.Helper()
	fresh(t)
	up := httptest.NewServer(&minuteVendor{})
	t.Cleanup(up.Close)
	if err := provider.Save(provider.Provider{ID: "dc", Name: "Daily", Key: "k", Models: []string{"m"}, Chat: up.URL + "/v1", DayCalls: limit}); err != nil {
		t.Fatal(err)
	}
	s := New()
	gw := httptest.NewServer(lanGuard(s.Handler()))
	t.Cleanup(gw.Close)
	return s, gw.URL
}

// holdDay holds the gateway's day at now for the test, and gives it back
// after.
func holdDay(t *testing.T, now time.Time) *time.Time {
	t.Helper()
	old := dayNow
	dayNow = func() time.Time { return now }
	t.Cleanup(func() { dayNow = old })
	return &now
}

// A provider with a DayCalls of 1 takes the first request, turns the next
// away with a 429 whose Retry-After is the next midnight on this computer's
// clock, and takes requests again once the day turns.
func TestDayCallsTurnedAwayUntilMidnight(t *testing.T) {
	now := holdDay(t, time.Date(2026, 11, 18, 22, 0, 0, 0, time.Local))
	_, gw := dayCallsGateway(t, 1)

	if r := rpmAnswered(t, "the first", askRPM(context.Background(), gw, dcPath, dcBody)); r.err != nil || r.code != http.StatusOK {
		t.Fatalf("the first: %v %d %s", r.err, r.code, r.body)
	}
	r := rpmAnswered(t, "the second, to be turned away,", askRPM(context.Background(), gw, dcPath, dcBody))
	if r.err != nil || r.code != http.StatusTooManyRequests {
		t.Fatalf("the second: %v %d %s", r.err, r.code, r.body)
	}
	if got := r.header.Get("Retry-After"); got != "7200" {
		t.Fatalf("Retry-After %q, want 7200 (22:00 to the next midnight)", got)
	}
	if !strings.Contains(r.body, "requests a day") {
		t.Fatalf("the refusal doesn't say why: %s", r.body)
	}

	// the day turns: the old count is dropped and the provider takes requests
	*now = time.Date(2026, 11, 19, 0, 0, 1, 0, time.Local)
	if r := rpmAnswered(t, "the next day's first", askRPM(context.Background(), gw, dcPath, dcBody)); r.err != nil || r.code != http.StatusOK {
		t.Fatalf("the next day: %v %d %s", r.err, r.code, r.body)
	}
}

// The count is kept in daycalls.json, so a restarted gateway still knows how
// many requests the provider has taken today.
func TestDayCallsKeptAcrossRestart(t *testing.T) {
	holdDay(t, time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local))
	_, gw := dayCallsGateway(t, 2)
	if r := rpmAnswered(t, "the first", askRPM(context.Background(), gw, dcPath, dcBody)); r.err != nil || r.code != http.StatusOK {
		t.Fatalf("the first: %v %d %s", r.err, r.code, r.body)
	}

	// a new server over the same config folder reads the count back
	s := New()
	restarted := httptest.NewServer(lanGuard(s.Handler()))
	t.Cleanup(restarted.Close)
	if r := rpmAnswered(t, "the second after a restart", askRPM(context.Background(), restarted.URL, dcPath, dcBody)); r.err != nil || r.code != http.StatusOK {
		t.Fatalf("the second after a restart: %v %d %s", r.err, r.code, r.body)
	}
	if r := rpmAnswered(t, "the third after a restart, to be turned away,", askRPM(context.Background(), restarted.URL, dcPath, dcBody)); r.err != nil || r.code != http.StatusTooManyRequests {
		t.Fatalf("the third after a restart: %v %d %s", r.err, r.code, r.body)
	}
}
