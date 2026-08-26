package util

import (
	"errors"
	"fmt"
	"os"

	"github.com/gofrs/flock"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

type Lockfile struct {
	*flock.Flock
}

func NewLockfile(filename string) *Lockfile {
	return &Lockfile{
		Flock: flock.New(filename),
	}
}

func (lf *Lockfile) Unlock(fsys vfs.FS) error {
	if err := lf.Flock.Unlock(); err != nil {
		return err
	}

	if err := fsys.Remove(lf.Path()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

func (lf *Lockfile) TryLock() error {
	if locked, err := lf.Flock.TryLock(); err != nil {
		return err
	} else if !locked {
		return fmt.Errorf("unable to lock file %s", lf.Path())
	}

	return nil
}
