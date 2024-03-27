package graph

import (
	"context"
	"sort"

	"github.com/gruntwork-io/terragrunt/cli/commands"
	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	graphdependencies "github.com/gruntwork-io/terragrunt/cli/commands/graph-dependencies"
	"github.com/gruntwork-io/terragrunt/cli/commands/hclfmt"
	renderjson "github.com/gruntwork-io/terragrunt/cli/commands/render-json"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	terragruntinfo "github.com/gruntwork-io/terragrunt/cli/commands/terragrunt-info"
	validateinputs "github.com/gruntwork-io/terragrunt/cli/commands/validate-inputs"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "graph"
)

func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	globalFlags := commands.NewGlobalFlags(opts)
	globalFlags.Add(
		&cli.GenericFlag[string]{
			Name:        "terragrunt-graph-root",
			Destination: &opts.GraphRoot,
			Usage:       "Root directory from where to build graph dependencies.",
		})
	return globalFlags
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                   CommandName,
		Usage:                  "Execute commands on the full graph of dependent modules for the current module, ensuring correct execution order.",
		DisallowUndefinedFlags: true,
		Flags:                  NewFlags(opts).Sort(),
		Subcommands:            subCommands(opts).SkipRunning(),
		Action:                 action(opts),
	}
}

func action(opts *options.TerragruntOptions) cli.ActionFunc {
	return func(cliCtx *cli.Context) error {
		opts.RunTerragrunt = func(ctx context.Context, opts *options.TerragruntOptions) error {
			if cmd := cliCtx.Command.Subcommand(opts.TerraformCommand); cmd != nil {
				cliCtx := cliCtx.WithValue(options.ContextKey, opts)

				return cmd.Action(cliCtx)
			}

			return terraform.Run(ctx, opts)
		}

		return Run(cliCtx.Context, opts.OptionsFromContext(cliCtx))
	}
}

func subCommands(opts *options.TerragruntOptions) cli.Commands {
	cmds := cli.Commands{
		terragruntinfo.NewCommand(opts),    // terragrunt-info
		validateinputs.NewCommand(opts),    // validate-inputs
		graphdependencies.NewCommand(opts), // graph-dependencies
		hclfmt.NewCommand(opts),            // hclfmt
		renderjson.NewCommand(opts),        // render-json
		awsproviderpatch.NewCommand(opts),  // aws-provider-patch
	}
	sort.Sort(cmds)
	cmds.Add(terraform.NewCommand(opts))

	return cmds
}
