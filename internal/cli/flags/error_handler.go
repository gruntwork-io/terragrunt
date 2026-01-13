package flags

import (
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

// ErrorHandler returns `FlagErrHandlerFunc` which takes a flag parsing error
// and tries to suggest the correct command to use with this flag. Otherwise returns the error as is.
func ErrorHandler(commands clihelper.Commands) clihelper.FlagErrHandlerFunc {
	return func(ctx *clihelper.Context, err error) error {
		var undefinedFlagErr clihelper.UndefinedFlagError
		if !errors.As(err, &undefinedFlagErr) {
			return err
		}

		undefFlag := string(undefinedFlagErr)

		if cmds, flag := findFlagInCommands(commands, undefFlag); cmds != nil {
			var (
				flagHint = util.FirstElement(util.RemoveEmptyElements(flag.Names()))
				cmdHint  = strings.Join(cmds.Names(), " ")
			)

			if ctx.Parent().Command != nil {
				return NewCommandFlagHintError(ctx.Command.Name, undefFlag, cmdHint, flagHint)
			}

			return NewGlobalFlagHintError(undefFlag, cmdHint, flagHint)
		}

		return err
	}
}

func findFlagInCommands(commands clihelper.Commands, undefFlag string) (clihelper.Commands, clihelper.Flag) {
	if len(commands) == 0 {
		return nil, nil
	}

	for _, cmd := range commands {
		for _, flag := range cmd.Flags {
			flagNames := flag.Names()

			if flag, ok := flag.(interface{ DeprecatedNames() []string }); ok {
				flagNames = append(flagNames, flag.DeprecatedNames()...)
			}

			if slices.Contains(flagNames, undefFlag) {
				return clihelper.Commands{cmd}, flag
			}
		}

		if cmds, flag := findFlagInCommands(cmd.Subcommands, undefFlag); cmds != nil {
			return append(clihelper.Commands{cmd}, cmds...), flag
		}
	}

	return nil, nil
}
