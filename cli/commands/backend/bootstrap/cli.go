package bootstrap

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/cli"
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
	sharedFlags = append(sharedFlags, shared.NewAllFlag(opts, CommandName, prefix))

	return sharedFlags
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

	return cmd
}
