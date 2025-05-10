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

	BucketFlagName             = "bucket"
	ForceBackendDeleteFlagName = "force"
)

func NewFlags(opts *options.TerragruntOptions, cmdPrefix flags.Name) cli.Flags {
	flags := cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        BucketFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(BucketFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(BucketFlagName),
			Usage:       "Delete the entire bucket.",
			Hidden:      true,
			Destination: &opts.DeleteBucket,
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        ForceBackendDeleteFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(ForceBackendDeleteFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(ForceBackendDeleteFlagName),
			Usage:       "Force the backend to be deleted, even if the bucket is not versioned.",
			Destination: &opts.ForceBackendDelete,
		}),
	}

	return append(flags, run.NewFlags(opts).Filter(run.ConfigFlagName, run.DownloadDirFlagName)...)
}

func NewCommand(opts *options.TerragruntOptions, cmdPrefix flags.Name) *cli.Command {
	cmdPrefix = cmdPrefix.Append(CommandName)

	cmd := &cli.Command{
		Name:  CommandName,
		Usage: "Delete OpenTofu/Terraform state.",
		Flags: NewFlags(opts, cmdPrefix),
		Action: func(ctx *cli.Context) error {
			return Run(ctx, opts.OptionsFromContext(ctx))
		},
	}

	cmd = runall.WrapCommand(opts, cmd, run.Run)

	return cmd
}
