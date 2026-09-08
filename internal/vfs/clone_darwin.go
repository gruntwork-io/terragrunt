//go:build darwin

package vfs

import (
	"os"

	"golang.org/x/sys/unix"
)

// CloneFileIfPossible creates newname as a copy-on-write clone of oldname
// with the clonefile syscall, which carries the source's permissions over to
// the new file. APFS supports it; HFS+ and any other volume report
// [ErrNoCloneFile].
//
// A symlink at oldname is cloned as a symlink rather than followed, so the
// caller cannot be handed the content of a file it did not name.
func (fsys *osFS) CloneFileIfPossible(oldname, newname string) error {
	if err := unix.Clonefile(oldname, newname, unix.CLONE_NOFOLLOW); err != nil {
		return &os.LinkError{Op: cloneOp, Old: oldname, New: newname, Err: cloneErr(err)}
	}

	return nil
}
