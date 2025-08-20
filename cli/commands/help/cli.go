// Package help represents the help CLI command that works the same as the `--help` flag.
package help

import (
	"os"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "help"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                         CommandName,
		Usage:                        "Show help.",
		Hidden:                       true,
		DisabledErrorOnUndefinedFlag: true,
		Action: func(ctx *cli.Context) error {
			return Action(ctx, l, opts)
		},
	}
}

func Action(ctx *cli.Context, l log.Logger, opts *options.TerragruntOptions) error {
	var (
		args = ctx.Args()
		cmds = ctx.Commands
	)

	if l.Level() >= log.DebugLevel {
		// https: //github.com/urfave/cli/blob/f035ffaa3749afda2cd26fb824aa940747297ef1/help.go#L401
		if err := os.Setenv("CLI_TEMPLATE_ERROR_DEBUG", "1"); err != nil {
			return errors.Errorf("failed to set CLI_TEMPLATE_ERROR_DEBUG environment variable: %w", err)
		}
	}

	if cmdName := args.CommandName(); cmdName == "" || cmds.Get(cmdName) == nil {
		return cli.ShowAppHelp(ctx)
	}

	const maxCommandDepth = 1000 // Maximum depth of nested subcommands

	for i := 0; i < maxCommandDepth && args.Len() > 0; i++ {
		cmdName := args.CommandName()

		cmd := cmds.Get(cmdName)
		if cmd == nil {
			break
		}

		args = args.Remove(cmdName)
		cmds = cmd.Subcommands
		ctx = ctx.NewCommandContext(cmd, args)
	}

	if ctx.Command != nil {
		return cli.ShowCommandHelp(ctx)
	}

	return cli.NewExitError(errors.New(cli.InvalidCommandNameError(args.First())), cli.ExitCodeGeneralError)
}
