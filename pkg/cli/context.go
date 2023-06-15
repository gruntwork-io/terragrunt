package cli

import (
	"context"

	"github.com/urfave/cli/v2"
)

// Context can be used to retrieve context-specific args and parsed command-line options.
type Context struct {
	context.Context
	*App
	Command *Command
	args    *Args
	rawArgs *Args
}

func NewContext(parentCtx *cli.Context, app *App) *Context {
	return &Context{
		Context: parentCtx.Context,
		rawArgs: &Args{parentCtx.Args()},
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

func (ctx Context) WithValue(key, val any) *Context {
	ctx.Context = context.WithValue(ctx.Context, key, val)
	return &ctx
}

func (ctx *Context) Value(key any) any {
	return ctx.Context.Value(key)
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
	return ctx.rawArgs
}

// Args returns the command line arguments associated with the context.
func (ctx *Context) Args() *Args {
	return ctx.args
}
