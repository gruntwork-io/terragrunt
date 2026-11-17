// Package help represents the help CLI command that works the same as the `--help` flag.
package help

import (
	"context"
	"fmt"
	"os"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// CommandName is the name of the `terragrunt help` command.
	CommandName = "help"
)

// NewCommand returns the hidden `terragrunt help` command, which prints the
// same help as --help.
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

// Action prints help for the deepest command that the arguments name, such
// as `terragrunt help backend bootstrap`, or the app help when the first
// argument names no command. At debug log level it sets
// CLI_TEMPLATE_ERROR_DEBUG so the CLI library reports help template errors.
func Action(
	ctx context.Context,
	cliCtx *clihelper.Context,
	l log.Logger,
	_ *options.TerragruntOptions,
) error {
	var (
		args = cliCtx.Args()
		cmds = cliCtx.Commands
	)

	if l.Level() >= log.DebugLevel {
		// https: //github.com/urfave/cli/blob/f035ffaa3749afda2cd26fb824aa940747297ef1/help.go#L401
		if err := os.Setenv("CLI_TEMPLATE_ERROR_DEBUG", "1"); err != nil {
			return fmt.Errorf(
				"failed to set CLI_TEMPLATE_ERROR_DEBUG environment variable: %w",
				err,
			)
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
		clihelper.InvalidCommandNameError(args.First()),
		clihelper.ExitCodeGeneralError,
	)
}
