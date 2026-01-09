// Package runall provides the feature that runs a terraform command
// against a 'stack' by running the specified command in each subfolder.
package runall

import (
	"context"

	"github.com/gruntwork-io/terragrunt/cli/commands/common"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	AllFlagName  = "all"
	AllFlagAlias = "a"
)

func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        AllFlagName,
			Aliases:     []string{AllFlagAlias},
			EnvVars:     tgPrefix.EnvVars(AllFlagName),
			Destination: &opts.RunAll,
			Usage:       `Run the specified command on the stack of units in the current directory.`,
			Action: func(_ context.Context, _ *cli.Context, _ bool) error {
				if opts.Graph {
					return errors.New(new(common.AllGraphFlagsError))
				}

				return nil
			},
		}),
	}
}

// WrapCommand appends flags to the given `cmd` and wraps its action.
func WrapCommand(
	l log.Logger,
	opts *options.TerragruntOptions,
	cmd *cli.Command,
	runFn func(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, r *report.Report) error,
	alwaysDisableSummary bool,
) *cli.Command {
	cmd = cmd.WrapAction(func(ctx context.Context, cliCtx *cli.Context, action cli.ActionFunc) error {
		if alwaysDisableSummary {
			opts.SummaryDisable = true
		}

		if !opts.RunAll {
			return action(ctx, cliCtx)
		}

		opts.RunTerragrunt = func(innerCtx context.Context, l log.Logger, opts *options.TerragruntOptions, r *report.Report) error {
			if opts.TerraformCommand == cmd.Name {
				innerCtx = context.WithValue(innerCtx, options.ContextKey, opts)

				return action(innerCtx, cliCtx)
			}

			return runFn(innerCtx, l, opts, r)
		}

		return Run(ctx, l, opts.OptionsFromContext(ctx))
	})

	cmd.Flags = append(cmd.Flags, NewFlags(opts, nil)...)

	return cmd
}
