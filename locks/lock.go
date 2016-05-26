package locks

import (
	"github.com/gruntwork-io/terragrunt/util"
	"sync/atomic"
)

// Every type of lock must implement this interface
type Lock interface {
	// Acquire a lock
	AcquireLock() 		error

	// Release a lock
	ReleaseLock() 		error

	// Print a string representation of the lock
	String()      		string
}

// Acquire a lock, execute the given function, and release the lock
func WithLock(lock Lock, action func() error) error {
	var unlockCount = int32(0)

	if err := lock.AcquireLock(); err != nil {
		return err
	}

	// This function uses an atomic compare-and-swap operation on an int to ensure that we try to release the lock
	// only once
	tryToReleaseLock := func(logError bool) error {
		if atomic.CompareAndSwapInt32(&unlockCount, 0, 1) {
			err := lock.ReleaseLock()
			if err != nil && logError {
				util.Logger.Printf("ERROR: failed to release lock %s: %s", lock, err.Error())
			}
			return err
		}
		return nil
	}

	defer func() {
		// Make sure to release the lock in case a panic occurred. Log any errors that happen while releasing
		// the lock, as it's too late to return those errors.
		if r := recover(); r != nil {
			util.Logger.Printf("Recovered from panic: %s", r)
			tryToReleaseLock(true)
		}
	}()

	actionErr := action()
	if actionErr == nil {
		// The action completed successfully. Try to release the lock. Since we return the error from releasing
		// the lock, we don't need to log it here.
		return tryToReleaseLock(false)
	} else {
		// We already have an error from the action. Since we want to return that one, we'll try to release the
		// lock and merely log (rather than return) any errors that happen while releasing.
		tryToReleaseLock(true)
		return actionErr
	}
}