//nolint:unparam
package cli

import (
	"fmt"
	"strings"

	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
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
	CommandNameSpinUp:      replaceDeprecatedCommandFunc(runall.CommandName, terraform.CommandNameApply),
	CommandNameTearDown:    replaceDeprecatedCommandFunc(runall.CommandName, terraform.CommandNameDestroy),
	CommandNameApplyAll:    replaceDeprecatedCommandFunc(runall.CommandName, terraform.CommandNameApply),
	CommandNameDestroyAll:  replaceDeprecatedCommandFunc(runall.CommandName, terraform.CommandNameDestroy),
	CommandNamePlanAll:     replaceDeprecatedCommandFunc(runall.CommandName, terraform.CommandNamePlan),
	CommandNameValidateAll: replaceDeprecatedCommandFunc(runall.CommandName, terraform.CommandNameValidate),
	CommandNameOutputAll:   replaceDeprecatedCommandFunc(runall.CommandName, terraform.CommandNameOutput),
}

type replaceDeprecatedCommandFuncType func(opts *options.TerragruntOptions) func(ctx *cli.Context) error

// replaceDeprecatedCommandFunc returns the `Action` function of the replacement command that is assigned to the deprecated command.
func replaceDeprecatedCommandFunc(terragruntCommandName, terraformCommandName string) replaceDeprecatedCommandFuncType {
	return func(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
		return func(ctx *cli.Context) error {
			command := ctx.App.Commands.Get(terragruntCommandName)
			args := append([]string{terraformCommandName}, ctx.Args().Slice()...)

			deprecatedCommandName := ctx.Command.Name
			newCommandFriendly := fmt.Sprintf("terragrunt %s %s", terragruntCommandName, strings.Join(args, " "))

			opts.Logger.Warnf(
				"'%s' is deprecated. Running '%s' instead. Please update your workflows to use '%s', as '%s' may be removed in the future!\n",
				deprecatedCommandName,
				newCommandFriendly,
				newCommandFriendly,
				deprecatedCommandName,
			)

			err := command.Run(ctx, args...)
			return err
		}
	}
}

func deprecatedCommands(opts *options.TerragruntOptions) cli.Commands {
	var commands cli.Commands

	for commandName, runFunc := range replaceDeprecatedCommandsFuncs {
		runFunc := runFunc

		command := &cli.Command{
			Name:   commandName,
			Hidden: true,
			Action: runFunc(opts),
		}
		commands = append(commands, command)
	}

	return commands
}
