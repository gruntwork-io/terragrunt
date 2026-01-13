package delete

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "delete"

	BucketFlagName             = "bucket"
	ForceBackendDeleteFlagName = "force"
)

func NewFlags(l log.Logger, opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	sharedFlags := clihelper.Flags{
		shared.NewConfigFlag(opts, prefix, CommandName),
		shared.NewDownloadDirFlag(opts, prefix, CommandName),
	}
	sharedFlags = append(sharedFlags, shared.NewBackendFlags(opts, prefix)...)
	sharedFlags = append(sharedFlags, shared.NewFeatureFlags(opts, prefix)...)

	return append(sharedFlags,
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        BucketFlagName,
			EnvVars:     tgPrefix.EnvVars(BucketFlagName),
			Usage:       "Delete the entire bucket.",
			Hidden:      true,
			Destination: &opts.DeleteBucket,
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        ForceBackendDeleteFlagName,
			EnvVars:     tgPrefix.EnvVars(ForceBackendDeleteFlagName),
			Usage:       "Force the backend to be deleted, even if the bucket is not versioned.",
			Destination: &opts.ForceBackendDelete,
		}),
	)
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	cmdFlags := NewFlags(l, opts, nil)
	cmdFlags = append(cmdFlags, shared.NewAllFlag(opts, nil), shared.NewFailFastFlag(opts))

	cmd := &clihelper.Command{
		Name:  CommandName,
		Usage: "Delete OpenTofu/Terraform state.",
		Flags: cmdFlags,
		Action: func(ctx context.Context, _ *clihelper.Context) error {
			return Run(ctx, l, opts.OptionsFromContext(ctx))
		},
	}

	return cmd
}
