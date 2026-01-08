package cli

import (
	"context"
	"slices"
	"strings"

	"maps"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/urfave/cli/v2"
)

var (
	// AppVersionTemplate is the text template for the Default version topic.
	AppVersionTemplate = ""

	// AppHelpTemplate is the text template for the Default help topic.
	AppHelpTemplate = ""

	// CommandHelpTemplate is the text template for the command help topic.
	CommandHelpTemplate = ""
)

// ShowAppHelp prints App help.
func ShowAppHelp(_ context.Context, cliCtx *Context) error {
	tpl := cliCtx.CustomAppHelpTemplate
	if tpl == "" {
		tpl = AppHelpTemplate
	}

	if tpl == "" {
		return errors.Errorf("app help template not defined")
	}

	if cliCtx.HelpName == "" {
		cliCtx.HelpName = cliCtx.Name
	}

	cli.HelpPrinterCustom(cliCtx.Writer, tpl, cliCtx, map[string]any{
		"parentCommands": parentCommands,
		"offsetCommands": offsetCommands,
	})

	return NewExitError(nil, ExitCodeSuccess)
}

// ShowCommandHelp prints command help for the given `cliCtx`.
func ShowCommandHelp(ctx context.Context, cliCtx *Context) error {
	if cliCtx.Command.HelpName == "" {
		cliCtx.Command.HelpName = cliCtx.Command.Name
	}

	if cliCtx.Command.CustomHelp != nil {
		if err := cliCtx.Command.CustomHelp(ctx, cliCtx); err != nil {
			return err
		}

		return NewExitError(nil, ExitCodeSuccess)
	}

	tpl := cliCtx.Command.CustomHelpTemplate
	if tpl == "" {
		tpl = CommandHelpTemplate
	}

	if tpl == "" {
		return errors.Errorf("command help template not defined")
	}

	HelpPrinterCustom(cliCtx, tpl, nil)

	return NewExitError(nil, ExitCodeSuccess)
}

func HelpPrinterCustom(cliCtx *Context, tpl string, customFuncs map[string]any) {
	var funcs = map[string]any{
		"parentCommands": parentCommands,
		"offsetCommands": offsetCommands,
	}

	if customFuncs != nil {
		maps.Copy(funcs, customFuncs)
	}

	cli.HelpPrinterCustom(cliCtx.Writer, tpl, cliCtx, funcs)
}

func ShowVersion(_ context.Context, cliCtx *Context) error {
	tpl := cliCtx.CustomAppVersionTemplate
	if tpl == "" {
		tpl = AppVersionTemplate
	}

	if tpl == "" {
		return errors.Errorf("app version template not defined")
	}

	cli.HelpPrinterCustom(cliCtx.Writer, tpl, cliCtx, nil)

	return NewExitError(nil, ExitCodeSuccess)
}

func parentCommands(ctx *Context) Commands {
	var cmds Commands

	for parent := ctx.Parent(); parent != nil; parent = parent.Parent() {
		if cmd := parent.Command; cmd != nil {
			if cmd.HelpName == "" {
				cmd.HelpName = cmd.Name
			}

			cmds = append(cmds, cmd)
		}
	}

	slices.Reverse(cmds)

	return cmds
}

// offsetCommands tries to find the max width of the names column.
func offsetCommands(cmds Commands, fixed int) int {
	var width = 0

	for _, cmd := range cmds {
		s := strings.Join(cmd.Names(), ", ")
		if len(s) > width {
			width = len(s)
		}
	}

	return width + fixed
}
