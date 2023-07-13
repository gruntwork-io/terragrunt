package cli

import (
	"github.com/gruntwork-io/terragrunt/pkg/errors"
	"github.com/urfave/cli/v2"
)

var (
	// AppHelpTemplate is the text template for the Default help topic.
	// cli uses text/template to render templates. You can
	// render custom help text by setting this variable.
	AppHelpTemplate = ""

	// CommandHelpTemplate is the text template for the command help topic.
	// cli uses text/template to render templates. You can
	// render custom help text by setting this variable.
	CommandHelpTemplate = ""
)

// ShowAppHelp prints App help.
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

	customAppData := func() map[string]interface{} {
		return map[string]interface{}{
			"CommmandVisibleFlags": ctx.Command.Flags.VisibleFlags,
		}
	}

	cli.HelpPrinterCustom(ctx.App.Writer, tpl, ctx.App, customAppData())
	return nil
}

// ShowCommandHelp prints help for the given command.
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

func ShowHelp(ctx *Context, tpl string) error {
	cli.HelpPrinterCustom(ctx.App.Writer, tpl, ctx.App, nil)
	return nil
}
