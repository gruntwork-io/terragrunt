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

const (
	CommandName = "browse"

	NoHiddenFlagName = "no-hidden"
)

func NewFlags(l log.Logger, opts *Options, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	filterFlags := shared.NewFilterFlags(l, opts.TerragruntOptions)

	const numLocalFlags = 1

	result := make(clihelper.Flags, 0, numLocalFlags+len(filterFlags))
	result = append(result,
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoHiddenFlagName,
			EnvVars:     tgPrefix.EnvVars(NoHiddenFlagName),
			Destination: &opts.NoHidden,
			Usage:       "Exclude hidden directories from the browser.",
		}),
	)

	return append(result, filterFlags...)
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	cmdOpts := NewOptions(opts)
	prefix := flags.Prefix{CommandName}

	cmdFlags := NewFlags(l, cmdOpts, prefix)
	cmdFlags = append(cmdFlags, shared.NewFeatureFlags(opts, prefix)...)
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
