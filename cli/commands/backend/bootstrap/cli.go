package bootstrap

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const CommandName = "bootstrap"

func NewFlags(l log.Logger, opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	sharedFlags := cli.Flags{
		shared.NewConfigFlag(opts, prefix, CommandName),
		shared.NewDownloadDirFlag(opts, prefix, CommandName),
	}
	sharedFlags = append(sharedFlags, shared.NewBackendFlags(opts, prefix)...)
	sharedFlags = append(sharedFlags, shared.NewFeatureFlags(opts, prefix)...)

	return sharedFlags
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	cmdFlags := NewFlags(l, opts, nil)
	cmdFlags = append(cmdFlags, shared.NewAllFlag(opts, nil))

	cmd := &cli.Command{
		Name:  CommandName,
		Usage: "Bootstrap OpenTofu/Terraform backend infrastructure.",
		Flags: cmdFlags,
		Action: func(ctx *cli.Context) error {
			tgOpts := opts.OptionsFromContext(ctx)
			tgOpts.SummaryDisable = true

			if tgOpts.RunAll {
				return runall.Run(ctx.Context, l, tgOpts)
			}

			return Run(ctx, l, tgOpts)
		},
	}

	return cmd
}
