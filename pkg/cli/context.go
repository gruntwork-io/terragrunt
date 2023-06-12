package cli

import (
	"github.com/urfave/cli/v2"
)

// Context can be used to retrieve context-specific args and parsed command-line options.
type Context struct {
	*cli.Context
	*App
	Command *Command
	args    *Args
}

func NewContext(parentCtx *cli.Context, app *App, command *Command, args []string) *Context {
	return &Context{
		Context: parentCtx,
		App:     app,
		Command: command,
		args:    newArgs(args),
	}
}

func (ctx *Context) Clone(command *Command, args []string) *Context {
	return NewContext(ctx.Context, ctx.App, command, args)
}

// Args returns the command line arguments associated with the context.
func (ctx *Context) RawArgs() cli.Args {
	return ctx.Context.Args()
}

// Args returns the command line arguments associated with the context.
func (ctx *Context) Args() *Args {
	return ctx.args
}
