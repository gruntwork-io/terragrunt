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
		return nil
	}

	return WithStackTrace(MultiError{Errors: nilsRemoved})
}

func (errs MultiError) Error() string {
	errorMessages := []string{}
	for _, err := range errs.Errors {
		if err != nil {
			errorMessages = append(errorMessages, err.Error())
		}
	}

	return fmt.Sprintf("\n%s", strings.Join(errorMessages, "\n"))
}
