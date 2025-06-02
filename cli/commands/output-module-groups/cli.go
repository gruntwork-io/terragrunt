// Package outputmodulegroups provides a command to output groups of modules ordered by command (apply or destroy) as a list of list in JSON (useful for CI use cases).
package outputmodulegroups

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName       = "output-module-groups"
	SubCommandApply   = "apply"
	SubCommandDestroy = "destroy"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	newCommand := "terragrunt " + find.CommandName + " --" + find.JSONFlagName

	control := controls.NewDeprecatedReplacedCommand(CommandName, newCommand)
	opts.StrictControls.FilterByNames(controls.DeprecatedCommands, controls.CLIRedesign, CommandName).AddSubcontrolsToCategory(controls.CLIRedesignCommandsCategoryName, control)

	return &cli.Command{
		Name:  CommandName,
		Flags: run.NewFlags(l, opts, nil),
		Usage: "Output groups of modules ordered by command (apply or destroy) as a list of list in JSON (useful for CI use cases).",
		Subcommands: cli.Commands{
			subCommandFunc(l, SubCommandApply, opts),
			subCommandFunc(l, SubCommandDestroy, opts),
		},
		DisabledErrorOnUndefinedFlag: true,
		Hidden:                       true,
		Before: func(ctx *cli.Context) error {
			if err := control.Evaluate(ctx); err != nil {
				return cli.NewExitError(err, cli.ExitCodeGeneralError)
			}

			return nil
		},
		Action: func(ctx *cli.Context) error { return Run(ctx, l, opts.OptionsFromContext(ctx)) },
	}
}

func subCommandFunc(l log.Logger, cmd string, opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                         cmd,
		Usage:                        "Recursively find terragrunt modules in the current directory tree and output the dependency order as a list of list in JSON for the " + cmd,
		DisabledErrorOnUndefinedFlag: true,
		Action: func(ctx *cli.Context) error {
			opts.TerraformCommand = cmd
			return Run(ctx, l, opts.OptionsFromContext(ctx))
		},
	}
}
