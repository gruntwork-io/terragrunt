package tips

import (
	"strings"
)

// InvalidTipNameError is an error that is returned when an invalid tip name is requested.
type InvalidTipNameError struct {
	allowedNames []string
}

func NewInvalidTipNameError(allowedNames []string) *InvalidTipNameError {
	return &InvalidTipNameError{
		allowedNames: allowedNames,
	}
}

func (err InvalidTipNameError) Error() string {
	return "allowed tip(s): " + strings.Join(err.allowedNames, ", ")
}

func (err InvalidTipNameError) Is(target error) bool {
	_, ok := target.(*InvalidTipNameError)
	return ok
}
