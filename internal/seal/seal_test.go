package seal

import (
	"bytes"
	"encoding/json"
	"runtime"
	"testing"
)

// TestRoundTrip seals the bytes and reads them back: on this machine the
// original comes back whole; off Windows the seal is the identity, so the
// bytes are the same, and the file on the disk never changed shape.
func TestRoundTrip(t *testing.T) {
	plain := []byte(`{"providers":[{"id":"x","key":"sk-a1b2c3"}]}`)
	sealed, err := Seal(plain)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if runtime.GOOS == "windows" {
		if bytes.Equal(sealed, plain) {
			t.Fatal("on Windows the seal left the bytes as-is")
		}
		if !Sealed(sealed) {
			t.Fatal("the sealed bytes lack the marker")
		}
		if Sealed(plain) {
			t.Fatal("plain JSON read as sealed")
		}
	} else if !bytes.Equal(sealed, plain) {
		t.Fatalf("off Windows the seal is the identity, got %q", sealed)
	}
	back, err := Unseal(sealed)
	if err != nil {
		t.Fatalf("Unseal: %v", err)
	}
	if !bytes.Equal(back, plain) {
		t.Fatalf("round trip: %q", back)
	}
	// the last-good reader accepts a generation in either shape: sealed
	// reads back as the JSON, plain as itself
	if !bytes.Equal(Plain(sealed), plain) {
		t.Fatalf("Plain of the sealed: %q", Plain(sealed))
	}
	if !bytes.Equal(Plain(plain), plain) {
		t.Fatalf("Plain of the plain: %q", Plain(plain))
	}
	// and a sealed blob that is not this machine's user's (another's
	// copy of the file) is unknown, not empty
	if b, err := Unseal(sealed[:len(sealed)-3]); err == nil {
		t.Fatalf("a truncated sealed blob unsealed: %q", b)
	}
}

// TestSealedIsJSONLike says a sealed blob is never mistaken for the JSON a
// last-good check wants, and a JSON is never mistaken for a sealed blob.
func TestSealedIsJSONLike(t *testing.T) {
	if Sealed([]byte("{}")) || Sealed([]byte("[]")) || Sealed([]byte("\xef\xbb\xbf{}")) || Sealed(nil) || Sealed([]byte{0x4d, 0x47}) {
		t.Fatal("plain JSON or a short byte string read as sealed")
	}
	// a sealed blob must unmarshal as something other than a JSON object
	// of its own: the marker, so lastgood's valid says no, and the
	// recovery path is what reads it through Plain
	sealed, err := Seal([]byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		var m map[string]any
		if json.Unmarshal(sealed, &m) == nil {
			t.Fatal("the sealed blob read as JSON: the last-good check would keep it, and a read through the plain path would drop it")
		}
	}
}
