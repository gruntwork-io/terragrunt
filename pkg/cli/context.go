package cli

import (
	"flag"
	"strings"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/urfave/cli/v2"
)

const errFlagUndefined = "flag provided but not defined:"

// Context can be used to retrieve context-specific args and parsed command-line options.
type Context struct {
	*cli.Context
	*App
	undefinedArgs []string
}

// Args returns the command line arguments associated with the context.
func (ctx *Context) Args() cli.Args {
	return ctx.Context.Args()
}

// UndefinedArgs returns flags that are not defined but specified in the command.
func (ctx *Context) UndefinedArgs() []string {
	return ctx.undefinedArgs
}

func (ctx *Context) parseFlags(flags Flags) error {
	filterArgs := func(args []string) []string {
		for i, arg := range args {
			if arg[0] == '-' {
				args = args[i:]
				break
			}
			ctx.undefinedArgs = append(ctx.undefinedArgs, arg)
		}
		return args
	}

	args := filterArgs(ctx.Args().Slice())

	for {
		flagSet, err := flags.newFlagSet("root-cmd", flag.ContinueOnError)
		if err != nil {
			return err
		}

		err = flagSet.Parse(args)
		if err == nil {
			ctx.undefinedArgs = append(ctx.undefinedArgs, flagSet.Args()...)
			return flags.normalize(flagSet)
		}

		// check if the error is due to an undefined flag
		var undefined string
		if errStr := err.Error(); strings.HasPrefix(errStr, errFlagUndefined) {
			undefined = strings.Trim(strings.TrimPrefix(errStr, errFlagUndefined), " -")
		} else {
			return errors.WithStackTrace(err)
		}

		// regenerate the initial args without undefined
		argsRegenerated := false
		for i, arg := range args {
			if trimmed := strings.Trim(arg, "-"); trimmed == undefined {
				ctx.undefinedArgs = append(ctx.undefinedArgs, arg)
				args = filterArgs(append(args[:i], args[i+1:]...))
				argsRegenerated = true
				break
			}

		}

		// This should be an impossible to reach code path, but in case the arg
		// splitting failed to happen, this will prevent infinite loops
		if !argsRegenerated {
			return err
		}
	}
}

func NewContext(cliCtx *cli.Context, app *App) *Context {
	return &Context{Context: cliCtx, App: app}
}
