// Package run contains the logic for interacting with OpenTofu/Terraform.
package run

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/tf"
)

const (
	CommandName = "run"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
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
		Flags:       NewFlags(opts, nil),
		Subcommands: NewSubcommands(opts),
		Action: func(ctx *cli.Context) error {
			if len(ctx.Args()) == 0 {
				return cli.ShowCommandHelp(ctx)
			}

			return Action(opts)(ctx)
		},
	}

	cmd = runall.WrapCommand(opts, cmd, Run)
	cmd = graph.WrapCommand(opts, cmd, Run)
	cmd = wrapWithStackGenerate(opts, cmd)

	return cmd
}

func NewSubcommands(opts *options.TerragruntOptions) cli.Commands {
	var subcommands = make(cli.Commands, len(tf.CommandNames))

	for i, name := range tf.CommandNames {
		usage, visible := tf.CommandUsages[name]

		subcommand := &cli.Command{
			Name:       name,
			Usage:      usage,
			Hidden:     !visible,
			CustomHelp: ShowTFHelp(opts),
			Action: func(ctx *cli.Context) error {
				return Action(opts)(ctx)
			},
		}
		subcommands[i] = subcommand
	}

	return subcommands
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

	if isTerraformPath(opts) {
		return WrongTerraformCommand(opts.TerraformCommand)
	}

	return WrongTofuCommand(opts.TerraformCommand)
}

func isTerraformPath(opts *options.TerragruntOptions) bool {
	return strings.HasSuffix(opts.TerraformPath, options.TerraformDefaultPath)
}

// wrapWithStackGenerate wraps a CLI command to automatically generate stack configurations
// when running with --all or --graph flags, unless --no-stack-generate is specified.
// This allows users to transparently use stack features without manual stack generation.
func wrapWithStackGenerate(opts *options.TerragruntOptions, cmd *cli.Command) *cli.Command {
	// Wrap the original command action with our stack generation logic
	cmd = cmd.WrapAction(func(ctx *cli.Context, action cli.ActionFunc) error {
		// Set the default stack configuration path in the working directory
		stackConfigPath := filepath.Join(opts.WorkingDir, config.DefaultStackFile)
		opts.TerragruntStackConfigPath = stackConfigPath

		// Determine if we should generate the stack configuration
		// We generate if:
		// 1. User is running with --all or --graph flag AND
		// 2. User hasn't explicitly disabled stack generation with --no-stack-generate
		shouldGenerateStack := (opts.RunAll || opts.Graph) && !opts.NoStackGenerate

		if shouldGenerateStack {
			// Generate the stack configuration
			err := generateStackConfiguration(ctx, opts)
			if err != nil {
				return fmt.Errorf("failed to generate stack file: %w", err)
			}
		} else {
			// Log that we're skipping stack generation
			opts.Logger.Debugf("Skipping stack generation for %s", stackConfigPath)
		}

		// Execute the original command action
		return action(ctx)
	})

	return cmd
}

// generateStackConfiguration handles the actual stack generation process,
// including telemetry collection for monitoring and debugging.
func generateStackConfiguration(ctx *cli.Context, opts *options.TerragruntOptions) error {
	// Collect telemetry data about the stack generation
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, "stack_generate", map[string]any{
		"stack_config_path": opts.TerragruntStackConfigPath,
		"working_dir":       opts.WorkingDir,
	}, func(ctx context.Context) error {
		// Generate the actual stack configuration
		return config.GenerateStacks(ctx, opts)
	})
}
