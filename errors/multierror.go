package errors

import (
	"fmt"
	"strings"
)

type MultiError struct {
	Errors []error
}

func NewMultiError(errs ...error) error {
	if len(errs) == 0 {
		return nil
	}

	return MultiError{Errors: errs}
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
