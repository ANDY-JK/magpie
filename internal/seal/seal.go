// Package seal protects magpie's own state files against theft by another
// local user of the machine: on Windows one is sealed with the current
// user's credentials (DPAPI), so a file copied to another machine, or read
// by another account, decrypts to nothing; elsewhere the seal is the
// identity and files stay plain bytes.
//
// Sealed bytes carry a short marker first, so a reader that must accept
// both sealed and plain generations (last good) tells them apart without
// ever guessing: plain JSON starts with '{' or '[' and never with the
// marker.
package seal

import "bytes"

// magic heads every sealed blob. A plain file's first byte ('{', '[', a
// BOM) never matches, so Sealed is exact.
var magic = []byte{0x4d, 0x47, 0x50, 0x01}

// Sealed says b is a sealed blob (the marker, then the sealed bytes), as
// opposed to a plain one.
func Sealed(b []byte) bool {
	return len(b) >= len(magic) && bytes.Equal(b[:len(magic)], magic)
}

// Plain gives b's contents: a sealed file unsealed, any other byte for
// byte. The last-good reader of a file that may be sealed or plain uses it
// for validity, either way; a blob that fails to unseal (another user's
// machine) is left as-is and so reads as not plain JSON, never as empty.
func Plain(b []byte) []byte {
	if !Sealed(b) {
		return b
	}
	out, err := Unseal(b)
	if err != nil {
		return b
	}
	return out
}
