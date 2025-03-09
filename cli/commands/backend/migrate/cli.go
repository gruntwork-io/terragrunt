package migrate

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "migrate"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                 CommandName,
		Usage:                "Migrate OpenTofu/Terraform state from one location to another.",
		Flags:                run.NewFlags(opts, nil).Filter(run.ConfigFlagName, run.DownloadDirFlagName),
		ErrorOnUndefinedFlag: true,
		Action: func(ctx *cli.Context) error {
			return Run(ctx, opts)
		},
	}
}
