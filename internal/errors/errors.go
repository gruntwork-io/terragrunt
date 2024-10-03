// Package errors contains helper functions for wrapping errors with stack traces, stack output, and panic recovery.
package errors

import (
	"errors"
	"fmt"

	goerrors "github.com/go-errors/errors"
	"github.com/urfave/cli/v2"
)

// Errorf creates a new error and wraps in an Error type that contains the stack trace.
func Errorf(message string, args ...interface{}) error {
	err := fmt.Errorf(message, args...)
	return goerrors.Wrap(err, 1)
}

// ErrorWithExitCode is a custom error that is used to specify the app exit code.
type ErrorWithExitCode struct {
	Err      error
	ExitCode int
}

func (err ErrorWithExitCode) Error() string {
	return err.Err.Error()
}

// WithStackTrace wraps the given error in an Error type that contains the stack trace. If the given error already has a stack trace,
// it is used directly. If the given error is nil, return nil.
func WithStackTrace(err error) error {
	if err == nil {
		return nil
	}

	return goerrors.Wrap(err, 1)
}

// WithStackTraceAndPrefix wraps the given error in an Error type that contains the stack trace and has the given message prepended as part of
// the error message. If the given error already has a stack trace, it is used directly. If the given error is nil,
// return nil.
func WithStackTraceAndPrefix(err error, message string, args ...interface{}) error {
	if err == nil {
		return nil
	}

	return goerrors.WrapPrefix(err, fmt.Sprintf(message, args...), 1)
}

// IsError returns true if actual is the same type of error as expected. This method unwraps the given error objects (if they
// are wrapped in objects with a stacktrace) and then does a simple equality check on them.
func IsError(actual error, expected error) bool {
	return goerrors.Is(actual, expected)
}

// ErrorWithStackTrace returns a string that contains both the error message and the callstack.
func ErrorWithStackTrace(err error) string {
	if err == nil {
		return ""
	}

	return goError(err).ErrorStack()
}

// StackTrace returns the callstack formatted the same way that go does in runtime/debug.Stack().
func StackTrace(err error) string {
	if err == nil {
		return ""
	}

	return string(goError(err).Stack())
}

func goError(err error) *goerrors.Error {
	if err == nil {
		return nil
	}

	goerr := &goerrors.Error{Err: err}

	for {
		if goError := new(goerrors.Error); errors.As(err, &goError) {
			goerr = goError
		}

		if err = errors.Unwrap(err); err == nil {
			break
		}
	}

	return goerr
}

// Recover tries to recover from panics, and if it succeeds, calls the given onPanic function with an error that
// explains the cause of the panic. This function should only be called from a defer statement.
func Recover(onPanic func(cause error)) {
	if rec := recover(); rec != nil {
		err, isError := rec.(error)
		if !isError {
			err = fmt.Errorf("%v", rec)
		}

		onPanic(WithStackTrace(err))
	}
}

// WithPanicHandling wraps every command you add to *cli.App to handle panics by logging them with a stack trace and returning
// an error up the chain.
func WithPanicHandling(action func(c *cli.Context) error) func(c *cli.Context) error {
	return func(context *cli.Context) (err error) {
		defer Recover(func(cause error) {
			err = cause
		})

		return action(context)
	}
}
