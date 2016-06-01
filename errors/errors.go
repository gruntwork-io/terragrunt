package errors

import (
	goerrors "github.com/go-errors/errors"
	"fmt"
)

// Wrap the given error in an Error type that contains the stack trace. If the given error already has a stack trace,
// it is used directly. If the given error is nil, return nil.
func WithStackTrace(err error) error {
	if err == nil {
		return nil
	}

	return goerrors.Wrap(err, 1)
}

// Wrap the given error in an Error type that contains the stack trace and has the given message prepended as part of
// the error message. If the given error already has a stack trace, it is used directly. If the given error is nil,
// return nil.
func WithStackTraceAndPrefix(err error, message string, args ... interface{}) error {
	if err == nil {
		return nil
	}

	return goerrors.WrapPrefix(err, fmt.Sprintf(message, args...), 1)
}

// Returns true if actual is the same type of error as expected
func IsError(actual error, expected error) bool {
	return goerrors.Is(actual, expected)
}

// If the given error is a wrapper that contains a stacktrace, unwrap it and return the original, underlying error.
// In all other cases, return the error unchanged
func Unwrap(err error) error {
	if err == nil {
		return nil
	}

	goError, isGoError := err.(*goerrors.Error)
	if isGoError {
		return goError.Err
	}

	return err
}

// Convert the given error to a string, including the stack trace if available
func PrintErrorWithStackTrace(err error) string {
	if err == nil {
		return ""
	}

	switch underlyingErr := err.(type) {
	case *goerrors.Error: return underlyingErr.ErrorStack()
	default: return err.Error()
	}
}