package util

import (
	"fmt"
	"os"

	"github.com/gofrs/flock"
)

type Lockfile struct {
	*flock.Flock
}

func NewLockfile(filename string) *Lockfile {
	return &Lockfile{
		flock.New(filename),
	}
}

func (lockfile *Lockfile) Unlock() error {
	if lockfile.Flock == nil {
		return nil
	}

	if err := lockfile.Flock.Unlock(); err != nil {
		return err
	}

	if FileExists(lockfile.Path()) {
		if err := os.Remove(lockfile.Path()); err != nil {
			return err
		}
	}

	return nil
}

func (lockfile *Lockfile) TryLock() error {
	if locked, err := lockfile.Flock.TryLock(); err != nil {
		return err
	} else if !locked {
		return fmt.Errorf("unable to lock file %s", lockfile.Path())
	}

	return nil
}
