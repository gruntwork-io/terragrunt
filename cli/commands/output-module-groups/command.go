// Package outputmodulegroups provides a command to output groups of modules ordered by
// command (apply or destroy) as a list of list in JSON (useful for CI use cases).
package outputmodulegroups

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	// CommandName is the name of the command.
	CommandName = "output-module-groups"

	// SubCommandApply is the sub-command for the apply command.
	SubCommandApply = "apply"

	// SubCommandDestroy is the sub-command for the destroy command.
	SubCommandDestroy = "destroy"
)

// NewCommand returns a new command.
func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:  CommandName,
		Usage: "Output groups of modules ordered by command (apply or destroy) as a list of list in JSON (useful for CI use cases).", //nolint:lll
		Subcommands: cli.Commands{
			subCommandFunc(SubCommandApply, opts),
			subCommandFunc(SubCommandDestroy, opts),
		},
		Action: func(ctx *cli.Context) error { return Run(ctx, opts.OptionsFromContext(ctx)) },
	}
}

func subCommandFunc(cmd string, opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:  cmd,
		Usage: "Recursively find terragrunt modules in the current directory tree and output the dependency order as a list of list in JSON for the " + cmd, //nolint:lll
		Action: func(ctx *cli.Context) error {
			opts.TerraformCommand = cmd

			return Run(ctx, opts.OptionsFromContext(ctx))
		},
	}
}
