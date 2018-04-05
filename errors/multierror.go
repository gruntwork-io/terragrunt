package errors

import (
	"fmt"
	"strings"
)

type MultiError struct {
	Errors []error
}

func NewMultiError(errs ...error) error {
	nilsRemoved := make([]error, 0, len(errs))
	for _, item := range errs {
		if item != nil {
			nilsRemoved = append(nilsRemoved, item)
		}
	}

	if len(nilsRemoved) == 0 {
		return MultiError{}
	}

	return MultiError{Errors: nilsRemoved}
}

func (errs MultiError) HasErrors() bool {
	if len(errs.Errors) > 0 {
		return true
	}

	return false
}

func (errs *MultiError) AppendError(newError error) {
	errs.Errors = append(errs.Errors, newError)
}

func (errs MultiError) Error() string {
	errorMessages := []string{}
	for _, err := range errs.Errors {
		if err != nil {
			errorMessages = append(errorMessages, err.Error())
		}
	}

	return fmt.Sprintf("Hit multiple errors:\n%s", strings.Join(errorMessages, "\n"))
}
