package cli

import (
	"github.com/urfave/cli/v2"
)

// Context can be used to retrieve context-specific args and parsed command-line options.
type Context struct {
	*cli.Context
	App *App
}

// Args returns the command line arguments associated with the context.
func (ctx *Context) Args() *Args {
	return &Args{
		Args: ctx.Context.Args(),
		App:  ctx.App,
	}
}

func NewContext(cliCtx *cli.Context, app *App) *Context {
	return &Context{
		Context: cliCtx,
		App:     app,
	}
}
