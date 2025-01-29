package cli

import (
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/urfave/cli/v2"
	"golang.org/x/exp/slices"
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
	tpl := ctx.App.CustomAppHelpTemplate
	if tpl == "" {
		tpl = AppHelpTemplate
	}

	if tpl == "" {
		return errors.Errorf("app help template not defined")
	}

	if ctx.App.HelpName == "" {
		ctx.App.HelpName = ctx.App.Name
	}

	cli.HelpPrinterCustom(ctx.App.Writer, tpl, ctx, nil)

	return NewExitError(nil, ExitCodeSuccess)
}

// ShowCommandHelp prints command help for the given `ctx`.
func ShowCommandHelp(ctx *Context) error {
	if ctx.Command.CustomHelp != nil {
		return ctx.Command.CustomHelp(ctx)
	}

	tpl := ctx.Command.CustomHelpTemplate
	if tpl == "" {
		tpl = CommandHelpTemplate
	}

	if tpl == "" {
		return errors.Errorf("command help template not defined")
	}

	if ctx.Command.HelpName == "" {
		ctx.Command.HelpName = ctx.Command.Name
	}

	cli.HelpPrinterCustom(ctx.App.Writer, tpl, ctx, map[string]any{
		"parentCommands": parentCommands,
	})

	return NewExitError(nil, ExitCodeSuccess)
}

func ShowVersion(ctx *Context) error {
	tpl := ctx.App.CustomAppVersionTemplate
	if tpl == "" {
		tpl = AppVersionTemplate
	}

	if tpl == "" {
		return errors.Errorf("app version template not defined")
	}

	cli.HelpPrinterCustom(ctx.App.Writer, tpl, ctx, nil)

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
