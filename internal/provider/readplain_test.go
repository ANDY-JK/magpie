package provider

import (
	"os"

	"github.com/yetone/magpie/internal/seal"
)

// readPlain is os.ReadFile with the file's seal read off: providers.json and
// logins.json are sealed for the user on Windows (DPAPI), so a test that
// checks their contents — or that they are unchanged — reads them as the JSON
// they hold, not the sealed bytes. Off Windows the seal is the identity, so
// readPlain is os.ReadFile; a file that was written plain (a test's
// writeProviderFile) or sealed (store, writeLogins) is read as its JSON either
// way. err is os.ReadFile's, as-is.
func readPlain(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	return seal.Plain(b), err
}
