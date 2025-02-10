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

// CompletedExperimentsError is an error that is returned when completed experiments are requested.
type CompletedExperimentsError struct {
	experimentsNames []string
}

func NewCompletedExperimentsError(experimentsNames []string) *CompletedExperimentsError {
	return &CompletedExperimentsError{
		experimentsNames: experimentsNames,
	}
}

func (err CompletedExperimentsError) Error() string {
	return "The following experiment(s) are already completed: " + strings.Join(err.experimentsNames, ", ") + ". Please remove any completed experiments, as setting them no longer does anything. For a list of all ongoing experiments, and the outcomes of previous experiments, see https://terragrunt.gruntwork.io/docs/reference/experiments"
}
