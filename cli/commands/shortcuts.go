package commands

import (
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf"

	"github.com/gruntwork-io/terragrunt/cli/commands/run"
)

var (
	shortcutCommandNames = []string{
		tf.CommandNameInit,
		tf.CommandNameValidate,
		tf.CommandNamePlan,
		tf.CommandNameApply,
		tf.CommandNameDestroy,
		tf.CommandNameForceUnlock,
		tf.CommandNameImport,
		tf.CommandNameOutput,
		tf.CommandNameRefresh,
		tf.CommandNameShow,
		tf.CommandNameState,
		tf.CommandNameTest,
	}
)

func NewShortcutsCommands(opts *options.TerragruntOptions) cli.Commands {
	// Note: Some functionality is gated behind the CLIRedesign experiment.
	// This experiment controls the deprecation warnings for non-shortcut commands.
	var (
		runCmd       = run.NewCommand(opts)
		cmds         = make(cli.Commands, len(runCmd.Subcommands))
		strictGroups = opts.StrictControls.FilterByNames(controls.DeprecatedCommands, controls.DefaultCommands)
	)

	for i, runSubCmd := range runCmd.Subcommands {
		var (
			isNotShortcutCmd = !slices.Contains(shortcutCommandNames, runSubCmd.Name)
			control          strict.Control
		)

		if isNotShortcutCmd {
			newCommand := "terragrunt run -- " + runSubCmd.Name
			control = controls.NewDeprecatedCommand(runSubCmd.Name, newCommand)
			strictGroups.AddSubcontrolsToCategory(controls.DefaultCommandsCategoryName, control)
		}

		cmds[i] = &cli.Command{
			Name:       runSubCmd.Name,
			Usage:      runSubCmd.Usage,
			Hidden:     isNotShortcutCmd,
			Flags:      runCmd.Flags,
			CustomHelp: runSubCmd.CustomHelp,
			Action: func(ctx *cli.Context) error {
				if isNotShortcutCmd && opts.Experiments.Evaluate(experiment.CLIRedesign) {
					if err := control.Evaluate(ctx); err != nil {
						return cli.NewExitError(err, cli.ExitCodeGeneralError)
					}
				}

				return runSubCmd.Action(ctx)
			},
		}
	}

	return cmds
}
