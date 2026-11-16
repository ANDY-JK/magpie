package gateway

import (
	"sync/atomic"
	"testing"
	"time"
)

// resetProcessRing gives a test a ring of its own: the sampler's readings
// are the process's, and a test counts only the ones it put there.
func resetProcessRing() {
	processRing.Lock()
	defer processRing.Unlock()
	processRing.n, processRing.i = 0, 0
	processRing.buf = [processSamplesKept]ProcessSample{}
}

// The sampler's ring holds the last processSamplesKept readings and no
// more: one more than the cap pushes the oldest out, the history is the
// readings oldest first, and an empty ring is an empty history, never nil.
func TestProcessRingKeepsTheLastReadings(t *testing.T) {
	resetProcessRing()
	if h := ProcessHistory(); h == nil || len(h) != 0 {
		t.Fatalf("an empty ring read %d readings (nil: %t), want none", len(h), h == nil)
	}
	base := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	const more = 17
	for i := 0; i < processSamplesKept+more; i++ {
		ringPut(ProcessSample{T: base.Add(time.Duration(i) * time.Second), Goroutines: i + 1, HeapAlloc: uint64(i) * 1024})
	}
	h := ProcessHistory()
	if len(h) != processSamplesKept {
		t.Fatalf("history holds %d readings, want the cap %d", len(h), processSamplesKept)
	}
	if h[0].Goroutines != more+1 || h[0].HeapAlloc != uint64(more)*1024 {
		t.Fatalf("the oldest kept is reading %d (heap %d), want reading %d", h[0].Goroutines, h[0].HeapAlloc, more)
	}
	if last := h[len(h)-1]; last.Goroutines != processSamplesKept+more {
		t.Fatalf("the newest kept is reading %d, want %d", last.Goroutines, processSamplesKept+more)
	}
	for i, s := range h {
		if i > 0 && s.Goroutines != h[i-1].Goroutines+1 {
			t.Fatalf("readings out of order at %d: %d then %d", i, h[i-1].Goroutines, s.Goroutines)
		}
		if s.T.IsZero() {
			t.Fatalf("reading %d has no time", i)
		}
	}
}

// A reading taken of this process is one: a time, the heap as the runtime
// holds it, the goroutines there are, and the CPU the system said, when it
// says any (cpu_other.go's says none).
func TestProcessNowReadsThisProcess(t *testing.T) {
	resetProcessRing()
	st := ProcessNow()
	if st.PID <= 0 || st.Goroutines <= 0 || st.HeapAlloc == 0 || st.Uptime == "" {
		t.Fatalf("a bare reading: %+v", st)
	}
	if st.NumCPU <= 0 || st.GOMAXPROCS <= 0 {
		t.Fatalf("no processors said: %+v", st)
	}
	// Windows and the unixes say the CPU a process used; the systems that
	// don't say so, and CPUKnown false reads as 0, as the CPU profile's
	// header does
	if st.CPUKnown && st.CPUTotal < 0 {
		t.Fatalf("known CPU of %v", st.CPUTotal)
	}
	if !st.CPUKnown && st.CPUTotal != 0 {
		t.Fatalf("unknown CPU read %v", st.CPUTotal)
	}
	sampleProcess()
	h := ProcessHistory()
	if len(h) != 1 {
		t.Fatalf("one reading taken, history holds %d", len(h))
	}
	if s := h[0]; s.T.IsZero() || s.HeapAlloc == 0 || s.Goroutines <= 0 {
		t.Fatalf("a bare sample: %+v", s)
	}
	resetProcessRing()
}

// The sampler's readings never touch a request: the idle memory release
// counts requests alone (releaseIfResting), so sampling while the gateway
// rests leaves the rest alone and the release still comes at its end.
func TestProcessSamplingLeavesTheIdleRestAlone(t *testing.T) {
	fresh(t)
	s := New()
	var released atomic.Int32
	s.idle.rest = 40 * time.Millisecond
	s.idle.release = func() { released.Add(1) }
	t.Cleanup(s.idle.stop)
	s.armRelease(s.idle.restFor())
	for i := 0; i < 5; i++ {
		sampleProcess()
	}
	if n := s.Busy().Requests; n != 0 {
		t.Fatalf("sampling left %d requests in flight", n)
	}
	for deadline := time.Now().Add(10 * time.Second); released.Load() == 0; {
		if time.Now().After(deadline) {
			t.Fatal("the release never came while the sampler read")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if n := released.Load(); n != 1 {
		t.Fatalf("released %d times over one rest, want 1", n)
	}
	s.idle.mu.Lock()
	armed := s.idle.due != nil
	s.idle.mu.Unlock()
	if armed {
		t.Fatal("a release was armed again")
	}
}
