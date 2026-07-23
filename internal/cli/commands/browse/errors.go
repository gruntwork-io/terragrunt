package browse

import (
	"errors"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
)

// ErrExperimentRequired is returned when the browse command is run without the
// browse experiment enabled.
var ErrExperimentRequired = errors.New(
	"the '" + CommandName + "' command requires the '" + experiment.BrowseTUI + "' experiment to be enabled" +
		" (set --experiment " + experiment.BrowseTUI + " or TG_EXPERIMENT=" + experiment.BrowseTUI + ")",
)
