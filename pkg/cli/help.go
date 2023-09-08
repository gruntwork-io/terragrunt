package cli

import (
	"github.com/gruntwork-io/go-commons/errors"
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
	return nil
}

// ShowCommandHelp prints help for the given command.
func ShowCommandHelp(ctx *Context, cmdName string) error {
	for _, cmd := range ctx.Command.Subcommands {
		if cmd.HasName(cmdName) {
			tpl := cmd.CustomHelpTemplate
			if tpl == "" {
				tpl = CommandHelpTemplate
			}
			if tpl == "" {
				return errors.Errorf("command help template not defined")
			}

			if cmd.HelpName == "" {
				cmd.HelpName = cmd.Name
			}

			ctx = ctx.Clone(cmd, ctx.Args().Tail())

			cli.HelpPrinterCustom(ctx.App.Writer, tpl, ctx, nil)
			return nil
		}
	}

	return InvalidCommandNameError(cmdName)
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
	return nil
}
