package errors

import (
	"context"
	"errors"
	"slices"
	"strings"

	goerrors "github.com/go-errors/errors"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty/function"
)

// ErrorStack returns a stack trace if available, deduplicating identical
// frames pulled from different layers of the unwrap chain.
func ErrorStack(err error) string {
	var (
		stacks []string
		seen   = map[string]struct{}{}
	)

	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}

		if _, ok := seen[s]; ok {
			return
		}

		seen[s] = struct{}{}
		stacks = append(stacks, s)
	}

	for _, err := range UnwrapMultiErrors(err) {
		for _, unwrappedErr := range UnwrapErrors(err) {
			if unwrappedErr == nil {
				continue
			}

			if errWithStack, ok := unwrappedErr.(interface{ ErrorStack() string }); ok {
				add(errWithStack.ErrorStack())
			}

			add(ctyPanicStack(unwrappedErr))

			for _, functionCallErr := range functionCallErrors(unwrappedErr) {
				add(ErrorStack(functionCallErr))
			}
		}
	}

	return strings.Join(stacks, "\n")
}

// ContainsStackTrace returns true if the given error contain the stack trace.
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

// IsFunctionPanic reports whether an error (or one of its wrapped errors)
// represents a recovered panic. Detection is type-driven only — message-string
// heuristics are intentionally avoided so plain wrapped errors are not
// misclassified.
func IsFunctionPanic(err error) bool {
	for _, unwrappedErr := range UnwrapErrors(err) {
		if unwrappedErr == nil {
			continue
		}

		if isFunctionPanic(unwrappedErr) {
			return true
		}

		for _, functionCallErr := range functionCallErrors(unwrappedErr) {
			if IsFunctionPanic(functionCallErr) {
				return true
			}
		}
	}

	return false
}

// IsError returns true if actual is the same type of error as expected.
func IsError(actual error, expected error) bool {
	return goerrors.Is(actual, expected)
}

// Recover recovers from panics and invokes onPanic with a FunctionPanicError
// wrapping the recovered value. Must be invoked as `defer errors.Recover(...)`
// — wrapping it inside another deferred closure makes recover() a no-op.
func Recover(onPanic func(cause error)) {
	rec := recover()
	if rec == nil {
		return
	}

	if panicErr, ok := rec.(FunctionPanicError); ok {
		onPanic(panicErr)
		return
	}

	onPanic(NewFunctionPanicError(rec))
}

// UnwrapMultiErrors unwraps all nested multierrors into error slice.
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

// UnwrapErrors unwraps all nested multierrors, and errors that were wrapped with fmt.Errorf.
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

// Private helper functions

// ctyPanicStack returns the panic stack carried by github.com/zclconf/go-cty's
// own function.PanicError. Other panic types should expose ErrorStack().
func ctyPanicStack(err error) string {
	var ctyPanic function.PanicError
	if errors.As(err, &ctyPanic) {
		return strings.TrimSpace(string(ctyPanic.Stack))
	}

	return ""
}

func functionCallErrors(err error) []error {
	var diags hcl.Diagnostics
	if errors.As(err, &diags) {
		return functionCallErrorsFromDiagnostics(diags)
	}

	var diag *hcl.Diagnostic
	if errors.As(err, &diag) {
		return functionCallErrorsFromDiagnostic(diag)
	}

	return nil
}

func functionCallErrorsFromDiagnostics(diags hcl.Diagnostics) []error {
	functionCallErrs := make([]error, 0, len(diags))

	for _, diag := range diags {
		functionCallErrs = append(functionCallErrs, functionCallErrorsFromDiagnostic(diag)...)
	}

	return functionCallErrs
}

func functionCallErrorsFromDiagnostic(diag *hcl.Diagnostic) []error {
	if diag == nil {
		return nil
	}

	functionCallExtra, ok := hcl.DiagnosticExtra[hclsyntax.FunctionCallDiagExtra](diag)
	if !ok || functionCallExtra == nil {
		return nil
	}

	functionCallErr := functionCallExtra.FunctionCallError()
	if functionCallErr == nil {
		return nil
	}

	return UnwrapErrors(functionCallErr)
}

func isFunctionPanic(err error) bool {
	if err == nil {
		return false
	}

	if marker, ok := err.(interface{ IsFunctionPanic() bool }); ok && marker.IsFunctionPanic() {
		return true
	}

	var ctyPanic function.PanicError

	return errors.As(err, &ctyPanic)
}
