package commands

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
var replaceDeprecatedCommandsFuncs = map[string]deprecatedCommandActionFuncType{
	CommandNameSpinUp:      replaceDeprecatedCommand(runall.CommandName, terraform.CommandNameApply),
	CommandNameTearDown:    replaceDeprecatedCommand(runall.CommandName, terraform.CommandNameDestroy),
	CommandNameApplyAll:    replaceDeprecatedCommand(runall.CommandName, terraform.CommandNameApply),
	CommandNameDestroyAll:  replaceDeprecatedCommand(runall.CommandName, terraform.CommandNameDestroy),
	CommandNamePlanAll:     replaceDeprecatedCommand(runall.CommandName, terraform.CommandNamePlan),
	CommandNameValidateAll: replaceDeprecatedCommand(runall.CommandName, terraform.CommandNameValidate),
	CommandNameOutputAll:   replaceDeprecatedCommand(runall.CommandName, terraform.CommandNameOutput),
}

type deprecatedCommandActionFuncType func(opts *options.TerragruntOptions) func(ctx *cli.Context) error

func replaceDeprecatedCommand(newTerragruntCommandName, newTerraformCommandName string) deprecatedCommandActionFuncType {
	return func(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
		return func(ctx *cli.Context) error {
			newCommand := ctx.App.Commands.Get(newTerragruntCommandName)
			newArgs := append([]string{newTerraformCommandName}, ctx.Args().Slice()...)
			newCtx, err := ctx.ParseArgs(newCommand, newArgs)
			if err != nil {
				return err
			}

			deprecatedCommandName := ctx.Command.Name
			newCommandFriendly := fmt.Sprintf("terragrunt %s %s", newTerragruntCommandName, strings.Join(newArgs, " "))

			opts.Logger.Warnf(
				"'%s' is deprecated. Running '%s' instead. Please update your workflows to use '%s', as '%s' may be removed in the future!\n",
				deprecatedCommandName,
				newCommandFriendly,
				newCommandFriendly,
				deprecatedCommandName,
			)

			err = newCommand.Run(newCtx)
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
		}
		commands = append(commands, command)
	}

	return commands
}
