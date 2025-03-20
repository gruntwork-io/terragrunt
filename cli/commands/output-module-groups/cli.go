// Package outputmodulegroups provides a command to output groups of modules ordered by command (apply or destroy) as a list of list in JSON (useful for CI use cases).
package outputmodulegroups

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName       = "output-module-groups"
	SubCommandApply   = "apply"
	SubCommandDestroy = "destroy"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:  CommandName,
		Flags: run.NewFlags(opts, nil),
		Usage: "Output groups of modules ordered by command (apply or destroy) as a list of list in JSON (useful for CI use cases).",
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
		Usage: "Recursively find terragrunt modules in the current directory tree and output the dependency order as a list of list in JSON for the " + cmd,
		Action: func(ctx *cli.Context) error {
			opts.TerraformCommand = cmd
			return Run(ctx, opts.OptionsFromContext(ctx))
		},
	}
}
