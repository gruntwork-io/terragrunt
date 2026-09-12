package util

import (
	"errors"
	"fmt"

	"github.com/gofrs/flock"
)

// ErrLockfileHeld is returned by [Lockfile.TryLock] when another holder
// owns the lock.
var ErrLockfileHeld = errors.New("lock file is held")

// Lockfile is an advisory lock shared between Terragrunt processes,
// backed by a flock on a file path.
//
// The file is created on first acquisition and never removed. Unlinking
// it on release would let two holders coexist: a waiter that acquired
// the flock on the old inode just before the unlink, and a newcomer that
// created a fresh file at the same path and locked that one. Leaving
// the file in place keeps every holder contending on a single inode,
// at the cost of one file per lock path.
type Lockfile struct {
	*flock.Flock
}

// NewLockfile returns an unacquired lock on filename.
func NewLockfile(filename string) *Lockfile {
	return &Lockfile{
		Flock: flock.New(filename),
	}
}

// TryLock acquires the lock without blocking and returns
// [ErrLockfileHeld] when another holder owns it.
func (lf *Lockfile) TryLock() error {
	locked, err := lf.Flock.TryLock()
	if err != nil {
		return err
	}

	if !locked {
		return fmt.Errorf("%w: %s", ErrLockfileHeld, lf.Path())
	}

	return nil
}
