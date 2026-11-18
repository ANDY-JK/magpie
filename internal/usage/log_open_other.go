//go:build !windows

package usage

import "os"

// openAppendLog opens path to append to: a rename or removal by another
// program is allowed while this process holds it open.
func openAppendLog(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}

// holdLogBetweenBatches is true: the log stays open between batches, as a
// rename or removal is not blocked by the handle here.
const holdLogBetweenBatches = true

// openLogRead opens path for reading; a rotation is not blocked by it.
func openLogRead(path string) (*os.File, error) {
	return os.Open(path)
}
