package gateway

// Baseline (2026-11-18, Windows 10.0.26300 x64, 16 threads, -benchtime=10x):
// BenchmarkReasoningTiming 41,010 ns/op, 3,325 B/op, 50 allocs/op.

import (
	"testing"
	"time"
)

func BenchmarkReasoningTiming(b *testing.B) {
	ev := []byte(`data: {"choices":[{"delta":{"reasoning_content":"The user wants a file written."}}]}` + "\n\n")
	b.ReportAllocs()
	for b.Loop() {
		f := firstToken{start: time.Now()}
		f.see(ev)
	}
}
