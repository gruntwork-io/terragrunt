package experiment

import (
	"strings"
)

// InvalidExperimentNameError is an error that is returned when an invalid experiment name is requested.
type InvalidExperimentNameError struct {
	allowedNames []string
}

func NewInvalidExperimentNameError(allowedNames []string) *InvalidExperimentNameError {
	return &InvalidExperimentNameError{
		allowedNames: allowedNames,
	}
}

func (err InvalidExperimentNameError) Error() string {
	return "allowed experiment(s): " + strings.Join(err.allowedNames, ", ")
}

func (err InvalidExperimentNameError) Is(target error) bool {
	_, ok := target.(*InvalidExperimentNameError)
	return ok
}
