package strict

import (
	"strings"
)

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

// CompletedControlsError is an error that is returned when completed controls are requested.
type CompletedControlsError struct {
	controlNames []string
}

func NewCompletedControlsError(controlNames []string) *CompletedControlsError {
	return &CompletedControlsError{
		controlNames: controlNames,
	}
}

func (err CompletedControlsError) Error() string {
	return "The following strict control(s) are already completed: " + strings.Join(err.controlNames, ", ") + ". Please remove any completed strict controls, as setting them no longer does anything. For a list of all ongoing strict controls, and the outcomes of previous strict controls, see https://terragrunt.gruntwork.io/docs/reference/strict-mode"
}
