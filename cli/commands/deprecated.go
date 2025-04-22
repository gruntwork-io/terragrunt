//nolint:unparam
package commands

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/cli/commands/hcl"
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/validate"
	"github.com/gruntwork-io/terragrunt/cli/commands/render"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf"
)

// The following commands are DEPRECATED
const (
	CommandSpinUpName      = "spin-up"
	CommandTearDownName    = "tear-down"
	CommandPlanAllName     = "plan-all"
	CommandApplyAllName    = "apply-all"
	CommandDestroyAllName  = "destroy-all"
	CommandOutputAllName   = "output-all"
	CommandValidateAllName = "validate-all"

	CommandHCLFmtName         = "hclfmt"
	CommandHCLValidateName    = "hclvalidate"
	CommandValidateInputsName = "validate-inputs"
	CommandRenderJSONName     = "render-json"
)

// deprecatedCommands is a map of deprecated commands to a handler that knows how to convert the command to the known
// alternative. The handler should return the new TerragruntOptions (if any modifications are needed) and command
// string.
var replaceDeprecatedCommandsFuncs = map[string]replaceDeprecatedCommandFuncType{
	CommandSpinUpName:         replaceDeprecatedCommandFunc(controls.LegacyAll, cli.Args{runall.CommandName, tf.CommandNameApply}),
	CommandTearDownName:       replaceDeprecatedCommandFunc(controls.LegacyAll, cli.Args{runall.CommandName, tf.CommandNameDestroy}),
	CommandApplyAllName:       replaceDeprecatedCommandFunc(controls.LegacyAll, cli.Args{runall.CommandName, tf.CommandNameApply}),
	CommandDestroyAllName:     replaceDeprecatedCommandFunc(controls.LegacyAll, cli.Args{runall.CommandName, tf.CommandNameDestroy}),
	CommandPlanAllName:        replaceDeprecatedCommandFunc(controls.LegacyAll, cli.Args{runall.CommandName, tf.CommandNamePlan}),
	CommandValidateAllName:    replaceDeprecatedCommandFunc(controls.LegacyAll, cli.Args{runall.CommandName, tf.CommandNameValidate}),
	CommandOutputAllName:      replaceDeprecatedCommandFunc(controls.LegacyAll, cli.Args{runall.CommandName, tf.CommandNameOutput}),
	CommandHCLFmtName:         replaceDeprecatedCommandFunc(controls.CLIRedesign, cli.Args{hcl.CommandName, format.CommandName}),
	CommandHCLValidateName:    replaceDeprecatedCommandFunc(controls.CLIRedesign, cli.Args{hcl.CommandName, validate.CommandName}),
	CommandValidateInputsName: replaceDeprecatedCommandFunc(controls.CLIRedesign, cli.Args{hcl.CommandName, validate.CommandName, "--" + validate.InputsFlagName}),
	CommandRenderJSONName:     replaceDeprecatedCommandFunc(controls.CLIRedesign, cli.Args{render.CommandName, "--" + render.JSONFlagName, "--" + render.WriteFlagName}),
}

type replaceDeprecatedCommandFuncType func(opts *options.TerragruntOptions, deprecatedCommandName string) cli.ActionFunc

// replaceDeprecatedCommandFunc returns the `Action` function of the replacement command that is assigned to the deprecated command.
func replaceDeprecatedCommandFunc(strictControlName string, args cli.Args) replaceDeprecatedCommandFuncType {
	return func(opts *options.TerragruntOptions, deprecatedCommandName string) cli.ActionFunc {
		newCommandName := fmt.Sprintf("terragrunt %s", args)

		control := controls.NewDeprecatedCommand(deprecatedCommandName, newCommandName)
		opts.StrictControls.FilterByNames(controls.DeprecatedCommands, strictControlName, deprecatedCommandName).AddSubcontrolsToCategory(controls.RunAllCommandsCategoryName, control)

		return func(ctx *cli.Context) error {
			if err := control.Evaluate(ctx); err != nil {
				return cli.NewExitError(err, cli.ExitCodeGeneralError)
			}

			args := append(args, ctx.Args().Slice()...)
			return ctx.App.NewRootCommand().Run(ctx, args)
		}
	}
}

func NewDeprecatedCommands(opts *options.TerragruntOptions) cli.Commands {
	var commands cli.Commands

	for commandName, runFunc := range replaceDeprecatedCommandsFuncs {
		command := &cli.Command{
			Name:       commandName,
			Hidden:     true,
			CustomHelp: cli.ShowAppHelp,
			Action:     runFunc(opts, commandName),
			Flags:      run.NewFlags(opts, nil),
		}
		commands = append(commands, command)
	}

	return commands
}
