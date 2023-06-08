package cli

import (
	"github.com/urfave/cli/v2"
)

// ShowAppHelp is an action that displays the help.
func ShowAppHelp(ctx *Context) error {
	tpl := ctx.App.CustomAppHelpTemplate
	if tpl == "" {
		tpl = cli.AppHelpTemplate
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
