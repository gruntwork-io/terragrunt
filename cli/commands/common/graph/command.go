// Package graph provides the `graph` feature for Terragrunt.
package graph

import (
	"context"

	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	GraphFlagName     = "graph"
	GraphRootFlagName = "graph-root"

	DeprecatedGraphRootFlagName = "graph-root"
)

func NewFlags(opts *options.TerragruntOptions, commandName string, graphFlag *bool, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByCommand(opts.StrictControls, commandName)

	return cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        GraphFlagName,
			EnvVars:     tgPrefix.EnvVars(GraphFlagName),
			Destination: graphFlag,
			Usage:       "Run the specified OpenTofu/Terraform command following the Directed Acyclic Graph (DAG) of dependencies.",
		}),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        GraphRootFlagName,
			EnvVars:     tgPrefix.EnvVars(GraphRootFlagName),
			Destination: &opts.GraphRoot,
			Usage:       "Root directory from where to build graph dependencies.",
		},
			flags.WithDeprecatedName(terragruntPrefix.FlagName(DeprecatedGraphRootFlagName), terragruntPrefixControl)),
	}
}

// WrapCommand appends flags to the given `cmd` and wraps its action.
func WrapCommand(opts *options.TerragruntOptions, cmd *cli.Command) *cli.Command {
	var graphFlag bool

	cmd = cmd.WrapAction(func(cliCtx *cli.Context, action cli.ActionFunc) error {
		if !graphFlag {
			return action(cliCtx)
		}

		opts.RunTerragrunt = func(ctx context.Context, opts *options.TerragruntOptions) error {
			cliCtx := cliCtx.WithValue(options.ContextKey, opts)
			return action(cliCtx)
		}

		return Run(cliCtx, opts.OptionsFromContext(cliCtx))
	})

	flags := append(cmd.Flags, NewFlags(opts, cmd.Name, &graphFlag, nil)...)
	cmd.Flags = flags.Sort()

	return cmd
}
