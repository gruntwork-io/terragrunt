package locks

import (
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/gruntwork-io/terragrunt/errors"
	"os"
	"os/signal"
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

	// When Go receives the interrupt signal SIGINT (e.g. from someone hitting CTRL+C), the default behavior is to
	// exit the program immediately. Here, we override that behavior, which ensures our deferred code has a chance
	// to run and release the lock. Note that we don't have to do anything to cancel the running action, as
	// Terraform itself automatically detects SIGINT and does a graceful shutdown in response, so we can just allow
	// the blocking call to action() to return normally.
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt)
	go func() { util.Logger.Printf("Caught signal '%s'. Terraform should be shutting down gracefully now.", <- signalChannel) }()

	return action()
}