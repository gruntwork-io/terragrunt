package experiment

import (
	"strings"
)

// InvalidExperimentNameError is an error that is returned when an invalid experiment name is requested.
type InvalidExperimentNameError struct {
	allowedNames []string
}

// NewInvalidExperimentNameError returns an [InvalidExperimentNameError] that lists allowedNames.
func NewInvalidExperimentNameError(allowedNames []string) *InvalidExperimentNameError {
	return &InvalidExperimentNameError{
		allowedNames: allowedNames,
	}
}

func (err InvalidExperimentNameError) Error() string {
	return "allowed experiment(s): " + strings.Join(err.allowedNames, ", ")
}

// Is reports whether target is an *InvalidExperimentNameError, whatever names it lists.
func (err InvalidExperimentNameError) Is(target error) bool {
	_, ok := target.(*InvalidExperimentNameError)
	return ok
}
