package strict

import (
	"strings"
)

type InvalidControlNameError struct {
	allowedNames []string
}

func NewInvalidControlNameError(allowedNames []string) *InvalidControlNameError {
	return &InvalidControlNameError{
		allowedNames: allowedNames,
	}
}

func (err InvalidControlNameError) Error() string {
	return "allowed control(s): " + strings.Join(err.allowedNames, ", ")
}
