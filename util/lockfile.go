package util

import (
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

// func (lockfile *Lockfile) Unlock() {
// 	if !lockfile.Locked() {
// 		return
// 	}

// 	log.Tracef("Unlock file %s", lockfile.Path())
// 	lockfile.Flock.Unlock() //nolint:errcheck
// }

// func (lockfile *Lockfile) Lock(ctx context.Context, retryDelay time.Duration) error {
// 	var attepmt int

// 	log.Tracef("Try to lock file %s", lockfile.Path())
// 	for {
// 		if locked, err := lockfile.Flock.TryLockContext(ctx, retryDelay); err != nil {
// 			return errors.WithStackTrace(err)
// 		} else if locked {
// 			log.Tracef("Locked file %s", lockfile.Path())
// 			return nil
// 		}

// 		if attepmt >= maxAttempts {
// 			return errors.Errorf("unable to lock file %q, try removing file manually if you are sure no one terragrunt process is running", lockfile.Path())
// 		}
// 		attepmt++
// 		log.Tracef("File %q is already locked, next (%d of %d) locking attempt in %v", lockfile.Path(), attepmt, maxAttempts, waitForNextAttempt)

// 		select {
// 		case <-ctx.Done():
// 			return ctx.Err()
// 		case <-time.After(waitForNextAttempt):
// 		}
// 	}
// }

// func AcquireLockfile(ctx context.Context, filename string, maxAttempts int, waitForNextAttempt time.Duration) (*Lockfile, error) {
// 	lockfile := NewLockfile(filename)
// 	err := lockfile.Lock(ctx, maxAttempts, waitForNextAttempt)
// 	return lockfile, err
// }
