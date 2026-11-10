package delete

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// CommandName is the name of the `terragrunt backend delete` command.
	CommandName = "delete"

	// BucketFlagName is the name of the hidden flag that asks to delete the
	// whole bucket. The command rejects it as not yet supported.
	BucketFlagName = "bucket"
	// ForceBackendDeleteFlagName is the name of the flag that deletes state
	// even when the bucket has versioning disabled.
	ForceBackendDeleteFlagName = "force"
)

// NewFlags returns the flags of `terragrunt backend delete`, bound to opts.
func NewFlags(l log.Logger, opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	sharedFlags := clihelper.Flags{
		shared.NewConfigFlag(opts, prefix),
		shared.NewDownloadDirFlag(opts, prefix),
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

// NewCommand returns the `terragrunt backend delete` command.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv, s *rewrite.Store) *clihelper.Command {
	cmdFlags := NewFlags(l, opts, nil)
	cmdFlags = append(cmdFlags, shared.NewAllFlag(opts, nil), shared.NewFailFastFlag(opts))

	cmd := &clihelper.Command{
		Name:  CommandName,
		Usage: "Delete OpenTofu/Terraform state.",
		Flags: cmdFlags,
		Action: func(ctx context.Context, _ *clihelper.Context) error {
			return Run(ctx, l, v, s, opts.OptionsFromContext(ctx))
		},
	}

	return cmd
}
