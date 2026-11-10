package migrate

import (
	"context"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// CommandName is the name of the `terragrunt backend migrate` command.
	CommandName = "migrate"

	// ForceBackendMigrateFlagName is the name of the flag that migrates state
	// even when the source bucket has versioning disabled.
	ForceBackendMigrateFlagName = "force"

	usageText = "terragrunt backend migrate [options] <src-unit> <dst-unit>"
)

// NewFlags returns the flags of `terragrunt backend migrate`, bound to opts.
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
			Name:        ForceBackendMigrateFlagName,
			EnvVars:     tgPrefix.EnvVars(ForceBackendMigrateFlagName),
			Usage:       "Force the backend to be migrated, even if the bucket is not versioned.",
			Destination: &opts.ForceBackendMigrate,
		}),
	)
}

// NewCommand returns the `terragrunt backend migrate` command, which takes the
// source and destination unit paths as its two arguments.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv, s *rewrite.Store) *clihelper.Command {
	cmd := &clihelper.Command{
		Name:      CommandName,
		Usage:     "Migrate OpenTofu/Terraform state from one location to another.",
		UsageText: usageText,
		Flags:     NewFlags(l, opts, nil),
		Action: func(ctx context.Context, cliCtx *clihelper.Context) error {
			srcPath := cliCtx.Args().First()
			if srcPath == "" {
				return errors.New(usageText)
			}

			dstPath := cliCtx.Args().Second()
			if dstPath == "" {
				return errors.New(usageText)
			}

			return Run(ctx, l, v, s, srcPath, dstPath, opts.OptionsFromContext(ctx))
		},
	}

	return cmd
}
