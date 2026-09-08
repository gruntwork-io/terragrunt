package vfs

import (
	"os"

	"github.com/spf13/afero"
)

// NoSymlinkFS wraps a filesystem so it refuses to create symbolic links,
// which is what Windows reports to a process without the symlink privilege.
// Embedding the FS interface alone withholds the hard link and the
// copy-on-write clone as well, since a caller reaches those through optional
// interfaces this type no longer satisfies.
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
