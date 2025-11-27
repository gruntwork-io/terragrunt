// Package run contains the CLI command definition for interacting with OpenTofu/Terraform.
package run

import (
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
)

const (
	CommandName = "run"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	cmd := &cli.Command{
		Name:        CommandName,
		Usage:       "Run an OpenTofu/Terraform command.",
		UsageText:   "terragrunt run [options] -- <tofu/terraform command>",
		Description: "Run a command, passing arguments to an orchestrated tofu/terraform binary.\n\nThis is the explicit, and most flexible form of running an IaC command with Terragrunt. Shortcuts can be found in \"terragrunt --help\" for common use-cases.",
		Examples: []string{
			"# Run a plan\nterragrunt run -- plan\n# Shortcut:\n# terragrunt plan",
			"# Run output with -json flag\nterragrunt run -- output -json\n# Shortcut:\n# terragrunt output -json",
			// TODO: Add this example back when we support `run --all` again.
			//
			// "# Run a plan against a Stack of configurations in the current directory\nterragrunt run --all -- plan",
		},
		Flags:       NewFlags(l, opts, nil),
		Subcommands: NewSubcommands(l, opts),
		Action: func(ctx *cli.Context) error {
			if len(ctx.Args()) == 0 {
				return cli.ShowCommandHelp(ctx)
			}

			return Action(l, opts)(ctx)
		},
	}

	cmd = runall.WrapCommand(l, opts, cmd, run.Run, false)
	cmd = graph.WrapCommand(l, opts, cmd, run.Run, false)

	return cmd
}

func NewSubcommands(l log.Logger, opts *options.TerragruntOptions) cli.Commands {
	var subcommands = make(cli.Commands, len(tf.CommandNames))

	for i, name := range tf.CommandNames {
		usage, visible := tf.CommandUsages[name]

		subcommand := &cli.Command{
			Name:       name,
			Usage:      usage,
			Hidden:     !visible,
			CustomHelp: ShowTFHelp(l, opts),
			Action: func(ctx *cli.Context) error {
				return Action(l, opts)(ctx)
			},
		}
		subcommands[i] = subcommand
	}

	return subcommands
}

func Action(l log.Logger, opts *options.TerragruntOptions) cli.ActionFunc {
	return func(ctx *cli.Context) error {
		if opts.TerraformCommand == tf.CommandNameDestroy {
			opts.CheckDependentModules = !opts.NoDestroyDependenciesCheck
		}

		if err := validateCommand(opts); err != nil {
			return err
		}

		r := report.NewReport().WithWorkingDir(opts.WorkingDir)

		return run.Run(ctx.Context, l, opts.OptionsFromContext(ctx), r)
	}
}

func validateCommand(opts *options.TerragruntOptions) error {
	if opts.DisableCommandValidation || collections.ListContainsElement(tf.CommandNames, opts.TerraformCommand) {
		return nil
	}

	if isTerraformPath(opts) {
		return run.WrongTerraformCommand(opts.TerraformCommand)
	}

	return run.WrongTofuCommand(opts.TerraformCommand)
}

func isTerraformPath(opts *options.TerragruntOptions) bool {
	return strings.HasSuffix(opts.TFPath, options.TerraformDefaultPath)
}
