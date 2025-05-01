// Package graph provides the `graph` command for Terragrunt.
package graph

import (
	"context"
	"sort"

	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	graphdependencies "github.com/gruntwork-io/terragrunt/cli/commands/graph-dependencies"
	renderjson "github.com/gruntwork-io/terragrunt/cli/commands/render-json"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	terragruntinfo "github.com/gruntwork-io/terragrunt/cli/commands/terragrunt-info"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const CommandName = "graph"

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                         CommandName,
		Usage:                        "Execute commands on the full graph of dependent modules for the current module, ensuring correct execution order.",
		Flags:                        run.NewFlags(opts, nil),
		Subcommands:                  subCommands(opts).SkipRunning(),
		Action:                       action(opts),
		DisabledErrorOnUndefinedFlag: true,
	}
}

func action(opts *options.TerragruntOptions) cli.ActionFunc {
	return func(cliCtx *cli.Context) error {
		opts.RunTerragrunt = func(ctx context.Context, opts *options.TerragruntOptions) error {
			if cmd := cliCtx.Command.Subcommand(opts.TerraformCommand); cmd != nil {
				cliCtx := cliCtx.WithValue(options.ContextKey, opts)

				return cmd.Action(cliCtx)
			}

			return run.Run(ctx, opts)
		}

		return graph.Run(cliCtx.Context, opts.OptionsFromContext(cliCtx))
	}
}

func subCommands(opts *options.TerragruntOptions) cli.Commands {
	cmds := cli.Commands{
		terragruntinfo.NewCommand(opts),    // terragrunt-info
		graphdependencies.NewCommand(opts), // graph-dependencies
		renderjson.NewCommand(opts),        // render-json
		awsproviderpatch.NewCommand(opts),  // aws-provider-patch
	}
	sort.Sort(cmds)
	cmds.Add(run.NewCommand(opts))

	return cmds
}
