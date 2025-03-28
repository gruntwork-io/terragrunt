// Package graph provides the `graph` feature for Terragrunt.
package graph

import (
	"context"

	"github.com/gruntwork-io/terragrunt/cli/commands/common"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
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
func WrapCommand(opts *options.TerragruntOptions, cmd *cli.Command) *cli.Command {
	cmd = cmd.WrapAction(func(cliCtx *cli.Context, action cli.ActionFunc) error {
		if !opts.Graph {
			return action(cliCtx)
		}

		opts.RunTerragrunt = func(ctx context.Context, opts *options.TerragruntOptions) error {
			cliCtx := cliCtx.WithValue(options.ContextKey, opts)
			return action(cliCtx)
		}

		return Run(cliCtx, opts.OptionsFromContext(cliCtx))
	})

	flags := append(cmd.Flags, NewFlags(opts, cmd.Name, nil)...)
	cmd.Flags = flags.Sort()

	return cmd
}
