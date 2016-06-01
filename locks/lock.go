package locks

import (
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/gruntwork-io/terragrunt/errors"
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
func WithLock(lock Lock, action func() error) (finalErr error) {
	if err := lock.AcquireLock(); err != nil {
		return err
	}

	defer func() {
		// We call ReleaseLock in a deferred function so that we release locks even in the case of a panic
		err := lock.ReleaseLock()
		if err != nil {
			// We are using a named return variable so that if ReleaseLock returns an error, we can still
			// return that error from a deferred function. However, if that named return variable is
			// already set, that means the action executed and had an error, so we should return the
			// action's error and only log the ReleaseLock error.
			if finalErr == nil {
				finalErr = err
			} else {
				util.Logger.Printf("ERROR: failed to release lock %s: %s", lock, errors.PrintErrorWithStackTrace(err))
			}
		}
	}()

	return action()
}