//go:build !linux && !darwin

package vfs

import "os"

// CloneFileIfPossible reports that this platform offers no copy-on-write
// clone, so callers link or copy instead.
func (fsys *osFS) CloneFileIfPossible(oldname, newname string) error {
	return &os.LinkError{Op: cloneOp, Old: oldname, New: newname, Err: ErrNoCloneFile}
}
