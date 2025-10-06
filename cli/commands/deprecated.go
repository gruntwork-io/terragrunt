//nolint:unparam
package commands

import (
	"slices"

	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/dag"
	daggraph "github.com/gruntwork-io/terragrunt/cli/commands/dag/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl"
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/validate"
	"github.com/gruntwork-io/terragrunt/cli/commands/render"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
)

// The following commands are DEPRECATED
const (
	CommandRunAllName            = "run-all"
	CommandGraphName             = "graph"
	CommandHCLFmtName            = "hclfmt"
	CommandHCLValidateName       = "hclvalidate"
	CommandValidateInputsName    = "validate-inputs"
	CommandRenderJSONName        = "render-json"
	CommandGraphDependenciesName = "graph-dependencies"
)

// NewDeprecatedCommands returns a slice of deprecated commands to convert the command to the known alternative.
func NewDeprecatedCommands(l log.Logger, opts *options.TerragruntOptions) cli.Commands {
	deprecatedCommands := DeprecatedCommands{
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
		// `graph-dependencies`
		newDeprecatedCLIRedesignCommand(CommandGraphDependenciesName, cli.Args{
			dag.CommandName, daggraph.CommandName}),
		// `render-json`
		newDeprecatedCLIRedesignCommand(CommandRenderJSONName, cli.Args{
			render.CommandName,
			"--" + render.JSONFlagName,
			"--" + render.WriteFlagName,
			"--" + render.OutFlagName, "terragrunt_rendered.json"}),

		// `run-all` commands
		newDeprecatedCLIRedesignCommand(CommandRunAllName, cli.Args{run.CommandName, "--" + runall.AllFlagName},
			append(DeprecatedCommands{
				// `run-all hclfmt`
				newDeprecatedCLIRedesignCommand(CommandHCLFmtName,
					cli.Args{hcl.CommandName, format.CommandName,
						"--" + runall.AllFlagName}),
				// `run-all hclvalidate`
				newDeprecatedCLIRedesignCommand(CommandHCLValidateName, cli.Args{
					hcl.CommandName, validate.CommandName,
					"--" + runall.AllFlagName}),
				// `run-all validate-inputs`
				newDeprecatedCLIRedesignCommand(CommandValidateInputsName, cli.Args{
					hcl.CommandName, validate.CommandName,
					"--" + runall.AllFlagName,
					"--" + validate.InputsFlagName}),
				// `run-all render-json`
				newDeprecatedCLIRedesignCommand(CommandRenderJSONName, cli.Args{
					render.CommandName,
					"--" + runall.AllFlagName,
					"--" + render.JSONFlagName,
					"--" + render.WriteFlagName,
					"--" + render.OutFlagName, "terragrunt_rendered.json"}),
				// `run-all render`
				newDeprecatedCLIRedesignCommand(render.CommandName, cli.Args{
					render.CommandName,
					"--" + runall.AllFlagName}),
				// `run-all aws-provider-patch`
				newDeprecatedCLIRedesignCommand(awsproviderpatch.CommandName, cli.Args{
					awsproviderpatch.CommandName,
					"--" + runall.AllFlagName}),
				// `run-all graph-dependencies`
				newDeprecatedCLIRedesignCommand(CommandGraphDependenciesName, cli.Args{
					dag.CommandName, daggraph.CommandName,
					"--" + runall.AllFlagName}),
			},
				// `run-all plan/apply/...`
				newDeprecatedCLIRedesignTFCommands(cli.Args{"--" + runall.AllFlagName})...,
			)...,
		),

		// `graph` commands
		newDeprecatedCLIRedesignCommand(CommandGraphName, cli.Args{run.CommandName, "--" + render.JSONFlagName},
			append(DeprecatedCommands{
				// `graph render-json`
				newDeprecatedCLIRedesignCommand(CommandRenderJSONName, cli.Args{
					render.CommandName,
					"--" + graph.GraphFlagName,
					"--" + render.JSONFlagName,
					"--" + render.WriteFlagName,
					"--" + render.OutFlagName, "terragrunt_rendered.json"}),
				// `graph render`
				newDeprecatedCLIRedesignCommand(render.CommandName, cli.Args{
					render.CommandName,
					"--" + graph.GraphFlagName}),
			},
				// `graph plan/apply/...`
				newDeprecatedCLIRedesignTFCommands(cli.Args{"--" + graph.GraphFlagName})...,
			)...,
		),
	}

	// `push/untaint/...` all TF commands that are not shortcuts
	deprecatedDefaultCommands := newDeprecatedDefaultCommands(l, opts)

	return append(deprecatedCommands.CLICommands(opts), deprecatedDefaultCommands...)
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

func newDeprecatedCLIRedesignTFCommands(args cli.Args) DeprecatedCommands {
	var cmds = make(DeprecatedCommands, len(tf.CommandNames))

	for i, tfCommandName := range tf.CommandNames {
		cmds[i] = &DeprecatedCommand{
			commandName:     tfCommandName,
			replaceWithArgs: append(cli.Args{tfCommandName}, args...),
			controlName:     controls.CLIRedesign,
			controlCategory: controls.CLIRedesignCommandsCategoryName,
		}
	}

	return cmds
}

func newDeprecatedDefaultCommands(l log.Logger, opts *options.TerragruntOptions) cli.Commands {
	var (
		runCmd       = run.NewCommand(l, opts)
		cmds         = make(cli.Commands, 0, len(runCmd.Subcommands))
		strictGroups = opts.StrictControls.FilterByNames(controls.DeprecatedCommands, controls.DefaultCommands)
	)

	for _, runSubCmd := range runCmd.Subcommands {
		if isShortcutCmd := slices.Contains(shortcutCommandNames, runSubCmd.Name); isShortcutCmd {
			continue
		}

		newCommand := "terragrunt " + run.CommandName + " -- " + runSubCmd.Name
		control := controls.NewDeprecatedReplacedCommand(runSubCmd.Name, newCommand)
		strictGroups.AddSubcontrolsToCategory(controls.DefaultCommandsCategoryName, control)
		ctl := control

		cmd := &cli.Command{
			Name:       runSubCmd.Name,
			Usage:      runSubCmd.Usage,
			Flags:      runCmd.Flags,
			CustomHelp: runSubCmd.CustomHelp,
			// Removal of default command forwarding: immediately error with guidance to use `run --`.
			Action: func(ctx *cli.Context) error {
				return cli.NewExitError(ctl.Error, cli.ExitCodeGeneralError)
			},
			Hidden:                       true,
			DisabledErrorOnUndefinedFlag: true,
		}

		cmds = append(cmds, cmd)
	}

	return cmds
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
