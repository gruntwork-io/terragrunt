package outputmodulegroups

import (
	"fmt"

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
		Usage:  fmt.Sprintf("Recursively find terragrunt modules in the current directory tree and output the dependency order as a list of list in JSON for the %s", cmd),
		Action: func(ctx *cli.Context) error { return Run(opts.OptionsFromContext(ctx)) },
	}
}
