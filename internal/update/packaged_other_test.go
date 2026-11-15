//go:build !windows

package update

import (
	"path/filepath"
	"testing"
)

// dpkg names an arch-specific package's list name:arch.list, and the arch
// after the colon is cut off for the package name. Windows can't hold that
// file name (a colon makes an NTFS stream of it), so the form is read only
// where the file system can write it.
func TestPackagedByDpkgArchSuffix(t *testing.T) {
	root := t.TempDir()
	info := filepath.Join(root, "var/lib/dpkg/info")
	writeFile(t, filepath.Join(info, "magpie:amd64.list"), "/.\n/usr\n/usr/bin\n/usr/bin/magpie\n")
	p := packagedIn(root, "linux", "/usr/bin/magpie")
	if p == nil || p.Manager != "dpkg" || p.Package != "magpie" {
		t.Fatalf("%+v", p)
	}
}
