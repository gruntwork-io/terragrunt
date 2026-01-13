// Package run contains the CLI command definition for interacting with OpenTofu/Terraform.
package run

import (
	"context"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/graph"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
)

const (
	CommandName = "run"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	cmdFlags := NewFlags(l, opts, nil)
	cmdFlags = append(cmdFlags, shared.NewAllFlag(opts, nil), shared.NewGraphFlag(opts, nil))

	cmd := &clihelper.Command{
		Name:        CommandName,
		Usage:       "Run an OpenTofu/Terraform command.",
		UsageText:   "terragrunt run [options] -- <tofu/terraform command>",
		Description: "Run a command, passing arguments to an orchestrated tofu/terraform binary.\n\nThis is the explicit, and most flexible form of running an IaC command with Terragrunt. Shortcuts can be found in \"terragrunt --help\" for common use-cases.",
		Examples: []string{
			"# Run a plan\nterragrunt run -- plan\n# Shortcut:\n# terragrunt plan",
			"# Run output with -json flag\nterragrunt run -- output -json\n# Shortcut:\n# terragrunt output -json",
		},
		Flags:       cmdFlags,
		Subcommands: NewSubcommands(l, opts),
		Action: func(ctx context.Context, cliCtx *clihelper.Context) error {
			tgOpts := opts.OptionsFromContext(ctx)

			if tgOpts.RunAll {
				return runall.Run(ctx, l, tgOpts)
			}

			if tgOpts.Graph {
				return graph.Run(ctx, l, tgOpts)
			}

			if len(cliCtx.Args()) == 0 {
				return clihelper.ShowCommandHelp(ctx, cliCtx)
			}

			return Action(l, opts)(ctx, cliCtx)
		},
	}

	return cmd
}

func NewSubcommands(l log.Logger, opts *options.TerragruntOptions) clihelper.Commands {
	var subcommands = make(clihelper.Commands, len(tf.CommandNames))

	for i, name := range tf.CommandNames {
		usage, visible := tf.CommandUsages[name]

		subcommand := &clihelper.Command{
			Name:       name,
			Usage:      usage,
			Hidden:     !visible,
			CustomHelp: ShowTFHelp(l, opts),
			Action: func(ctx context.Context, cliCtx *clihelper.Context) error {
				return Action(l, opts)(ctx, cliCtx)
			},
		}
		subcommands[i] = subcommand
	}

	return subcommands
}

func Action(l log.Logger, opts *options.TerragruntOptions) clihelper.ActionFunc {
	return func(ctx context.Context, _ *clihelper.Context) error {
		if opts.TerraformCommand == tf.CommandNameDestroy {
			opts.CheckDependentModules = opts.DestroyDependenciesCheck
		}

		r := report.NewReport().WithWorkingDir(opts.WorkingDir)

		tgOpts := opts.OptionsFromContext(ctx)

		if tgOpts.RunAll {
			return runall.Run(ctx, l, tgOpts)
		}

		if tgOpts.Graph {
			return graph.Run(ctx, l, tgOpts)
		}

		return run.Run(ctx, l, tgOpts, r)
	}
}

// isTerraformPath returns true if the TFPath ends with the default Terraform path.
// This is used by help.go to determine whether to show "Terraform" or "OpenTofu" in help text.
func isTerraformPath(opts *options.TerragruntOptions) bool {
	return strings.HasSuffix(opts.TFPath, options.TerraformDefaultPath)
}
