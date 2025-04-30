//nolint:unparam
package commands

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl"
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/validate"
	"github.com/gruntwork-io/terragrunt/cli/commands/render"
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

	CommandRunAllName             = "run-all"
	CommandGraphName              = "graph"
	CommandHCLFmtName             = "hclfmt"
	CommandHCLValidateName        = "hclvalidate"
	CommandValidateInputsName     = "validate-inputs"
	CommandRenderJSONName         = "render-json"
	CommandTerragruntInfoName     = "terragrunt-info"
	CommandOutputModuleGroupsName = "output-module-groups"
	CommandGraphDependenciesName  = "graph-dependencies"
)

// NewDeprecatedCommands returns a slice of deprecated commands to convert the command to the known alternative.
func NewDeprecatedCommands(opts *options.TerragruntOptions) cli.Commands {
	deprecatedCommands := DeprecatedCommands{
		// legacy-all commands
		newDeprecatedLegacyAllCommand(CommandSpinUpName, tf.CommandNameApply),
		newDeprecatedLegacyAllCommand(CommandTearDownName, tf.CommandNameDestroy),
		newDeprecatedLegacyAllCommand(CommandPlanAllName, tf.CommandNamePlan),
		newDeprecatedLegacyAllCommand(CommandApplyAllName, tf.CommandNameApply),
		newDeprecatedLegacyAllCommand(CommandDestroyAllName, tf.CommandNameDestroy),
		newDeprecatedLegacyAllCommand(CommandValidateAllName, tf.CommandNameValidate),
		newDeprecatedLegacyAllCommand(CommandOutputAllName, tf.CommandNameOutput),

		// `hclfmt`
		newDeprecatedCLIRedesignCommand(CommandHCLFmtName, cli.Args{
			hcl.CommandName, format.CommandName}),
		// `hclvaliate`
		newDeprecatedCLIRedesignCommand(CommandHCLValidateName, cli.Args{
			hcl.CommandName, validate.CommandName}),
		// `validate-inputs`
		newDeprecatedCLIRedesignCommand(CommandValidateInputsName, cli.Args{
			hcl.CommandName, validate.CommandName,
			"--" + validate.InputsFlagName}),
		// `render-json`
		newDeprecatedCLIRedesignCommand(CommandRenderJSONName, cli.Args{
			render.CommandName,
			"--" + render.JSONFlagName,
			"--" + render.WriteFlagName,
			"--" + render.OutFlagName, "terragrunt_rendered.json"}),
	}

	return deprecatedCommands.CLICommands(opts)
}

func newDeprecatedLegacyAllCommand(deprecatedCommandName, tfCommandName string) *DeprecatedCommand {
	return &DeprecatedCommand{
		commandName: deprecatedCommandName,
		// we can't recoomand to use `run --all plan/apply/...` as alternative for `*-all` commands
		// because `run` command doesn't allow TF flags to be specified before `--` separator.
		replaceWithArgs: cli.Args{tfCommandName, "--" + runall.AllFlagName},
		controlName:     controls.LegacyAll,
		controlCategory: controls.RunAllCommandsCategoryName,
	}
}

func newDeprecatedCLIRedesignCommand(deprecatedCommandName string, replaceWithArgs cli.Args, subcommands ...*DeprecatedCommand) *DeprecatedCommand {
	cmd := &DeprecatedCommand{
		subcommands:     subcommands,
		commandName:     deprecatedCommandName,
		replaceWithArgs: replaceWithArgs,
		controlName:     controls.CLIRedesign,
		controlCategory: controls.CLIRedesignCommandsCategoryName,
	}

	for _, subCmd := range subcommands {
		subCmd.parentCommand = cmd
	}

	return cmd
}

type DeprecatedCommands []*DeprecatedCommand

func (deps DeprecatedCommands) CLICommands(opts *options.TerragruntOptions) cli.Commands {
	var commands = make(cli.Commands, len(deps))

	for i, dep := range deps {
		commands[i] = dep.CLICommand(opts)
	}

	return commands
}

type DeprecatedCommand struct {
	commandName     string
	controlName     string
	controlCategory string
	subcommands     DeprecatedCommands
	parentCommand   *DeprecatedCommand
	replaceWithArgs cli.Args
}

func (dep DeprecatedCommand) CLICommand(opts *options.TerragruntOptions) *cli.Command {
	newCommand := "terragrunt " + dep.replaceWithArgs.String()
	depCommand := dep.commandName

	if dep.parentCommand != nil {
		depCommand = dep.parentCommand.commandName + " " + depCommand
	}

	control := controls.NewDeprecatedReplacedCommand(depCommand, newCommand)
	opts.StrictControls.FilterByNames(controls.DeprecatedCommands, dep.controlName, dep.commandName).AddSubcontrolsToCategory(dep.controlCategory, control)

	return &cli.Command{
		Name:                         dep.commandName,
		Subcommands:                  dep.subcommands.CLICommands(opts),
		CustomHelp:                   cli.ShowAppHelp,
		DisabledErrorOnUndefinedFlag: true,
		Hidden:                       true,
		Action: func(ctx *cli.Context) error {
			if err := control.Evaluate(ctx); err != nil {
				return cli.NewExitError(err, cli.ExitCodeGeneralError)
			}

			// Since users can specify the same arguments that are already specified in the `replaceWith Args` slice,
			// we need to disable the check for multiple values set for the same flag.
			// This is a minor compromise that has virtually no impact on anything.
			cmd := ctx.App.NewRootCommand().DisableErrorOnMultipleSetFlag()
			args := append(dep.replaceWithArgs, ctx.Args().Slice()...)

			return cmd.Run(ctx, args)
		},
	}
}
