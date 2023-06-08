package cli

import (
	"strings"

	"github.com/urfave/cli/v2"
)

type Context struct {
	*cli.Context
	App *App
}

// DoublePrefixedFlags returns flags which are double prefixed, like `--debug`, `--log-level info`
func (ctx *Context) DoublePrefixedFlags() []string {
	var args []string
	var nextIsValue bool

	for _, arg := range ctx.Args().Slice() {
		if strings.HasPrefix(arg, "--") {
			args = append(args, arg)
			nextIsValue = true
		} else {
			if nextIsValue && !strings.HasPrefix(arg, "-") { // filter args with one prefix, like `-no-colors`
				args = append(args, arg)
			}
			nextIsValue = false
		}
	}

	return args
}

func NewContext(cliCtx *cli.Context, app *App) *Context {
	return &Context{
		Context: cliCtx,
		App:     app,
	}
}
