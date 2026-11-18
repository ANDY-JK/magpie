//go:build windows

package usage

import (
	"os"

	"golang.org/x/sys/windows"
)

// openAppendLog opens path to append to, sharing it for deleting as well as
// reading and writing: a rotation away or a removal by another program is not
// blocked by it. FILE_APPEND_DATA is O_APPEND's access without
// FILE_WRITE_DATA, so every write lands at the file's end.
func openAppendLog(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(
		p,
		windows.FILE_GENERIC_WRITE&^windows.FILE_WRITE_DATA,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}

// holdLogBetweenBatches is false: another program cannot rename a file over
// a path a handle holds open on Windows (a rename away is allowed), so the
// writer closes the log after each batch; a batch's records still share one
// open rather than opening once each.
const holdLogBetweenBatches = false

// openLogRead opens path for reading, sharing it for deleting as well, so a
// reader's handle does not block the writer's rotation.
func openLogRead(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(
		p,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}
