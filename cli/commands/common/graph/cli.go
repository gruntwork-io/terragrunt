// Package graph provides the `graph` feature for Terragrunt.
package graph

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

const GraphFlagName = "graph"

func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        GraphFlagName,
			EnvVars:     tgPrefix.EnvVars(GraphFlagName),
			Destination: &opts.Graph,
			Usage:       "Run the specified OpenTofu/Terraform command following the Directed Acyclic Graph (DAG) of dependencies.",
			Action: func(_ context.Context, _ *cli.Context, _ bool) error {
				if opts.RunAll {
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

		if !opts.Graph {
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
