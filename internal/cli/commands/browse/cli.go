// Package browse provides the ability to interactively browse the Terragrunt
// configurations in your codebase via the `terragrunt browse` command.
package browse

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const CommandName = "browse"

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	cmdOpts := NewOptions(opts)
	prefix := flags.Prefix{CommandName}

	cmdFlags := shared.NewFeatureFlags(opts, prefix)
	cmdFlags = append(cmdFlags, shared.NewNoDiscoveryAuthProviderCmdFlag(opts, prefix))

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
			return Run(ctx, l, cmdOpts)
		},
	}
}
