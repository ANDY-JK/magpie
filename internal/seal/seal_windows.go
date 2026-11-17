package seal

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Seal wraps b: the marker, then b sealed for this user (DPAPI), so a
// copy of the file on another machine or in another account decrypts to
// nothing, never to the keys.
func Seal(b []byte) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
	var out windows.DataBlob
	err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append(magic, unsafe.Slice(out.Data, out.Size)...), nil
}

// Unseal reverses Seal; a blob that is not this user's fails, so a file
// read from another account is unknown, never empty.
func Unseal(b []byte) ([]byte, error) {
	if !Sealed(b) {
		return b, nil
	}
	in := windows.DataBlob{Size: uint32(len(b) - len(magic)), Data: &b[len(magic)]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
