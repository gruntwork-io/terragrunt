package errors

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"slices"

	goerrors "github.com/go-errors/errors"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// ErrorStack returns an stack trace if available.
func ErrorStack(err error) (stack string) {
	var errStacks []string

	for _, err := range UnwrapMultiErrors(err) {
		for _, unwrappedErr := range UnwrapErrors(err) {
			if errWithStack, ok := unwrappedErr.(interface{ ErrorStack() string }); ok {
				errStacks = append(errStacks, errWithStack.ErrorStack())
			}

			if panicStack := functionPanicStack(unwrappedErr); panicStack != "" {
				errStacks = append(errStacks, panicStack)
			}

			for _, functionCallErr := range functionCallErrors(unwrappedErr) {
				errStacks = append(errStacks, ErrorStack(functionCallErr))
			}
		}
	}

	return strings.Join(errStacks, "\n")
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

// IsFunctionPanic reports whether an error (or one of its wrapped errors) is a function panic.
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

// Recover tries to recover from panics and calls the given onPanic function.
func Recover(onPanic func(cause error)) {
	rec := recover()
	if rec == nil {
		return
	}

	if err, isError := rec.(error); isError {
		onPanic(New(err))
		return
	}

	onPanic(New(fmt.Errorf("panic: %v", rec)))
}

// UnwrapMultiErrors unwraps all nested multierrors into error slice.
func UnwrapMultiErrors(err error) (errs []error) {
	errs = []error{err}

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
func UnwrapErrors(err error) (errs []error) {
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

func functionPanicStack(err error) string {
	if panicErr, ok := err.(interface{ ErrorStack() string }); ok {
		if errStack := strings.TrimSpace(panicErr.ErrorStack()); errStack != "" {
			return errStack
		}
	}

	return legacyPanicStack(err)
}

func legacyPanicStack(err error) string {
	recoveryErr := reflect.ValueOf(err)
	if !recoveryErr.IsValid() {
		return ""
	}

	for recoveryErr.Kind() == reflect.Pointer {
		if recoveryErr.IsNil() {
			return ""
		}

		recoveryErr = recoveryErr.Elem()
	}

	if recoveryErr.Kind() != reflect.Struct {
		return ""
	}

	stackField := recoveryErr.FieldByName("Stack")
	if !stackField.IsValid() {
		return ""
	}

	switch stackField.Type() {
	case reflect.TypeOf([]byte{}):
		return strings.TrimSpace(string(stackField.Interface().([]byte)))
	case reflect.TypeOf(""):
		return strings.TrimSpace(stackField.Interface().(string))
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

	if panicErr, ok := err.(interface{ IsFunctionPanic() bool }); ok && panicErr.IsFunctionPanic() {
		return true
	}

	return functionPanicStack(err) != ""
}
