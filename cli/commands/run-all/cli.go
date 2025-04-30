// Package runall provides the `run-all` command that runs a terraform command against a 'stack' by running the specified command in each subfolder.
package runall

import (
	"context"
	"sort"

	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	graphdependencies "github.com/gruntwork-io/terragrunt/cli/commands/graph-dependencies"
	"github.com/gruntwork-io/terragrunt/cli/commands/render"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	terragruntinfo "github.com/gruntwork-io/terragrunt/cli/commands/terragrunt-info"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "run-all"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                         CommandName,
		Usage:                        "Run a terraform command against a 'stack' by running the specified command in each subfolder.",
		Description:                  "The command will recursively find terragrunt modules in the current directory tree and run the terraform command in dependency order (unless the command is destroy, in which case the command is run in reverse dependency order).",
		Subcommands:                  subCommands(opts).SkipRunning(),
		Action:                       action(opts),
		Flags:                        run.NewFlags(opts, nil),
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

		return runall.Run(cliCtx.Context, opts.OptionsFromContext(cliCtx))
	}
}

func subCommands(opts *options.TerragruntOptions) cli.Commands {
	cmds := cli.Commands{
		terragruntinfo.NewCommand(opts),    // terragrunt-info
		graphdependencies.NewCommand(opts), // graph-dependencies
		render.NewCommand(opts),            // render
		awsproviderpatch.NewCommand(opts),  // aws-provider-patch
	}

	sort.Sort(cmds)

	// add terraform command `*` after sorting to put the command at the end of the list in the help.
	cmds.Add(run.NewCommand(opts))

	return cmds
}
