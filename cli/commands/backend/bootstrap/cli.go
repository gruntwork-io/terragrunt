package bootstrap

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const CommandName = "bootstrap"

func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	return run.NewFlags(opts, nil).Filter(run.ConfigFlagName, run.DownloadDirFlagName)
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmd := &cli.Command{
		Name:  CommandName,
		Usage: "Bootstrap OpenTofu/Terraform backend infrastructure.",
		Flags: NewFlags(opts, nil),
		Action: func(ctx *cli.Context) error {
			return Run(ctx, opts.OptionsFromContext(ctx))
		},
	}

	cmd = runall.WrapCommand(opts, cmd, run.Run)

	return cmd
}
