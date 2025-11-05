package bootstrap

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	runcmd "github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const CommandName = "bootstrap"

func NewFlags(l log.Logger, opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	base := runcmd.NewFlags(l, opts, prefix).Filter(runcmd.ConfigFlagName, runcmd.DownloadDirFlagName)
	// Also include backend-related and feature flags explicitly for backend commands
	base = append(base, runcmd.NewBackendFlags(l, opts, prefix)...)
	base = append(base, runcmd.NewFeatureFlags(l, opts, prefix)...)

	return base
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	cmd := &cli.Command{
		Name:  CommandName,
		Usage: "Bootstrap OpenTofu/Terraform backend infrastructure.",
		Flags: NewFlags(l, opts, nil),
		Action: func(ctx *cli.Context) error {
			return Run(ctx, l, opts.OptionsFromContext(ctx))
		},
	}

	cmd = runall.WrapCommand(l, opts, cmd, run.Run, true)

	return cmd
}
