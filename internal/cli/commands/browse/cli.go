// Package browse provides the ability to interactively browse the Terragrunt
// configurations in your codebase via the `terragrunt browse` command.
package browse

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// CommandName is the name of the browse command.
const CommandName = "browse"

// NewCommand returns the browse command, gated behind the browse-tui experiment.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *clihelper.Command {
	cmdOpts := NewOptions(opts)
	cmdFlags := shared.NewFeatureFlags(opts, nil)
	cmdFlags = append(cmdFlags, shared.NewNoDiscoveryAuthProviderCmdFlag(opts, nil))
	flags.DeprecateEnvVarPrefix(cmdFlags, flags.Prefix{flags.TgPrefix, CommandName}, opts.StrictControls)

	return &clihelper.Command{
		Name:  CommandName,
		Usage: "Browse Terragrunt configurations in an interactive TUI.",
		Flags: cmdFlags,
		Before: func(_ context.Context, _ *clihelper.Context) error {
			if !opts.Experiments.Evaluate(experiment.BrowseTUI) {
				return clihelper.NewExitError(ErrExperimentRequired, clihelper.ExitCodeGeneralError)
			}

			return nil
		},
		Action: func(ctx context.Context, _ *clihelper.Context) error {
			return Run(ctx, l, v, cmdOpts)
		},
	}
}
