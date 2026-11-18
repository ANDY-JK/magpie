package gateway

// The process's own resources, read while it runs: what the window's
// GET /api/process answers for a report of magpie holding memory or CPU.
// The profile paths (/v1/magpie/heap, /v1/magpie/cpu) collect, for a
// profile of what the heap or the CPU time is spent on; this only reads
// what the runtime and the system hold now, so a look costs a running
// desktop nothing, and a sampler tick every so often even less.

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/yetone/magpie/internal/usage"
)

// processSampleEvery is how long between the sampler's readings, and
// processSamplesKept how many the ring holds: 960 of 15 seconds is about
// four hours. ReadMemStats stops the world for the read; once every 15
// seconds a desktop process doesn't notice. A reading never touches a
// request: the idle memory release (idleMemory) counts requests alone, so
// a quiet gateway still hands its memory back while the sampler reads.
const (
	processSampleEvery = 15 * time.Second
	processSamplesKept = 960
)

// ProcessSample is one reading of this process, the fields a history chart
// draws. CPUTotal is the seconds of CPU used since the process started
// (processCPU), 0 where the system doesn't say — CPUKnown false on the
// CPU profile says the same.
type ProcessSample struct {
	T            time.Time `json:"t"`
	HeapAlloc    uint64    `json:"heapAlloc"`
	HeapSys      uint64    `json:"heapSys"`
	HeapReleased uint64    `json:"heapReleased"`
	Goroutines   int       `json:"goroutines"`
	CPUTotal     float64   `json:"cpuTotal"`
}

// processRing is the sampler's ring: the last processSamplesKept readings.
// One is written at a time, and a reader takes the whole history as one
// copy under the lock (markOf's pattern), so it never sees half a reading
// or one moving under it.
var processRing struct {
	sync.Mutex
	buf [processSamplesKept]ProcessSample
	n   int // how many of buf are readings, up to the cap
	i   int // where the next reading goes
}

// ringPut takes one reading into the ring, the oldest gone when it is full.
func ringPut(s ProcessSample) {
	processRing.Lock()
	defer processRing.Unlock()
	processRing.buf[processRing.i] = s
	processRing.i++
	if processRing.i == len(processRing.buf) {
		processRing.i = 0
	}
	if processRing.n < len(processRing.buf) {
		processRing.n++
	}
}

// ProcessHistory is the ring's readings, oldest first; empty, never nil,
// until the sampler has ticked.
func ProcessHistory() []ProcessSample {
	processRing.Lock()
	defer processRing.Unlock()
	out := make([]ProcessSample, 0, processRing.n)
	first := processRing.i - processRing.n
	if first < 0 {
		first += len(processRing.buf)
	}
	for k := 0; k < processRing.n; k++ {
		out = append(out, processRing.buf[(first+k)%len(processRing.buf)])
	}
	return out
}

// processStatsOnce keeps the sampler to one per process: the app starts it
// once and it reads for the process's life.
var processStatsOnce sync.Once

// StartProcessStats starts the sampler, once per process: a reading every
// processSampleEvery into the ring. The app calls it as it starts, so a
// window's history is its own from its first minutes, whether or not this
// process serves the gateway; a process that never calls it has none to
// answer with.
func StartProcessStats() {
	processStatsOnce.Do(func() {
		go runProcessStats()
	})
}

// runProcessStats is the sampler's loop: one reading at once, for the
// process's life.
func runProcessStats() {
	sampleProcess()
	t := time.NewTicker(processSampleEvery)
	defer t.Stop()
	for range t.C {
		sampleProcess()
	}
}

// sampleProcess takes one reading of this process into the ring.
func sampleProcess() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s := ProcessSample{
		T:            time.Now(),
		HeapAlloc:    m.HeapAlloc,
		HeapSys:      m.HeapSys,
		HeapReleased: m.HeapReleased,
		Goroutines:   runtime.NumGoroutine(),
	}
	if cpu, ok := processCPU(); ok {
		s.CPUTotal = cpu.Seconds()
	}
	ringPut(s)
}

// ProcessStats is what the runtime and the system say of this process now.
// Its fields are named as the profiles' headers name theirs (HeapStats,
// CPUStats), without the two a collection is for: LiveHeap is the heap
// profile's, and CPU the CPU profile's over its recording. CPUTotal is 0
// with CPUKnown false where the system doesn't say. UsageDrops is how many
// usage records the ledger could not write since start (K-02): a filled
// disk or a read-only folder dropping the account of every call, said in
// the log and shown here so a Usage number that falls short of the calls
// made is not a mystery.
type ProcessStats struct {
	PID          int     `json:"pid"`
	Program      string  `json:"program"`
	Version      string  `json:"version"`
	Window       bool    `json:"window"` // the app's window or magpie web serves the page
	Uptime       string  `json:"uptime"`
	HeapAlloc    uint64  `json:"heapAlloc"`
	HeapInuse    uint64  `json:"heapInuse"`
	HeapIdle     uint64  `json:"heapIdle"`
	HeapReleased uint64  `json:"heapReleased"`
	HeapSys      uint64  `json:"heapSys"`
	StackSys     uint64  `json:"stackSys"`
	Sys          uint64  `json:"sys"`
	NumGC        uint32  `json:"numGC"`
	Goroutines   int     `json:"goroutines"`
	CPUKnown     bool    `json:"cpuKnown"`
	CPUTotal     float64 `json:"cpuTotal"` // seconds of CPU used since the process started
	NumCPU       int     `json:"numCPU"`
	GOMAXPROCS   int     `json:"gomaxprocs"`
	UsageDrops   int64   `json:"usageDrops,omitempty"` // usage records the ledger could not write
}

// ProcessNow reads the process's memory, CPU and goroutines once. Nothing
// is collected: the heap profile runs one for its live heap, this takes
// the values as they are, which is what a look at a running desktop wants.
func ProcessNow() ProcessStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	st := ProcessStats{
		PID:          os.Getpid(),
		Version:      Version,
		Window:       Window,
		Uptime:       time.Since(processStarted).Round(time.Second).String(),
		HeapAlloc:    m.HeapAlloc,
		HeapInuse:    m.HeapInuse,
		HeapIdle:     m.HeapIdle,
		HeapReleased: m.HeapReleased,
		HeapSys:      m.HeapSys,
		StackSys:     m.StackSys,
		Sys:          m.Sys,
		NumGC:        m.NumGC,
		Goroutines:   runtime.NumGoroutine(),
		NumCPU:       runtime.NumCPU(),
		GOMAXPROCS:   runtime.GOMAXPROCS(0),
		UsageDrops:   usage.AppendsFailed(),
	}
	if exe, err := os.Executable(); err == nil {
		st.Program = filepath.Base(exe)
	}
	if cpu, ok := processCPU(); ok {
		st.CPUKnown, st.CPUTotal = true, cpu.Seconds()
	}
	return st
}
