package errors

import (
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
)

// MultiError is an error type to track multiple errors.
type MultiError struct {
	inner *multierror.Error
}

// WrappedErrors returns the error slice that this Error is wrapping.
func (errs *MultiError) WrappedErrors() []error {
	if errs.inner == nil {
		return nil
	}

	return errs.inner.WrappedErrors()
}

func (errs *MultiError) Unwrap() []error {
	return errs.WrappedErrors()
}

// ErrorOrNil returns an error interface if this Error represents
// a list of errors, or returns nil if the list of errors is empty.
func (errs *MultiError) ErrorOrNil() error {
	if errs == nil || errs.inner == nil {
		return nil
	}

	if err := errs.inner.ErrorOrNil(); err != nil {
		return errs
	}

	return nil
}

// Append is a helper function that will append more errors
// onto an Multierror in order to create a larger errs-error.
func (errs *MultiError) Append(appendErrs ...error) *MultiError {
	if errs == nil {
		errs = &MultiError{inner: new(multierror.Error)}
	}

	if errs.inner == nil {
		errs.inner = new(multierror.Error)
	}

	return &MultiError{inner: multierror.Append(errs.inner, appendErrs...)}
}

// Len implements sort.Interface function for length.
func (errs *MultiError) Len() int {
	if errs == nil {
		errs = &MultiError{inner: new(multierror.Error)}
	}

	if errs.inner == nil {
		errs.inner = new(multierror.Error)
	}

	return len(errs.inner.Errors)
}

// Swap implements sort.Interface function for swapping elements.
func (errs *MultiError) Swap(i, j int) {
	errs.inner.Errors[i], errs.inner.Errors[j] = errs.inner.Errors[j], errs.inner.Errors[i]
}

// Less implements sort.Interface function for determining order.
func (errs *MultiError) Less(i, j int) bool {
	return errs.inner.Errors[i].Error() < errs.inner.Errors[j].Error()
}

// Error implements the error interface.
func (errs *MultiError) Error() string {
	unwrappedErrs := UnwrapMultiErrors(errs)

	strs := make([]string, len(unwrappedErrs))

	for i := range unwrappedErrs {
		strs[i] = addIndent(unwrappedErrs[i].Error())
	}

	errStr := strings.Join(strs, "\n\n")

	if len(strs) == 1 {
		return fmt.Sprintf("error occurred:\n\n%s\n", errStr)
	}

	return fmt.Sprintf("%d errors occurred:\n\n%s\n", len(strs), errStr)
}

func addIndent(str string) string {
	// for output on Windows OS
	str = strings.ReplaceAll(str, "\r\n", "\n")
	rawLines := strings.Split(str, "\n")

	var lines []string //nolint:prealloc

	for i, line := range rawLines {
		format := "  %s"
		if i == 0 {
			format = "* %s"
		}

		line = fmt.Sprintf(format, line)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}
