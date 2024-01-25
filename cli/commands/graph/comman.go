package graph

import (
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
			Name:        "graph-root",
			Destination: &opts.GraphRoot,
			Usage:       "Root directory from where to build graph dependencies.",
		})
	return globalFlags
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                   CommandName,
		Usage:                  "Execute command on dependency graph.",
		DisallowUndefinedFlags: true,
		Flags:                  NewFlags(opts).Sort(),
		Subcommands:            subCommands(opts).SkipRunning(),
		Action: func(ctx *cli.Context) error {
			return Run(opts.OptionsFromContext(ctx))
		},
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
