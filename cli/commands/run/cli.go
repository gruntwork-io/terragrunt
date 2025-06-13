// Package run contains the logic for interacting with OpenTofu/Terraform.
package run

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
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

	cmd = runall.WrapCommand(l, opts, cmd, Run, false)
	cmd = graph.WrapCommand(l, opts, cmd, Run, false)
	cmd = wrapWithStackGenerate(l, opts, cmd)

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

		return Run(ctx.Context, l, opts.OptionsFromContext(ctx), r)
	}
}

func validateCommand(opts *options.TerragruntOptions) error {
	if opts.DisableCommandValidation || collections.ListContainsElement(tf.CommandNames, opts.TerraformCommand) {
		return nil
	}

	if isTerraformPath(opts) {
		return WrongTerraformCommand(opts.TerraformCommand)
	}

	return WrongTofuCommand(opts.TerraformCommand)
}

func isTerraformPath(opts *options.TerragruntOptions) bool {
	return strings.HasSuffix(opts.TerraformPath, options.TerraformDefaultPath)
}

// wrapWithStackGenerate wraps a CLI command to handle automatic stack generation.
// It generates a stack configuration file when running terragrunt with --all or --graph flags,
// unless explicitly disabled with --no-stack-generate.
func wrapWithStackGenerate(l log.Logger, opts *options.TerragruntOptions, cmd *cli.Command) *cli.Command {
	// Wrap the command's action to inject stack generation logic
	cmd = cmd.WrapAction(func(ctx *cli.Context, action cli.ActionFunc) error {
		// Determine if stack generation should occur based on command flags
		// Stack generation is triggered by --all or --graph flags, unless --no-stack-generate is set
		shouldGenerateStack := (opts.RunAll || opts.Graph) && !opts.NoStackGenerate

		// Skip stack generation if not needed
		if !shouldGenerateStack {
			l.Debugf("Skipping stack generation in %s", opts.WorkingDir)
			return action(ctx)
		}

		// Set the stack config path to the default location in the working directory
		opts.TerragruntStackConfigPath = filepath.Join(opts.WorkingDir, config.DefaultStackFile)

		// Generate the stack configuration with telemetry tracking
		err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "stack_generate", map[string]any{
			"stack_config_path": opts.TerragruntStackConfigPath,
			"working_dir":       opts.WorkingDir,
		}, func(ctx context.Context) error {
			return config.GenerateStacks(ctx, l, opts)
		})

		// Handle any errors during stack generation
		if err != nil {
			return errors.Errorf("failed to generate stack file: %w", err)
		}

		// Execute the original command action after successful stack generation
		return action(ctx)
	})

	return cmd
}
