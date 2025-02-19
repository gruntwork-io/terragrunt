package flags

import (
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

const flagHintFmt = "flag `--%s` is not a valid flag for `%s`. Did you mean to use `%s --%s`?"

// ErrorHandler returns `FlagErrHandlerFunc` which takes a flag parsing error
// and tries to suggest the correct command to use that flag. Otherwise returns the error as is.
func ErrorHandler(commands cli.Commands) cli.FlagErrHandlerFunc {
	return func(ctx *cli.Context, err error) error {
		var undefinedFlagErr cli.UndefinedFlagError
		if !errors.As(err, &undefinedFlagErr) {
			return err
		}

		undefFlag := string(undefinedFlagErr)

		if cmds, flag := findFlagInCommands(commands, undefFlag); cmds != nil {
			flagName := util.FirstElement(util.RemoveEmptyElements(flag.Names()))

			return errors.Errorf(flagHintFmt, undefFlag, ctx.Command.Name, strings.Join(cmds.Names(), " "), flagName)
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
