package flags

import (
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

// ErrorHandler returns `FlagErrHandlerFunc` which takes a flag parsing error
// and tries to suggest the correct command to use with this flag. Otherwise returns the error as is.
func ErrorHandler(commands cli.Commands) cli.FlagErrHandlerFunc {
	return func(ctx *cli.Context, err error) error {
		var undefinedFlagErr cli.UndefinedFlagError
		if !errors.As(err, &undefinedFlagErr) {
			return cli.NewFatalFlagError(err)
		}

		undefArg := undefinedFlagErr.Arg

		if cmds, flag := findFlagInCommands(commands, undefArg); cmds != nil {
			var (
				flagHint = util.FirstElement(util.RemoveEmptyElements(flag.Names()))
				cmdHint  = strings.Join(cmds.Names(), " ")
			)

			if ctx.Parent().Command != nil {
				return NewCommandFlagHintError(ctx.Command.Name, undefArg, cmdHint, flagHint)
			}

			return NewGlobalFlagHintError(undefArg, cmdHint, flagHint)
		}

		return err
	}
}

func findFlagInCommands(commands cli.Commands, undefArg string) (cli.Commands, cli.Flag) {
	if len(commands) == 0 {
		return nil, nil
	}

	for _, cmd := range commands {
		for _, flag := range cmd.Flags {
			flagNames := flag.Names()

			if flag, ok := flag.(interface{ DeprecatedNames() []string }); ok {
				flagNames = append(flagNames, flag.DeprecatedNames()...)
			}

			if slices.Contains(flagNames, undefArg) {
				return cli.Commands{cmd}, flag
			}
		}

		if cmds, flag := findFlagInCommands(cmd.Subcommands, undefArg); cmds != nil {
			return append(cli.Commands{cmd}, cmds...), flag
		}
	}

	return nil, nil
}
