//go:build !windows

package seal

// NotWindows is true where the seal is the identity: nothing on the disk
// is hidden from what owns the file already.
const NotWindows = true

// Seal is the identity off Windows: the bytes are what they are, and the
// file's permissions are its protection, as they were before the seal.
func Seal(b []byte) ([]byte, error) { return b, nil }

// Unseal is the identity; a byte not a sealed blob reads as itself.
func Unseal(b []byte) ([]byte, error) { return b, nil }
