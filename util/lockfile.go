package util

import (
	"context"
	"time"

	"github.com/containers/storage/pkg/lockfile"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

type Lockfile struct {
	*lockfile.LockFile
	path string
}

func (lockfile *Lockfile) Unlock() {
	log.Debugf("Released file %s", lockfile.path)
	lockfile.LockFile.Unlock() //nolint:errcheck
}

func AcquireLockfile(ctx context.Context, filename string, maxAttempts int, waitForNextAttempt time.Duration) (*Lockfile, error) {
	var (
		attepmt int
	)

	log.Debugf("Try to lock file %s", filename)
	for {
		if lock, err := lockfile.GetLockFile(filename); err != nil {
			return nil, errors.WithStackTrace(err)
		} else if lock.IsReadWrite() {
			lock.Lock()
			log.Debugf("Locked file %s", filename)
			return &Lockfile{LockFile: lock, path: filename}, nil
		}

		if attepmt >= maxAttempts {
			return nil, errors.Errorf("unable to lock file %q, try removing file manually if you are sure no one terragrunt process is running", filename)
		}
		attepmt++

		log.Debugf("File %q is already locked, next (%d of %d) locking attempt in %v", filename, attepmt, maxAttempts, waitForNextAttempt)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(waitForNextAttempt):
		}
	}
}
