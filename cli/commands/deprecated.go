//nolint:unparam
package commands

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf"
)

// The following commands are DEPRECATED
const (
	CommandNameSpinUp      = "spin-up"
	CommandNameTearDown    = "tear-down"
	CommandNamePlanAll     = "plan-all"
	CommandNameApplyAll    = "apply-all"
	CommandNameDestroyAll  = "destroy-all"
	CommandNameOutputAll   = "output-all"
	CommandNameValidateAll = "validate-all"
)

// deprecatedCommands is a map of deprecated commands to a handler that knows how to convert the command to the known
// alternative. The handler should return the new TerragruntOptions (if any modifications are needed) and command
// string.
var replaceDeprecatedCommandsFuncs = map[string]replaceDeprecatedCommandFuncType{
	CommandNameSpinUp:      replaceDeprecatedCommandFunc(runall.CommandName, tf.CommandNameApply),
	CommandNameTearDown:    replaceDeprecatedCommandFunc(runall.CommandName, tf.CommandNameDestroy),
	CommandNameApplyAll:    replaceDeprecatedCommandFunc(runall.CommandName, tf.CommandNameApply),
	CommandNameDestroyAll:  replaceDeprecatedCommandFunc(runall.CommandName, tf.CommandNameDestroy),
	CommandNamePlanAll:     replaceDeprecatedCommandFunc(runall.CommandName, tf.CommandNamePlan),
	CommandNameValidateAll: replaceDeprecatedCommandFunc(runall.CommandName, tf.CommandNameValidate),
	CommandNameOutputAll:   replaceDeprecatedCommandFunc(runall.CommandName, tf.CommandNameOutput),
}

type replaceDeprecatedCommandFuncType func(opts *options.TerragruntOptions) cli.ActionFunc

// replaceDeprecatedCommandFunc returns the `Action` function of the replacement command that is assigned to the deprecated command.
func replaceDeprecatedCommandFunc(terragruntCommandName, terraformCommandName string) replaceDeprecatedCommandFuncType {
	return func(opts *options.TerragruntOptions) cli.ActionFunc {
		return func(ctx *cli.Context) error {
			command := ctx.App.Commands.Get(terragruntCommandName)
			args := append([]string{terraformCommandName}, ctx.Args().Slice()...)

			deprecatedCommandName := ctx.Command.Name
			newCommandFriendly := fmt.Sprintf("terragrunt %s %s", terragruntCommandName, strings.Join(args, " "))

			control, ok := strict.GetStrictControl(deprecatedCommandName)
			if ok {
				warning, triggered, err := control.Evaluate(opts)
				if err != nil {
					return err //nolint:wrapcheck
				}

				if !triggered {
					opts.Logger.Warn(warning)
				}

			} else { //nolint:wsl,whitespace
				// This else clause should never be hit, as all the commands above are accounted for.
				// This might be missed accidentally in the future, so we'll keep it for safety.
				opts.Logger.Warnf(
					"'%s' is deprecated. Running '%s' instead. Please update your workflows to use '%s', as '%s' may be removed in the future!\n", //nolint:lll
					deprecatedCommandName,
					newCommandFriendly,
					newCommandFriendly,
					deprecatedCommandName,
				)
			}

			err := command.Run(ctx, args)

			return err
		}
	}
}

func NewDeprecatedCommands(opts *options.TerragruntOptions) cli.Commands {
	var commands cli.Commands

	for commandName, runFunc := range replaceDeprecatedCommandsFuncs {
		runFunc := runFunc

		command := &cli.Command{
			Name:   commandName,
			Hidden: true,
			Action: runFunc(opts),
			Flags:  run.NewFlags(opts),
		}
		commands = append(commands, command)
	}

	return commands
}
