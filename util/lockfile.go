package util

import (
	"context"
	"time"

	"github.com/alexflint/go-filemutex"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

type Lockfile struct {
	lock     *filemutex.FileMutex
	filename string
}

func NewLockfile(filename string) *Lockfile {
	return &Lockfile{
		filename: filename,
	}
}

func (lockfile *Lockfile) Path() string {
	return lockfile.filename
}

func (lockfile *Lockfile) Unlock() {
	log.Tracef("Unlock file %s", lockfile.Path())
	lockfile.lock.Unlock() //nolint:errcheck
}

func (lockfile *Lockfile) Lock(ctx context.Context, maxRetries int, retryDelay time.Duration) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	var repeat int

	log.Tracef("Try to lock file %s", lockfile.Path())
	for {
		lock, err := filemutex.New(lockfile.Path())
		if err != nil {
			return errors.WithStackTrace(err)
		}
		lockfile.lock = lock

		if err := lock.TryLock(); err != nil {
			if err != filemutex.AlreadyLocked {
				return errors.WithStackTrace(err)
			}
		} else {
			log.Tracef("Locked file %s", lockfile.Path())
			return nil
		}

		if repeat >= maxRetries {
			return errors.Errorf("unable to lock file %q, try removing file manually if you are sure no one terragrunt process is running", lockfile.Path())
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
