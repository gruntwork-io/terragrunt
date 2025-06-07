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

func NewFlags(opts *options.TerragruntOptions, commandName string, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        GraphFlagName,
			EnvVars:     tgPrefix.EnvVars(GraphFlagName),
			Destination: &opts.Graph,
			Usage:       "Run the specified OpenTofu/Terraform command following the Directed Acyclic Graph (DAG) of dependencies.",
			Action: func(_ *cli.Context, _ bool) error {
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
	cmd = cmd.WrapAction(func(cliCtx *cli.Context, action cli.ActionFunc) error {
		if alwaysDisableSummary {
			opts.SummaryDisable = true
		}

		if !opts.Graph {
			return action(cliCtx)
		}

		opts.RunTerragrunt = func(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, r *report.Report) error {
			if opts.TerraformCommand == cmd.Name {
				cliCtx := cliCtx.WithValue(options.ContextKey, opts)

				return action(cliCtx)
			}

			return runFn(ctx, l, opts, r)
		}

		return Run(cliCtx, l, opts.OptionsFromContext(cliCtx))
	})

	cmd.Flags = append(cmd.Flags, NewFlags(opts, cmd.Name, nil)...)

	return cmd
}
