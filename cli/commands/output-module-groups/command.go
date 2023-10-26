package outputmodulegroups

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName       = "output-module-groups"
	SubCommandApply   = "apply"
	SubCommandDestroy = "destroy"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:        CommandName,
		Usage:       "Output groups of modules ordered by command (apply or destroy) as a list of list in JSON (useful for CI use cases).",
		Description: "The command will recursively find terragrunt modules in the current directory tree and output the list of list in JSON for the terraform command in dependency order (unless the command is destroy, in which case the command is run in reverse dependency order).",
		Subcommands: subCommands(opts),
		Action:      func(ctx *cli.Context) error { return Run(opts.OptionsFromContext(ctx)) },
	}
}

func subCommands(opts *options.TerragruntOptions) cli.Commands {
	cmds := cli.Commands{
		subCommandFunc(SubCommandApply, opts),
		subCommandFunc(SubCommandDestroy, opts),
	}

	return cmds
}

func subCommandFunc(cmd string, opts *options.TerragruntOptions) *cli.Command {
	opts.TerraformCommand = cmd
	return &cli.Command{
		Name:   cmd,
		Action: func(ctx *cli.Context) error { return Run(opts.OptionsFromContext(ctx)) },
	}
}
