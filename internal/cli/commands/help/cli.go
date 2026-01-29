// Package help represents the help CLI command that works the same as the `--help` flag.
package help

import (
	"context"
	"os"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	CommandName = "help"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	return &clihelper.Command{
		Name:                         CommandName,
		Usage:                        "Show help.",
		Hidden:                       true,
		DisabledErrorOnUndefinedFlag: true,
		Action: func(ctx context.Context, cliCtx *clihelper.Context) error {
			return Action(ctx, cliCtx, l, opts)
		},
	}
}

func Action(ctx context.Context, cliCtx *clihelper.Context, l log.Logger, _ *options.TerragruntOptions) error {
	var (
		args = cliCtx.Args()
		cmds = cliCtx.Commands
	)

	if l.Level() >= log.DebugLevel {
		// https: //github.com/urfave/cli/blob/f035ffaa3749afda2cd26fb824aa940747297ef1/help.go#L401
		if err := os.Setenv("CLI_TEMPLATE_ERROR_DEBUG", "1"); err != nil {
			return errors.Errorf("failed to set CLI_TEMPLATE_ERROR_DEBUG environment variable: %w", err)
		}
	}

	if cmdName := args.CommandName(); cmdName == "" || cmds.Get(cmdName) == nil {
		return clihelper.ShowAppHelp(ctx, cliCtx)
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
		cliCtx = cliCtx.NewCommandContext(cmd, args)
	}

	if cliCtx.Command != nil {
		return clihelper.ShowCommandHelp(ctx, cliCtx)
	}

	return clihelper.NewExitError(
		errors.New(
			clihelper.InvalidCommandNameError(
				args.First(),
			),
		),
		clihelper.ExitCodeGeneralError,
	)
}
