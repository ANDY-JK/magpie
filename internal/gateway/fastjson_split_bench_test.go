package gateway

// Baseline (2026-11-18, Windows 10.0.26300 x64, 16 threads, -benchtime=20x):
// BenchmarkDecodeReaders 64,580 ns/op, 3,098.76 MB/s, 266 B/op, 6 allocs/op.
// A change far off it is the difference; re-record when the decode path changes.

import (
	"strings"
	"testing"
)

// BenchmarkDecodeReaders is a request's readers decoding one 200KB body,
// each into the few members it wants: the model and stream flag, the
// metadata, the max tokens.
func BenchmarkDecodeReaders(b *testing.B) {
	body := []byte(`{"model":"claude-x","stream":true,"max_tokens":9,"messages":[{"role":"user","content":"` +
		strings.Repeat("a long prompt with \\\"quotes\\\" and words ", 5000) + `"}],"metadata":{"user_id":"u"}}`)
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for b.Loop() {
		var a struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		var e struct {
			Metadata struct {
				UserID string `json:"user_id"`
			} `json:"metadata"`
		}
		var m struct {
			Max int `json:"max_tokens"`
		}
		for _, dst := range []any{&a, &e, &m} {
			if err := decodeJSON(body, dst); err != nil {
				b.Fatal(err)
			}
		}
	}
}
