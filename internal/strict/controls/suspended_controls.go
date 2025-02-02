package controls

import (
	"github.com/gruntwork-io/terragrunt/internal/strict"
)

// NewSuspendedControls is a list of suspended controls that are no longer used,
// but are present to avoid returning `InvalidControlNameError`.
func NewSuspendedControls() strict.Controls {
	return strict.Controls{
		&Control{
			Name:   "spin-up",
			Status: strict.SuspendedStatus,
		},
		&Control{
			Name:   "tear-down",
			Status: strict.SuspendedStatus,
		},
		&Control{
			Name:   "plan-all",
			Status: strict.SuspendedStatus,
		},
		&Control{
			Name:   "apply-all",
			Status: strict.SuspendedStatus,
		},
		&Control{
			Name:   "destroy-all",
			Status: strict.SuspendedStatus,
		},
		&Control{
			Name:   "output-all",
			Status: strict.SuspendedStatus,
		},
		&Control{
			Name:   "validate-all",
			Status: strict.SuspendedStatus,
		},
	}
}
