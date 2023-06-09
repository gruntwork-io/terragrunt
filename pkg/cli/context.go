package cli

import (
	"github.com/urfave/cli/v2"
)

// Context can be used to retrieve context-specific args and parsed command-line options.
type Context struct {
	*cli.Context
	*App
	args []string
}

// Args returns the command line arguments associated with the context.
func (ctx *Context) RawArgs() cli.Args {
	return ctx.Context.Args()
}

// Args returns the command line arguments associated with the context.
func (ctx *Context) Args() []string {
	return ctx.args
}
