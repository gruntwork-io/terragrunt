package util

import (
	"context"
	"time"

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

func (lockfile *Lockfile) Unlock() {
	if !lockfile.Locked() {
		return
	}

	log.Tracef("Unlock file %s", lockfile.Path())
	lockfile.Flock.Unlock() //nolint:errcheck
}

func (lockfile *Lockfile) Lock(ctx context.Context, maxRetries int, retryDelay time.Duration) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	var repeat int

	log.Tracef("Try to lock file %s", lockfile.Path())
	for {
		if locked, err := lockfile.Flock.TryLock(); err != nil {
			return errors.WithStackTrace(err)
		} else if locked {
			log.Tracef("Locked file %s", lockfile.Path())
			return nil
		}

		if repeat >= maxRetries {
			return errors.Errorf("unable to lock file %q, try removing file manually", lockfile.Path())
		}
		repeat++
		log.Tracef("File %q is already locked, next (%d of %d) lock attempt in %v", lockfile.Path(), repeat, maxRetries, retryDelay)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryDelay):
			// try again
		}
	}
}

func AcquireLockfile(ctx context.Context, filename string, maxRetries int, retryDelay time.Duration) (*Lockfile, error) {
	lockfile := NewLockfile(filename)
	err := lockfile.Lock(ctx, maxRetries, retryDelay)
	return lockfile, err
}
