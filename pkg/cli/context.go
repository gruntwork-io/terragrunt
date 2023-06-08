package cli

import (
	"flag"
	"strings"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/urfave/cli/v2"
)

const errStrUndefined = "flag provided but not defined:"

// Context can be used to retrieve context-specific args and parsed command-line options.
type Context struct {
	*cli.Context
	undefinedFlags []string
}

// Args returns the command line arguments associated with the context.
func (ctx *Context) Args() cli.Args {
	return ctx.Context.Args()
}

// UndefinedFlags returns flags that are not defined but specified in the command.
func (ctx *Context) UndefinedFlags() []string {
	return ctx.undefinedFlags
}

func (ctx *Context) parseFlags(flags Flags) error {
	args := ctx.Args().Slice()

	for {
		flagSet, err := flags.newFlagSet("root-cmd", flag.ContinueOnError)
		if err != nil {
			return err
		}

		err = flagSet.Parse(args)
		if err == nil {
			return flags.normalize(flagSet)
		}

		// check if the error is due to an undefined flag
		var undefined string
		if errStr := err.Error(); strings.HasPrefix(errStr, errStrUndefined) {
			undefined = strings.Trim(strings.TrimPrefix(errStr, errStrUndefined), " -")
		} else {
			return errors.WithStackTrace(err)
		}

		// regenerate the initial args without undefined
		argsWereSplit := false
		for i, arg := range args {
			if strings.Trim(arg, "-") == undefined {
				ctx.undefinedFlags = append(ctx.undefinedFlags, arg)
				args = append(args[:i], args[i+1:]...)

				argsWereSplit = true
				break
			}
		}

		// This should be an impossible to reach code path, but in case the arg
		// splitting failed to happen, this will prevent infinite loops
		if !argsWereSplit {
			return err
		}
	}
}

func NewContext(cliCtx *cli.Context) *Context {
	return &Context{Context: cliCtx}
}
