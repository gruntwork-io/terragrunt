package cli

import (
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
func ShowAppHelp(ctx *Context) error {
	tpl := ctx.CustomAppHelpTemplate
	if tpl == "" {
		tpl = AppHelpTemplate
	}

	if tpl == "" {
		return errors.Errorf("app help template not defined")
	}

	if ctx.HelpName == "" {
		ctx.HelpName = ctx.Name
	}

	cli.HelpPrinterCustom(ctx.Writer, tpl, ctx, map[string]any{
		"parentCommands": parentCommands,
		"offsetCommands": offsetCommands,
	})

	return NewExitError(nil, ExitCodeSuccess)
}

// ShowCommandHelp prints command help for the given `ctx`.
func ShowCommandHelp(ctx *Context) error {
	if ctx.Command.HelpName == "" {
		ctx.Command.HelpName = ctx.Command.Name
	}

	if ctx.Command.CustomHelp != nil {
		if err := ctx.Command.CustomHelp(ctx); err != nil {
			return err
		}

		return NewExitError(nil, ExitCodeSuccess)
	}

	tpl := ctx.Command.CustomHelpTemplate
	if tpl == "" {
		tpl = CommandHelpTemplate
	}

	if tpl == "" {
		return errors.Errorf("command help template not defined")
	}

	HelpPrinterCustom(ctx, tpl, nil)

	return NewExitError(nil, ExitCodeSuccess)
}

func HelpPrinterCustom(ctx *Context, tpl string, customFuncs map[string]any) {
	var funcs = map[string]any{
		"parentCommands": parentCommands,
		"offsetCommands": offsetCommands,
	}

	if customFuncs != nil {
		maps.Copy(funcs, customFuncs)
	}

	cli.HelpPrinterCustom(ctx.Writer, tpl, ctx, funcs)
}

func ShowVersion(ctx *Context) error {
	tpl := ctx.CustomAppVersionTemplate
	if tpl == "" {
		tpl = AppVersionTemplate
	}

	if tpl == "" {
		return errors.Errorf("app version template not defined")
	}

	cli.HelpPrinterCustom(ctx.Writer, tpl, ctx, nil)

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
