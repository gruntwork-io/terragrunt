package runall

import (
	"context"
	"sort"

	"github.com/gruntwork-io/terragrunt/telemetry"

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
	CommandName = "run-all"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:        CommandName,
		Usage:       "Run a terraform command against a 'stack' by running the specified command in each subfolder.",
		Description: "The command will recursively find terragrunt modules in the current directory tree and run the terraform command in dependency order (unless the command is destroy, in which case the command is run in reverse dependency order).",
		Flags:       commands.NewGlobalFlags(opts),
		Subcommands: subCommands(opts).SkipRunning(),
		Action:      action(opts),
	}
}

func action(opts *options.TerragruntOptions) func(c *cli.Context) error {
	return func(cliCtx *cli.Context) error {
		return telemetry.Span(cliCtx.Context, "run-all "+opts.TerraformCommand, func(childCtx context.Context) error {
			opts.RunTerragrunt = func(opts *options.TerragruntOptions) error {
				if cmd := cliCtx.Command.Subcommand(opts.TerraformCommand); cmd != nil {
					ctx := cliCtx.WithValue(options.ContextKey, opts)
					return cmd.Action(ctx)
				}
				return terraform.Run(opts)
			}
			return Run(childCtx, opts.OptionsFromContext(childCtx))
		})
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

	// add terraform command `*` after sorting to put the command at the end of the list in the help.
	cmds.Add(terraform.NewCommand(opts))

	return cmds
}
