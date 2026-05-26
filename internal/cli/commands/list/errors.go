package list

import (
	"github.com/gruntwork-io/terragrunt/internal/experiment"
)

// TUIExperimentError is returned when --tui is used without the ls-tui
// experiment enabled.
type TUIExperimentError struct{}

// NewTUIExperimentError returns a new TUIExperimentError.
func NewTUIExperimentError() *TUIExperimentError {
	return &TUIExperimentError{}
}

func (err TUIExperimentError) Error() string {
	return "the --tui flag requires the '" + experiment.LsTUI + "' experiment to be enabled" +
		" (set --experiment " + experiment.LsTUI + " or TG_EXPERIMENT=" + experiment.LsTUI + ")"
}

func (err TUIExperimentError) Is(target error) bool {
	_, ok := target.(*TUIExperimentError)
	return ok
}
