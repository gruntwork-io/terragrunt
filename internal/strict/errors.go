package strict

import "strings"

// InvalidControlNameError is an error that is returned when an invalid control name is requested.
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

// Is reports whether `target` is also an `InvalidControlNameError`, enabling
// `errors.Is` matching without comparing on the error text.
func (err InvalidControlNameError) Is(target error) bool {
	_, ok := target.(*InvalidControlNameError)

	return ok
}
