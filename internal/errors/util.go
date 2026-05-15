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
func ErrorStack(err error) string {
	var errStacks []string

	for _, err := range UnwrapMultiErrors(err) {
		for _, unwrappedErr := range UnwrapErrors(err) {
			if errWithStack, ok := unwrappedErr.(interface{ ErrorStack() string }); ok {
				errStacks = append(errStacks, errWithStack.ErrorStack())
			}

			if ctyStack := ctyFunctionPanicStack(unwrappedErr); ctyStack != "" {
				errStacks = append(errStacks, ctyStack)
			}

			for _, functionCallErr := range functionCallErrors(unwrappedErr) {
				errStacks = append(errStacks, ErrorStack(functionCallErr))
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
			if errWithStack, ok := err.(interface{ ErrorStack() string }); ok && errWithStack.ErrorStack() != "" {
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

const ctyFunctionPanicPackage = "github.com/zclconf/go-cty/cty/function"

// IsFunctionPanic reports whether an error (or one of its wrapped errors) is a function panic.
func IsFunctionPanic(err error) bool {
	for _, unwrappedErr := range UnwrapErrors(err) {
		if isFunctionPanic(unwrappedErr) {
			return true
		}
	}

	return false
}

func isFunctionPanic(err error) bool {
	if panicErr, ok := err.(interface{ IsFunctionPanic() bool }); ok && panicErr.IsFunctionPanic() {
		return true
	}

	if isCTYFunctionPanicError(err) {
		return true
	}

	for _, functionCallErr := range functionCallErrors(err) {
		if IsFunctionPanic(functionCallErr) {
			return true
		}
	}

	return false
}

// IsError returns true if actual is the same type of error as expected.
// This method unwraps the given error objects (if they are wrapped in
// objects with a stacktrace) and then does a simple equality check on them.
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
				errs = slices.Delete(errs, index, index+1)
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

func ctyFunctionPanicStack(err error) string {
	if !isCTYFunctionPanicError(err) {
		return ""
	}

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
		return string(stackField.Interface().([]byte))
	case reflect.TypeOf(""):
		return stackField.Interface().(string)
	}

	return ""
}

func isCTYFunctionPanicError(err error) bool {
	errType := reflect.TypeOf(err)
	for errType != nil {
		if errType.PkgPath() == ctyFunctionPanicPackage && errType.Name() == "PanicError" {
			return true
		}

		if errType.Kind() == reflect.Pointer {
			errType = errType.Elem()
			continue
		}

		return false
	}

	return false
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
