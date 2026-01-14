package migrate

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	CommandName = "migrate"

	ForceBackendMigrateFlagName = "force"

	usageText = "terragrunt backend migrate [options] <src-unit> <dst-unit>"
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
			Name:        ForceBackendMigrateFlagName,
			EnvVars:     tgPrefix.EnvVars(ForceBackendMigrateFlagName),
			Usage:       "Force the backend to be migrated, even if the bucket is not versioned.",
			Destination: &opts.ForceBackendMigrate,
		}),
	)
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
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

			return Run(ctx, l, srcPath, dstPath, opts.OptionsFromContext(ctx))
		},
	}

	return cmd
}
