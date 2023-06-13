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

func NewContext(parentCtx *cli.Context, app *App) *Context {
	return &Context{
		Context: parentCtx,
		App:     app,
	}
}

func (ctx *Context) Clone(command *Command, args []string) *Context {
	return &Context{
		Context: ctx.Context,
		App:     ctx.App,
		Command: command,
		args:    newArgs(args),
	}
}

func (ctx *Context) ParseArgs(command *Command, args []string) (*Context, error) {
	command, args, err := command.parseArgs(args)
	if err != nil {
		return nil, err
	}

	return ctx.Clone(command, args), nil
}

// Args returns the command line arguments associated with the context.
func (ctx *Context) RawArgs() cli.Args {
	return ctx.Context.Args()
}

// Args returns the command line arguments associated with the context.
func (ctx *Context) Args() *Args {
	return ctx.args
}
