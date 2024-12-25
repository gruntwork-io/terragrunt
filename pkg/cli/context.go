package cli

import (
	"context"
)

// Context can be used to retrieve context-specific args and parsed command-line options.
type Context struct {
	context.Context
	*App
	Command       *Command
	parent        *Context
	args          Args
	nonAppArgs    Args
	shellComplete bool
}

func NewAppContext(ctx context.Context, app *App, args, nonAppArgs Args) *Context {
	return &Context{
		Context:    ctx,
		App:        app,
		args:       args,
		nonAppArgs: nonAppArgs,
	}
}

func (ctx *Context) NewCommandContext(command *Command, args Args) *Context {
	return &Context{
		Context:       ctx.Context,
		App:           ctx.App,
		nonAppArgs:    ctx.nonAppArgs,
		parent:        ctx,
		Command:       command,
		args:          args,
		shellComplete: ctx.shellComplete,
	}
}

func (ctx *Context) Parent() *Context {
	return ctx.parent
}

func (ctx *Context) WithValue(key, val any) *Context {
	newCtx := *ctx
	newCtx.Context = context.WithValue(newCtx.Context, key, val)

	return &newCtx
}

func (ctx *Context) Value(key any) any {
	return ctx.Context.Value(key)
}

// Args returns the command line arguments associated with the context.
func (ctx *Context) Args() Args {
	return ctx.args
}

// NonAppArgs returns the non-app args.
// https://www.gnu.org/software/bash/manual/html_node/Shell-Builtin-Commands.html
func (ctx *Context) NonAppArgs() Args {
	return ctx.nonAppArgs
}
