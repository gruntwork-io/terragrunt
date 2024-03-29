package util

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

var lockfiles []*Lockfile

type Lockfile struct {
	*flock.Flock
}

func (lockfile *Lockfile) Unlock() {
	if !lockfile.Locked() {
		return
	}

	log.Tracef("Unlock file %s", lockfile.Path())
	lockfile.Flock.Unlock() //nolint:errcheck
}

func AcquireLockfile(ctx context.Context, filename string, maxAttempts int, waitForNextAttempt time.Duration) (*Lockfile, error) {
	var (
		attepmt  int
		fileLock = flock.New(filename)
	)

	if err := os.MkdirAll(filepath.Dir(filename), os.ModePerm); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	log.Tracef("Try to lock file %s", filename)
	for {
		if locked, err := fileLock.TryLock(); err != nil {
			return nil, errors.WithStackTrace(err)
		} else if locked {
			log.Tracef("Locked file %s", filename)
			lockfile := &Lockfile{fileLock}
			lockfiles = append(lockfiles, lockfile)
			return lockfile, nil
		}

		if attepmt >= maxAttempts {
			return nil, errors.Errorf("unable to lock file %q, try removing file manually if you are sure no one terragrunt process is running", filename)
		}
		attepmt++
		log.Tracef("File %q is already locked, next (%d of %d) locking attempt in %v", filename, attepmt, maxAttempts, waitForNextAttempt)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(waitForNextAttempt):
		}
	}
}

func UnlockAllLockfiles() {
	for _, lockfile := range lockfiles {
		lockfile.Unlock()
	}
}
