package vfs

import (
	"os"

	"github.com/spf13/afero"
)

// NoSymlinkFS wraps a filesystem so it refuses to create symbolic links, as
// Windows does for a process without the symlink privilege. It embeds only
// the FS interface, so the optional hard link and copy-on-write clone
// interfaces are unavailable too.
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
