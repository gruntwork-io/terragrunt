package flags

import (
	"slices"

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

		for _, cmd := range commands {
			for _, flag := range cmd.Flags {
				flagNames := flag.Names()

				if flag, ok := flag.(interface{ DeprecatedNames() []string }); ok {
					flagNames = append(flagNames, flag.DeprecatedNames()...)
				}

				if !slices.Contains(flagNames, undefFlag) {
					continue
				}

				flagName := util.FirstElement(util.RemoveEmptyElements(flag.Names()))

				return errors.Errorf(flagHintFmt, undefFlag, ctx.Command.Name, cmd.Name, flagName)
			}
		}

		return err
	}
}
