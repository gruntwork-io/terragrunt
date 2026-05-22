package strict

import "strings"

// CompletedControlsWarning is the warning emitted when a user explicitly enables
// strict controls that have already been completed.
type CompletedControlsWarning struct {
	controlNames []string
}

func NewCompletedControlsWarning(controlNames []string) *CompletedControlsWarning {
	return &CompletedControlsWarning{
		controlNames: controlNames,
	}
}

func (w CompletedControlsWarning) String() string {
	return "The following strict control(s) are already completed: " +
		strings.Join(w.controlNames, ", ") +
		". Please remove any completed strict controls, as setting them no longer does anything." +
		" For a list of all ongoing strict controls, and the outcomes of previous strict controls," +
		" see https://docs.terragrunt.com/reference/strict-mode" +
		" or get the actual list by running the `terragrunt info strict` command."
}
