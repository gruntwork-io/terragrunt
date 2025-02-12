// Package run contains the logic for interacting with OpenTofu/Terraform.
package run

import (
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf"
)

const (
	CommandName = "run"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
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
		Flags:                NewFlags(opts, nil),
		ErrorOnUndefinedFlag: true,
		Subcommands:          NewSubcommands(opts),
		Action: func(ctx *cli.Context) error {
			if !opts.Experiments.Evaluate(experiment.CLIRedesign) {
				return cli.NewExitError(errors.Errorf("requires that the %[1]s experiment is enabled. e.g. --experiment %[1]s", experiment.CLIRedesign), cli.ExitCodeGeneralError)
			}

			if len(ctx.Args()) == 0 {
				return cli.ShowCommandHelp(ctx)
			}

			return Action(opts)(ctx)
		},
	}
}

func NewSubcommands(opts *options.TerragruntOptions) cli.Commands {
	var subcommands = make(cli.Commands, len(tf.CommandNames))

	for i, name := range tf.CommandNames {
		usage, visible := tf.CommandUsages[name]

		subcommands[i] = &cli.Command{
			Name:                 name,
			Usage:                usage,
			Hidden:               !visible,
			CustomHelp:           ShowTFHelp(opts),
			ErrorOnUndefinedFlag: true,
			Action: func(ctx *cli.Context) error {
				return Action(opts)(ctx)
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

func Action(opts *options.TerragruntOptions) cli.ActionFunc {
	return func(ctx *cli.Context) error {
		if opts.TerraformCommand == tf.CommandNameDestroy {
			opts.CheckDependentModules = !opts.NoDestroyDependenciesCheck
		}

		if err := validateCommand(opts); err != nil {
			return err
		}

		return Run(ctx.Context, opts.OptionsFromContext(ctx))
	}
}

func validateCommand(opts *options.TerragruntOptions) error {
	if opts.DisableCommandValidation || collections.ListContainsElement(tf.CommandNames, opts.TerraformCommand) {
		return nil
	}

	if strings.HasSuffix(opts.TerraformPath, options.TerraformDefaultPath) {
		return WrongTerraformCommand(opts.TerraformCommand)
	}

	return WrongTofuCommand(opts.TerraformCommand)
}
