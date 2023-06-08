package cli

import (
	"strings"

	"github.com/urfave/cli/v2"
)

type Args struct {
	cli.Args
	App *App
}

// DoublePrefixed returns double-prefixed args as the first value, eg `--debug`, `--log-level info`, and the rest as the second value.
func (args *Args) DoublePrefixed() ([]string, []string) {
	var doublePrefixedArgs, restArgs []string
	var nextIsValue bool

	for _, arg := range args.Slice() {
		if arg != "--" && strings.HasPrefix(arg, "--") {
			doublePrefixedArgs = append(doublePrefixedArgs, arg)

			flagName := arg[2:]
			if flag := args.App.Flags.BoolFlags().Get(flagName); flag == nil {
				nextIsValue = true
			}
		} else {
			if nextIsValue && !strings.HasPrefix(arg, "-") { // filter args with one prefix, like `-no-colors`
				doublePrefixedArgs = append(doublePrefixedArgs, arg)
			}
			nextIsValue = false
		}

		restArgs = append(restArgs, arg)
	}

	return doublePrefixedArgs, restArgs
}
