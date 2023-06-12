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
var deprecatedCommandsActionFuncs = map[string]deprecatedCommandActionFuncType{
	CommandNameSpinUp:      redirectDeprecatedCommandAll(runall.CommandName, terraform.CommandNameApply),
	CommandNameTearDown:    redirectDeprecatedCommandAll(runall.CommandName, terraform.CommandNameDestroy),
	CommandNameApplyAll:    redirectDeprecatedCommandAll(runall.CommandName, terraform.CommandNameApply),
	CommandNameDestroyAll:  redirectDeprecatedCommandAll(runall.CommandName, terraform.CommandNameDestroy),
	CommandNamePlanAll:     redirectDeprecatedCommandAll(runall.CommandName, terraform.CommandNamePlan),
	CommandNameValidateAll: redirectDeprecatedCommandAll(runall.CommandName, terraform.CommandNameValidate),
	CommandNameOutputAll:   redirectDeprecatedCommandAll(runall.CommandName, terraform.CommandNameOutput),
}

type deprecatedCommandActionFuncType func(opts *options.TerragruntOptions) func(ctx *cli.Context) error

func redirectDeprecatedCommandAll(newTerragruntCommandName, newTerraformCommandName string) deprecatedCommandActionFuncType {
	return func(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
		return func(ctx *cli.Context) error {
			newCommand := ctx.App.Commands.Get(newTerragruntCommandName)
			newArgs := append([]string{newTerraformCommandName}, ctx.Args().Slice()...)
			newCtx := ctx.Clone(newCommand, newArgs)

			deprecatedCommandName := ctx.Command.Name
			newCommandFriendly := fmt.Sprintf("terragrunt %s %s", newTerragruntCommandName, strings.Join(newArgs, " "))

			opts.Logger.Warnf(
				"'%s' is deprecated. Running '%s' instead. Please update your workflows to use '%s', as '%s' may be removed in the future!\n",
				deprecatedCommandName,
				newCommandFriendly,
				newCommandFriendly,
				deprecatedCommandName,
			)

			if newCommand.Before != nil {
				if err := newCommand.Before(newCtx); err != nil {
					return err
				}
			}

			if newCommand.Action != nil {
				if err := newCommand.Action(newCtx); err != nil {
					return err
				}
			}

			if newCommand.After != nil {
				if err := newCommand.After(newCtx); err != nil {
					return err
				}
			}

			return nil
		}
	}
}

func newDeprecatedCommands(opts *options.TerragruntOptions) cli.Commands {
	var commands cli.Commands

	for commandName, actionFunc := range deprecatedCommandsActionFuncs {
		actionFunc := actionFunc

		command := &cli.Command{
			Name:   commandName,
			Hidden: true,
			Action: actionFunc(opts),
		}

		commands = append(commands, command)
	}

	return commands
}
