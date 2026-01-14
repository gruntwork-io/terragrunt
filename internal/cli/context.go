package cli

// Context can be used to retrieve context-specific args and parsed command-line options.
type Context struct {
	*App
	Command       *Command
	parent        *Context
	args          Args
	shellComplete bool
}

func NewAppContext(app *App, args Args) *Context {
	return &Context{
		App:  app,
		args: args,
	}
}

func (ctx *Context) NewCommandContext(command *Command, args Args) *Context {
	return &Context{
		App:           ctx.App,
		Command:       command,
		parent:        ctx,
		args:          args,
		shellComplete: ctx.shellComplete,
	}
}

func (ctx *Context) Parent() *Context {
	return ctx.parent
}

// Args returns the command line arguments associated with the context.
func (ctx *Context) Args() Args {
	return ctx.args
}

// Flag retrieves a command flag by name. Returns nil if the command is not set
// or if the flag doesn't exist.
func (ctx *Context) Flag(name string) Flag {
	if ctx.Command != nil {
		return ctx.Command.Flags.Get(name)
	}

	return nil
}
