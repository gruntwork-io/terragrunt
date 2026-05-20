package shared

import "github.com/gruntwork-io/terragrunt/internal/errors"

// AllGraphFlagsError is returned when both --all and --graph flags are used simultaneously.
type AllGraphFlagsError byte

func (err *AllGraphFlagsError) Error() string {
	return "Using the `--all` and `--graph` flags simultaneously is not supported."
}

var ErrNoDiscoveryAuthProviderCmdRequiresExperiment = errors.New(
	"--no-discovery-auth-provider-cmd requires the 'opt-out-auth' experiment to be enabled (e.g., --experiment=opt-out-auth)",
)
