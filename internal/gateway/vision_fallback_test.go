package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// twoEyed is eyed with a second provider whose model sees, other/bigeye,
// on an upstream of its own; vision names the Image recognition model
// picked in Settings ("" for magpie's own pick).
func twoEyed(t *testing.T, vision string) (*Server, *eyeUp, *eyeUp) {
	t.Helper()
	s, u := eyed(t)
	u2 := &eyeUp{}
	srv := httptest.NewServer(u2)
	t.Cleanup(srv.Close)
	if err := provider.Save(provider.Provider{ID: "other", Name: "Other", Chat: srv.URL + "/v1", Key: "key", Models: []string{"bigeye"}}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive("other", srv.URL+"/v1", []catalog.Model{{ID: "bigeye", Images: true, ImageInput: imageInputBool(true)}}); err != nil {
		t.Fatal(err)
	}
	st := settings.Load()
	st.Vision = vision
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	return s, u, u2
}

func failing(u *eyeUp, fail bool) {
	u.mu.Lock()
	u.fail = fail
	u.mu.Unlock()
}

// the Routing row of the description model answered
func visionRow(t *testing.T, s *Server, model string) *CallFor {
	t.Helper()
	for _, r := range s.Trace(context.Background(), 0, 0).Routes {
		if r.Kind == "vision" && r.Model == model && r.Status == 200 {
			return r.For
		}
	}
	t.Fatalf("no answered vision row for %s in %+v", model, s.Trace(context.Background(), 0, 0).Routes)
	return nil
}

// kelvinjason11 on X: "图片识别好像只支持一个模型识别 没有fallback". The
// Image recognition model picked in Settings failing (a 500) failed the
// turn, though another model that sees was enabled. It is described by
// that one instead; the pick is still asked first for each new image.
func TestPickedDescriberThatFailsGivesWayToTheNextThatSees(t *testing.T) {
	s, u, u2 := twoEyed(t, "probe/eye")
	failing(u, true)
	code, body := postAs(t, s, "", pastedImage)
	if code != 200 {
		t.Fatalf("picked describer down: %d %s", code, body)
	}
	if got := u.sent("text"); len(got) != 1 || !strings.Contains(got[0], "HELLO") || !strings.Contains(got[0], "other/bigeye describes it") {
		t.Fatalf("text model was sent %v", got)
	}
	// routing tries a 500 again on its own before the description fails
	asked := len(u.sent("eye"))
	if asked == 0 || len(u2.sent("bigeye")) != 1 {
		t.Fatalf("eye asked %d times, bigeye %d", asked, len(u2.sent("bigeye")))
	}
	f := visionRow(t, s, "other/bigeye")
	if f == nil || f.Instead != "probe/eye" || !f.Picked || !strings.Contains(f.Why, "eye is down") || f.Model != "probe/text" {
		t.Fatalf("the answering description's row is for %+v", f)
	}
	// the same image sent again is described already, by whichever model
	if code, body := postAs(t, s, "", pastedImage); code != 200 || len(u.sent("eye")) != asked || len(u2.sent("bigeye")) != 1 {
		t.Fatalf("image sent again: %d %s; eye asked %d, bigeye %d", code, body, len(u.sent("eye")), len(u2.sent("bigeye")))
	}
	// a new image asks the pick first again, and it answers once it is back
	failing(u, false)
	if code, body := postAs(t, s, "", strings.Replace(pastedImage, "aGVsbG8=", "d29ybGQ=", 1)); code != 200 || len(u.sent("eye")) != asked+1 || len(u2.sent("bigeye")) != 1 {
		t.Fatalf("new image: %d %s; eye asked %d, bigeye %d", code, body, len(u.sent("eye")), len(u2.sent("bigeye")))
	}
}

// magpie's own pick failing gives way to its next one too.
func TestAutoDescriberThatFailsGivesWayToTheNext(t *testing.T) {
	s, u, u2 := twoEyed(t, "")
	first, next, down := AutoVision(), "other/bigeye", u
	if first == "other/bigeye" {
		next, down = "probe/eye", u2
	}
	failing(down, true)
	if code, body := postAs(t, s, "", pastedImage); code != 200 {
		t.Fatalf("auto describer down: %d %s", code, body)
	}
	if got := u.sent("text"); len(got) != 1 || !strings.Contains(got[0], next+" describes it") {
		t.Fatalf("text model was sent %v", got)
	}
	if f := visionRow(t, s, next); f == nil || f.Instead != first || f.Picked {
		t.Fatalf("the answering description's row is for %+v", f)
	}
}

// askedOf is how many descriptions u's models were asked.
func askedOf(u *eyeUp) int { return len(u.sent("eye")) + len(u.sent("bigeye")) }

// holdingEye is eyeUp whose model that sees holds a description until its
// request ends while hold is set, saying when one arrived.
type holdingEye struct {
	eyeUp
	arrived chan struct{}
	hmu     sync.Mutex
	hold    bool
}

func (u *holdingEye) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	u.hmu.Lock()
	hold := u.hold && strings.HasSuffix(modelOf(body), "eye")
	u.hmu.Unlock()
	if hold {
		u.arrived <- struct{}{}
		<-r.Context().Done()
		return
	}
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	u.eyeUp.ServeHTTP(w, r)
}

func (u *holdingEye) holding(on bool) {
	u.hmu.Lock()
	u.hold = on
	u.hmu.Unlock()
}

// A describer magpie picked itself whose description never came — the
// agent gave up on the request, or magpie gave it up for — is not picked
// for the next image while another that hasn't failed is there. A model
// that takes an image and doesn't answer it had every request held for it
// and every retry asked it again (a qoder Coding Plan's qfmodel: 2436
// descriptions in a day at ~41 seconds each, on the two accounts the
// user's own sessions use, while a model that answered described one in
// 7 seconds).
func TestADescriptionThatNeverCameMovesThePickOn(t *testing.T) {
	fresh(t)
	u := &holdingEye{arrived: make(chan struct{}, 1), hold: true}
	up := httptest.NewServer(u)
	t.Cleanup(up.Close)
	if err := provider.Save(provider.Provider{ID: "probe", Name: "Probe", Chat: up.URL + "/v1", Key: "key", Models: []string{"text", "eye"}}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive("probe", up.URL+"/v1", []catalog.Model{
		{ID: "text", ImageInput: imageInputBool(false)},
		{ID: "eye", Images: true, ImageInput: imageInputBool(true)},
	}); err != nil {
		t.Fatal(err)
	}
	u2 := &eyeUp{}
	srv := httptest.NewServer(u2)
	t.Cleanup(srv.Close)
	if err := provider.Save(provider.Provider{ID: "other", Name: "Other", Chat: srv.URL + "/v1", Key: "key", Models: []string{"bigeye"}}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive("other", srv.URL+"/v1", []catalog.Model{{ID: "bigeye", Images: true, ImageInput: imageInputBool(true)}}); err != nil {
		t.Fatal(err)
	}
	s := New()
	if first := AutoVision(); first != "probe/eye" {
		t.Fatalf("AutoVision = %q, want probe/eye", first)
	}
	// a request held on its image until its agent cancels it: what the
	// pick was asked never came
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(pastedImage)).WithContext(ctx))
		done <- rec.Code
	}()
	select {
	case <-u.arrived:
	case <-time.After(10 * time.Second):
		t.Fatal("the pick was never asked")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the canceled request never ended")
	}
	// the description is counted against it once its own work has ended
	deadline := time.Now().Add(10 * time.Second)
	for AutoVision() != "other/bigeye" {
		if time.Now().After(deadline) {
			t.Fatalf("after a description that never came, AutoVision = %q, want other/bigeye", AutoVision())
		}
		time.Sleep(10 * time.Millisecond)
	}
	// the next image is asked of the other one, none of the pick
	u.holding(false)
	asked := askedOf(&u.eyeUp)
	if code, body := postAs(t, s, "", strings.Replace(pastedImage, "aGVsbG8=", "d29ybGQ=", 1)); code != 200 {
		t.Fatalf("next image: %d %s", code, body)
	}
	if got := askedOf(&u.eyeUp); got != asked {
		t.Fatalf("probe/eye, whose description never came, was asked for the next image %d times", got-asked)
	}
	if got := u2.sent("bigeye"); len(got) != 1 || !strings.Contains(got[0], "image_url") {
		t.Fatalf("other/bigeye was asked %d times", len(got))
	}
}

// A pick that missed comes back once it describes an image, and after
// seerMissFor passes. The Settings pick is asked first however it ranks,
// and the only model that sees is still named with every one missed.
func TestAMissedPickComesBack(t *testing.T) {
	s, u, u2 := twoEyed(t, "")
	first, next, down := AutoVision(), "other/bigeye", u
	if first == "other/bigeye" {
		next, down = "probe/eye", u2
	}
	missedDescribing(first, time.Now())
	if got := AutoVision(); got != next {
		t.Fatalf("with %s missed, AutoVision = %q, want %q", first, got, next)
	}
	// the Settings pick is asked first all the same, and describing one
	// clears it
	st := settings.Load()
	st.Vision = first
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	if code, body := postAs(t, s, "", strings.Replace(pastedImage, "aGVsbG8=", "d29ybGQ=", 1)); code != 200 {
		t.Fatalf("the Settings pick: %d %s", code, body)
	}
	if got := askedOf(down); got != 1 {
		t.Fatalf("the Settings pick %s was asked %d times, want 1", first, got)
	}
	st = settings.Load()
	st.Vision = ""
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	if got := AutoVision(); got != first {
		t.Fatalf("after describing an image, AutoVision = %q, want %q", got, first)
	}
	// gone once seerMissFor passes
	missedDescribing(first, time.Now())
	if got := AutoVision(); got != next {
		t.Fatalf("with %s missed again, AutoVision = %q, want %q", first, got, next)
	}
	seerMiss.mu.Lock()
	seerMiss.m[first] = time.Now().Add(-seerMissFor - time.Second)
	seerMiss.mu.Unlock()
	if got := AutoVision(); got != first {
		t.Fatalf("after seerMissFor, AutoVision = %q, want %q", got, first)
	}
	// and every one missed names the first again: routing decides between
	// refusals then
	missedDescribing(first, time.Now())
	missedDescribing(next, time.Now())
	if got := AutoVision(); got != first {
		t.Fatalf("with every describer missed, AutoVision = %q, want %q", got, first)
	}
}

// Every model that sees failing: the error names each one and why.
func TestNoDescriberAnswersNamesEach(t *testing.T) {
	s, u, u2 := twoEyed(t, "probe/eye")
	failing(u, true)
	failing(u2, true)
	code, body := postAs(t, s, "", pastedImage)
	if code != 502 || !strings.Contains(body, "probe/eye couldn't describe the image for it: Probe: eye is down") || !strings.Contains(body, "nor could other/bigeye, asked in its place: Other: eye is down") {
		t.Fatalf("every describer down: %d %s", code, body)
	}
	if len(u.sent("text")) != 0 || len(u.sent("eye")) == 0 || len(u2.sent("bigeye")) == 0 {
		t.Fatalf("text sent %d, eye asked %d, bigeye %d", len(u.sent("text")), len(u.sent("eye")), len(u2.sent("bigeye")))
	}
}

// The picked model answering is all that is asked.
func TestPickedDescriberThatAnswersIsTheOnlyOneAsked(t *testing.T) {
	s, u, u2 := twoEyed(t, "probe/eye")
	if code, body := postAs(t, s, "", pastedImage); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if len(u.sent("eye")) != 1 || len(u2.sent("bigeye")) != 0 {
		t.Fatalf("eye asked %d times, bigeye %d", len(u.sent("eye")), len(u2.sent("bigeye")))
	}
	if got := u.sent("text"); len(got) != 1 || !strings.Contains(got[0], "probe/eye describes it") {
		t.Fatalf("text model was sent %v", got)
	}
	if f := visionRow(t, s, "probe/eye"); f == nil || f.Instead != "" {
		t.Fatalf("the description's row is for %+v", f)
	}
}

// heldEye holds a description until its request ends, saying when one
// has arrived.
type heldEye struct {
	eyeUp
	arrived chan struct{}
}

func (u *heldEye) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if strings.HasSuffix(modelOf(body), "eye") {
		u.arrived <- struct{}{}
		<-r.Context().Done()
		return
	}
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	u.eyeUp.ServeHTTP(w, r)
}

// A request its agent canceled asks no other model to describe its image.
func TestCanceledDescriptionAsksNoOther(t *testing.T) {
	fresh(t)
	u := &heldEye{arrived: make(chan struct{}, 1)}
	up := httptest.NewServer(u)
	t.Cleanup(up.Close)
	if err := provider.Save(provider.Provider{ID: "probe", Name: "Probe", Chat: up.URL + "/v1", Key: "key", Models: []string{"text", "eye"}}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive("probe", up.URL+"/v1", []catalog.Model{
		{ID: "text", ImageInput: imageInputBool(false)},
		{ID: "eye", Images: true, ImageInput: imageInputBool(true)},
	}); err != nil {
		t.Fatal(err)
	}
	u2 := &eyeUp{}
	srv := httptest.NewServer(u2)
	t.Cleanup(srv.Close)
	if err := provider.Save(provider.Provider{ID: "other", Name: "Other", Chat: srv.URL + "/v1", Key: "key", Models: []string{"bigeye"}}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive("other", srv.URL+"/v1", []catalog.Model{{ID: "bigeye", Images: true, ImageInput: imageInputBool(true)}}); err != nil {
		t.Fatal(err)
	}
	st := settings.Load()
	st.Vision = "probe/eye"
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	s := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(pastedImage)).WithContext(ctx))
		done <- rec.Code
	}()
	select {
	case <-u.arrived:
	case <-time.After(10 * time.Second):
		t.Fatal("the picked describer was never asked")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the canceled request never ended")
	}
	if n := len(u2.sent("bigeye")); n != 0 {
		t.Fatalf("a canceled request asked bigeye %d times", n)
	}
	// not even begun: a try on the canceled context is a row of its own
	for _, r := range s.Trace(context.Background(), 0, 0).Routes {
		if r.Kind == "vision" && r.Model == "other/bigeye" {
			t.Fatalf("a canceled request had bigeye describe: %+v", r)
		}
	}
	if n := len(u.sent("text")); n != 0 {
		t.Fatalf("a canceled request reached the text model %d times", n)
	}
}
