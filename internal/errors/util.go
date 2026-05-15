package errors

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"strings"

	goerrors "github.com/go-errors/errors"
)

// ErrorStack returns a stack trace assembled from any wrapped error that
// implements ErrorStack() (the convention used by go-errors). Multiple stacks
// in the unwrap chain are joined with newlines and identical traces are
// deduplicated.
func ErrorStack(err error) string {
	var (
		stacks []string
		seen   = map[string]struct{}{}
	)

	for _, err := range UnwrapMultiErrors(err) {
		for _, unwrapped := range UnwrapErrors(err) {
			errWithStack, ok := unwrapped.(interface{ ErrorStack() string })
			if !ok {
				continue
			}

			s := strings.TrimSpace(errWithStack.ErrorStack())
			if s == "" {
				continue
			}

			if _, dup := seen[s]; dup {
				continue
			}

			seen[s] = struct{}{}
			stacks = append(stacks, s)
		}
	}

	return strings.Join(stacks, "\n")
}

// ContainsStackTrace returns true if the given error contains a stack trace.
func ContainsStackTrace(err error) bool {
	for _, err := range UnwrapMultiErrors(err) {
		for {
			if errWithStack, ok := err.(interface{ ErrorStack() string }); ok && errWithStack.ErrorStack() != "" {
				return true
			}

			err = errors.Unwrap(err)
			if err == nil {
				break
			}
		}
	}

	return false
}

// IsContextCanceled returns `true` if error has occurred by event `context.Canceled`.
func IsContextCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}

// IsError returns true if actual is the same type of error as expected.
func IsError(actual error, expected error) bool {
	return goerrors.Is(actual, expected)
}

// Recover invokes onPanic with an error wrapping the recovered value. The
// goroutine stack from runtime/debug.Stack is appended to the message so
// downstream code can detect panic-origin errors by inspecting the stack
// (e.g. for a runtime.gopanic frame).
//
// Must be invoked as `defer errors.Recover(...)` — wrapping it inside
// another deferred closure makes the internal recover() return nil.
func Recover(onPanic func(cause error)) {
	rec := recover()
	if rec == nil {
		return
	}

	stack := debug.Stack()

	if err, ok := rec.(error); ok {
		onPanic(fmt.Errorf("panic: %w\n\n%s", err, stack))
		return
	}

	onPanic(fmt.Errorf("panic: %v\n\n%s", rec, stack))
}

// UnwrapMultiErrors unwraps all nested multierrors into an error slice.
func UnwrapMultiErrors(err error) []error {
	errs := []error{err}

	for index := 0; index < len(errs); index++ {
		err := errs[index]

		for {
			if err, ok := err.(interface{ Unwrap() []error }); ok {
				errs = slices.Delete(errs, index, index+1)
				index--

				errs = append(errs, err.Unwrap()...)

				break
			}

			err = errors.Unwrap(err)
			if err == nil {
				break
			}
		}
	}

	return errs
}

// UnwrapErrors unwraps all nested multierrors and errors wrapped with fmt.Errorf.
func UnwrapErrors(err error) []error {
	var errs []error

	for _, err := range UnwrapMultiErrors(err) {
		for {
			errs = append(errs, err)

			err = errors.Unwrap(err)
			if err == nil {
				break
			}
		}
	}

	return errs
}
