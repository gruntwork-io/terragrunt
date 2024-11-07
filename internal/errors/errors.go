// Package errors contains helper functions for wrapping errors with stack traces, stack output, and panic recovery.
package errors

import (
	"fmt"

	goerrors "github.com/go-errors/errors"
)

const (
	newSkip    = 2
	errorfSkip = 2
)

// New creates a new instance of Error.
// If the given value does not contain a stack trace, it will be created.
func New(val any) error {
	if val == nil {
		return nil
	}

	return newWithSkip(newSkip, val)
}

// Errorf creates a new error with the given format and values.
// It can be used as a drop-in replacement for fmt.Errorf() to provide descriptive errors in return values.
// If none of the given values contains a stack trace, it will be created.
func Errorf(format string, vals ...any) error {
	return errorfWithSkip(errorfSkip, format, vals...)
}

func newWithSkip(skip int, val any) error {
	if err, ok := val.(error); ok && ContainsStackTrace(err) {
		return fmt.Errorf("%w", err)
	}

	return goerrors.Wrap(val, skip)
}

func errorfWithSkip(skip int, format string, vals ...any) error {
	err := fmt.Errorf(format, vals...) //nolint:err113

	for _, val := range vals {
		if val, ok := val.(error); ok && val != nil && ContainsStackTrace(val) {
			return err
		}
	}

	return goerrors.Wrap(err, skip)
}
