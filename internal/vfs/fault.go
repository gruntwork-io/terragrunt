package vfs

import (
	"os"

	"github.com/spf13/afero"
)

// NoSymlinkFS wraps an afero.Fs and implements the afero.Linker interface,
// but always returns an error on symlink operations. This simulates a system
// where symlinking is not permitted, such as Windows without developer mode.
type NoSymlinkFS struct {
	FS
}

// SymlinkIfPossible always returns a LinkError, simulating a filesystem
// that does not support symlinks.
func (fs *NoSymlinkFS) SymlinkIfPossible(oldname, newname string) error {
	return &os.LinkError{
		Op:  "symlink",
		Old: oldname,
		New: newname,
		Err: afero.ErrNoSymlink,
	}
}
