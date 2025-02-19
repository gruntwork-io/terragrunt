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
			return err
		}

		undefFlag := string(undefinedFlagErr)

		if cmds, flag := findFlagInCommands(commands, undefFlag); cmds != nil {
			var (
				flagHint = util.FirstElement(util.RemoveEmptyElements(flag.Names()))
				cmdHint  = strings.Join(cmds.Names(), " ")
			)

			if ctx.Parent().Command == nil {
				return NewGlobalFlagHintError(undefFlag, cmdHint, flagHint)
			}

			return NewCommandFlagHintError(ctx.Command.Name, undefFlag, cmdHint, flagHint)
		}

		return err
	}
}

func findFlagInCommands(commands cli.Commands, undefFlag string) (cli.Commands, cli.Flag) {
	for _, cmd := range commands {
		for _, flag := range cmd.Flags {
			flagNames := flag.Names()

			if flag, ok := flag.(interface{ DeprecatedNames() []string }); ok {
				flagNames = append(flagNames, flag.DeprecatedNames()...)
			}

			if slices.Contains(flagNames, undefFlag) {
				return cli.Commands{cmd}, flag
			}
		}

		if cmd.Subcommands != nil {
			if cmds, flag := findFlagInCommands(cmd.Subcommands, undefFlag); cmds != nil {
				return append(cli.Commands{cmd}, cmds...), flag
			}
		}
	}
	return nil, nil
}
