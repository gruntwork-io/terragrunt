package delete

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "delete"

	BucketFlagName             = "bucket"
	ForceBackendDeleteFlagName = "force"
)

func NewFlags(l log.Logger, opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	flags := cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        BucketFlagName,
			EnvVars:     tgPrefix.EnvVars(BucketFlagName),
			Usage:       "Delete the entire bucket.",
			Hidden:      true,
			Destination: &opts.DeleteBucket,
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        ForceBackendDeleteFlagName,
			EnvVars:     tgPrefix.EnvVars(ForceBackendDeleteFlagName),
			Usage:       "Force the backend to be deleted, even if the bucket is not versioned.",
			Destination: &opts.ForceBackendDelete,
		}),
	}

	return append(flags, run.NewFlags(l, opts, nil).Filter(run.ConfigFlagName, run.DownloadDirFlagName)...)
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	cmd := &cli.Command{
		Name:  CommandName,
		Usage: "Delete OpenTofu/Terraform state.",
		Flags: NewFlags(l, opts, nil),
		Action: func(ctx *cli.Context) error {
			return Run(ctx, l, opts.OptionsFromContext(ctx))
		},
	}

	cmd = runall.WrapCommand(l, opts, cmd, run.Run, true)

	return cmd
}
