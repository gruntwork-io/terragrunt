//go:build !linux && !darwin

package vfs

import "os"

// CloneFileIfPossible always returns an error wrapping [ErrNoCloneFile].
func (fsys *osFS) CloneFileIfPossible(oldname, newname string) error {
	return &os.LinkError{Op: cloneOp, Old: oldname, New: newname, Err: ErrNoCloneFile}
}
