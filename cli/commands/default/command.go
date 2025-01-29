// Package defaultcmd represents the default CLI command.
package defaultcmd

import (
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf"

	runCmd "github.com/gruntwork-io/terragrunt/cli/commands/run"
)

const (
	CommandName     = ""
	CommandHelpName = "*"
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

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:        CommandName,
		HelpName:    CommandHelpName,
		Usage:       "Terragrunt forwards all other commands directly to OpenTofu/Terraform",
		Flags:       runCmd.NewFlags(opts, nil),
		Subcommands: NewSubcommands(opts),
		Action: func(ctx *cli.Context) error {
			return runCmd.Action(opts)(ctx)
		},
	}
}

func NewSubcommands(opts *options.TerragruntOptions) cli.Commands {
	var (
		subcommands  = make(cli.Commands, len(tf.CommandNames))
		strictGroups = opts.StrictControls.FilterByNames(controls.DeprecatedCommands, controls.CLIRedesign)
	)

	for i, name := range tf.CommandNames {
		var (
			isNotShortcutCmd = !slices.Contains(shortcutCommandNames, name)
			control          strict.Control
		)

		if isNotShortcutCmd {
			newCommand := "terragrunt run -- " + name
			control = controls.NewDeprecatedCommand(name, newCommand)
			strictGroups.AddSubcontrolsToCategory(controls.DefaultCommandsCategoryName, control)
		}

		subcommands[i] = &cli.Command{
			Name:       name,
			Hidden:     isNotShortcutCmd,
			CustomHelp: ShowTFHelp(opts),
			Action: func(ctx *cli.Context) error {
				if isNotShortcutCmd && opts.Experiments.Evaluate(experiment.CLIRedesign) {
					if err := control.Evaluate(ctx); err != nil {
						return cli.NewExitError(err, cli.ExitCodeGeneralError)
					}
				}

				return runCmd.Action(opts)(ctx)
			},
		}
	}

	return subcommands
}

// ShowTFHelp prints TF help for the given `ctx.Command` command.
func ShowTFHelp(opts *options.TerragruntOptions) cli.HelpFunc {
	return func(ctx *cli.Context) error {
		terraformHelpCmd := append([]string{tf.FlagNameHelpLong, ctx.Command.Name}, ctx.Args()...)

		return tf.RunCommand(ctx, opts, terraformHelpCmd...)
	}
}
