package delete

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "delete"

	BucketFlagName = "bucket"
)

func NewFlags(cmdOpts *Options, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	flags := cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        BucketFlagName,
			EnvVars:     tgPrefix.EnvVars(BucketFlagName),
			Usage:       "Delete the entire bucket.",
			Destination: &cmdOpts.DeleteBucket,
		}),
	}

	return append(flags, run.NewFlags(cmdOpts.TerragruntOptions, nil).Filter(run.ConfigFlagName, run.DownloadDirFlagName)...)
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmdOpts := NewOptions(opts)

	cmd := &cli.Command{
		Name:                 CommandName,
		Usage:                "Delete OpenTofu/Terraform state.",
		Flags:                NewFlags(cmdOpts, nil),
		ErrorOnUndefinedFlag: true,
		Action: func(ctx *cli.Context) error {
			return Run(ctx, cmdOpts.OptionsFromContext(ctx))
		},
	}

	cmd = runall.WrapCommand(opts, cmd)

	return cmd
}
