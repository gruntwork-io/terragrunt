//nolint:unparam
package cli

import (
	"fmt"
	"strings"

	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/terraform"
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

//nolint:lll,gochecknoglobals,stylecheck
var StrictControls = strict.Controls{
	CommandNameSpinUp: {
		Error:   errors.New("The `spin-up` command is no longer supported. Use `terragrunt run-all apply` instead."),
		Warning: "The `spin-up` command is deprecated and will be removed in a future version. Use `terragrunt run-all apply` instead.",
	},
	CommandNameTearDown: {
		Error:   errors.New("The `tear-down` command is no longer supported. Use `terragrunt run-all destroy` instead."),
		Warning: "The `tear-down` command is deprecated and will be removed in a future version. Use `terragrunt run-all destroy` instead.",
	},
	CommandNamePlanAll: {
		Error:   errors.New("The `plan-all` command is no longer supported. Use `terragrunt run-all plan` instead."),
		Warning: "The `plan-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all plan` instead.",
	},
	CommandNameApplyAll: {
		Error:   errors.New("The `apply-all` command is no longer supported. Use `terragrunt run-all apply` instead."),
		Warning: "The `apply-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all apply` instead.",
	},
	CommandNameDestroyAll: {
		Error:   errors.New("The `destroy-all` command is no longer supported. Use `terragrunt run-all destroy` instead."),
		Warning: "The `destroy-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all destroy` instead.",
	},
	CommandNameOutputAll: {
		Error:   errors.New("The `output-all` command is no longer supported. Use `terragrunt run-all output` instead."),
		Warning: "The `output-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all output` instead.",
	},
	CommandNameValidateAll: {
		Error:   errors.New("The `validate-all` command is no longer supported. Use `terragrunt run-all validate` instead."),
		Warning: "The `validate-all` command is deprecated and will be removed in a future version. Use `terragrunt run-all validate` instead.",
	},
}

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

type replaceDeprecatedCommandFuncType func(opts *options.TerragruntOptions) cli.ActionFunc

// replaceDeprecatedCommandFunc returns the `Action` function of the replacement command that is assigned to the deprecated command.
func replaceDeprecatedCommandFunc(terragruntCommandName, terraformCommandName string) replaceDeprecatedCommandFuncType {
	return func(opts *options.TerragruntOptions) cli.ActionFunc {
		return func(ctx *cli.Context) error {
			command := ctx.App.Commands.Get(terragruntCommandName)
			args := append([]string{terraformCommandName}, ctx.Args().Slice()...)

			deprecatedCommandName := ctx.Command.Name
			newCommandFriendly := fmt.Sprintf("terragrunt %s %s", terragruntCommandName, strings.Join(args, " "))

			// This else clause should never be hit, as all the commands above are accounted for.
			// This might be missed accidentally in the future, so we'll keep it for safety.
			opts.Logger.Warnf(
				"'%s' is deprecated. Running '%s' instead. Please update your workflows to use '%s', as '%s' may be removed in the future!\n", //nolint:lll
				deprecatedCommandName,
				newCommandFriendly,
				newCommandFriendly,
				deprecatedCommandName,
			)

			err := command.Run(ctx, args)

			return err
		}
	}
}

func DeprecatedCommands(opts *options.TerragruntOptions) cli.Commands {
	var commands cli.Commands

	for commandName, runFunc := range replaceDeprecatedCommandsFuncs {

		command := &cli.Command{
			Name:   commandName,
			Hidden: true,
			Action: runFunc(opts),
		}

		if control, ok := StrictControls.Get(commandName); ok {
			strictCommand := strict.Command{
				Command: command,
				Control: control,
			}

			commands = append(commands, strictCommand.CLICommand(opts))

			opts.RegisteredStrictControls = append(opts.RegisteredStrictControls, commandName)
		}

		commands = append(commands, command)
	}

	return commands
}
