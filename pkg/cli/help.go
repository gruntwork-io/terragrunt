package cli

import (
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/urfave/cli/v2"
)

const helpName = "help"

var (
	// AppHelpTemplate is the text template for the Default help topic.
	// cli.go uses text/template to render templates. You can
	// render custom help text by setting this variable.
	AppHelpTemplate = ""

	// CommandHelpTemplate is the text template for the command help topic.
	// cli.go uses text/template to render templates. You can
	// render custom help text by setting this variable.
	CommandHelpTemplate = ""
)

// ShowAppHelp is an action that displays the help.
func ShowAppHelp(ctx *Context) error {
	tpl := ctx.App.CustomAppHelpTemplate
	if tpl == "" {
		tpl = AppHelpTemplate
	}
	if tpl == "" {
		return errors.Errorf("help app template not defined")
	}

	if ctx.App.HelpName == "" {
		ctx.App.HelpName = ctx.App.Name
	}

	if ctx.App.ExtraInfo == nil {
		cli.HelpPrinter(ctx.App.Writer, tpl, ctx.App)
		return nil
	}

	customAppData := func() map[string]interface{} {
		return map[string]interface{}{
			"ExtraInfo": ctx.App.ExtraInfo,
		}
	}

	cli.HelpPrinterCustom(ctx.App.Writer, tpl, ctx.App, customAppData())
	return nil
}

// ShowCommandHelp prints help for the given command
func ShowCommandHelp(ctx *Context, command string) error {
	for _, cmd := range ctx.App.Commands {
		if cmd.HasName(command) {
			tpl := cmd.CustomHelpTemplate
			if tpl == "" {
				tpl = CommandHelpTemplate
			}
			if tpl == "" {
				return errors.Errorf("help command template not defined")
			}

			if cmd.HelpName == "" {
				cmd.HelpName = cmd.Name
			}

			cli.HelpPrinterCustom(ctx.App.Writer, tpl, cmd, nil)
			return nil
		}
	}

	return nil
}
