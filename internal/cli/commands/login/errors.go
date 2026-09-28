package login

import (
	"errors"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
)

// ErrExperimentRequired is returned when the login command is run without the
// tg-login experiment enabled.
var ErrExperimentRequired = errors.New(
	"the '" + CommandName + "' command requires the '" + experiment.TGLogin + "' experiment to be enabled" +
		" (set --experiment " + experiment.TGLogin + " or TG_EXPERIMENT=" + experiment.TGLogin + ")",
)

// ErrLoginUnavailable reports a portal that would not begin a login.
var ErrLoginUnavailable = errors.New("the portal is not accepting logins from the Terragrunt CLI")
