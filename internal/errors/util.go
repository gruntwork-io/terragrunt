package errors

import (
	"context"
	"errors"
	"fmt"
	"strings"

	goerrors "github.com/go-errors/errors"
)

// ErrorStack returns an stack trace if available.
func ErrorStack(err error) string {
	var errStacks []string

	for _, err := range UnwrapMultiErrors(err) {
		for {
			if err, ok := err.(interface{ ErrorStack() string }); ok {
				errStacks = append(errStacks, err.ErrorStack())
			}

			if err = errors.Unwrap(err); err == nil {
				break
			}
		}
	}

	return strings.Join(errStacks, "\n")
}

// ContainsStackTrace returns true if the given error contain the stack trace.
// Useful to avoid creating a nested stack trace.
func ContainsStackTrace(err error) bool {
	for _, err := range UnwrapMultiErrors(err) {
		for {
			if err, ok := err.(interface{ ErrorStack() string }); ok && err != nil {
				return true
			}

			if err = errors.Unwrap(err); err == nil {
				break
			}
		}
	}

	return false
}

// IsContextCanceled returns `true` if error has occurred by event `context.Canceled` which is not really an error.
func IsContextCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}

// IsError returns true if actual is the same type of error as expected. This method unwraps the given error objects (if they
// are wrapped in objects with a stacktrace) and then does a simple equality check on them.
func IsError(actual error, expected error) bool {
	return goerrors.Is(actual, expected)
}

// Recover tries to recover from panics, and if it succeeds, calls the given onPanic function with an error that
// explains the cause of the panic. This function should only be called from a defer statement.
func Recover(onPanic func(cause error)) {
	if rec := recover(); rec != nil {
		err, isError := rec.(error)
		if !isError {
			err = fmt.Errorf("%v", rec) //nolint:err113
		}

		onPanic(New(err))
	}
}

// UnwrapMultiErrors unwraps all nested multierrors into error slice.
func UnwrapMultiErrors(err error) []error {
	errs := []error{err}

	for index := 0; index < len(errs); index++ {
		err := errs[index]

		for {
			if err, ok := err.(interface{ Unwrap() []error }); ok {
				errs = append(errs[:index], errs[index+1:]...)
				index--

				errs = append(errs, err.Unwrap()...)

				break
			}

			if err = errors.Unwrap(err); err == nil {
				break
			}
		}
	}

	return errs
}

// UnwrapErrors unwraps all nested multierrors, and errors that were wrapped with `fmt.Errorf("%w", err)`.
func UnwrapErrors(err error) []error {
	var errs []error

	for _, err := range UnwrapMultiErrors(err) {
		for {
			errs = append(errs, err)

			if err = errors.Unwrap(err); err == nil {
				break
			}
		}
	}

	return errs
}
