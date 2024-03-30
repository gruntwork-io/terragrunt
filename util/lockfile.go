package util

import (
	"github.com/gofrs/flock"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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
	if !lockfile.Locked() {
		return nil
	}

	log.Tracef("Unlock file %s", lockfile.Path())
	return lockfile.Flock.Unlock() //nolint:errcheck
}

func (lockfile *Lockfile) Lock() error {
	if locked, err := lockfile.Flock.TryLock(); err != nil {
		return errors.WithStackTrace(err)
	} else if !locked {
		return errors.Errorf("unable to lock file %s", lockfile.Path())
	}

	return nil
}

func AcquireLockfile(filename string) (*Lockfile, error) {
	lockfile := NewLockfile(filename)
	err := lockfile.Lock()
	return lockfile, err
}
