package migrate

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "migrate"

	ForceBackendMigrateFlagName = "force"

	usageText = "terragrunt backend migrate [options] <src-unit> <dst-unit>"
)

func NewFlags(l log.Logger, opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	sharedFlags := cli.Flags{
		shared.NewConfigFlag(opts, prefix, CommandName),
		shared.NewDownloadDirFlag(opts, prefix, CommandName),
	}
	sharedFlags = append(sharedFlags, shared.NewBackendFlags(opts, prefix)...)
	sharedFlags = append(sharedFlags, shared.NewFeatureFlags(opts, prefix)...)

	return append(sharedFlags,
		flags.NewFlag(&cli.BoolFlag{
			Name:        ForceBackendMigrateFlagName,
			EnvVars:     tgPrefix.EnvVars(ForceBackendMigrateFlagName),
			Usage:       "Force the backend to be migrated, even if the bucket is not versioned.",
			Destination: &opts.ForceBackendMigrate,
		}),
	)
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	cmd := &cli.Command{
		Name:      CommandName,
		Usage:     "Migrate OpenTofu/Terraform state from one location to another.",
		UsageText: usageText,
		Flags:     NewFlags(l, opts, nil),
		Action: func(ctx *cli.Context) error {
			srcPath := ctx.Args().First()
			if srcPath == "" {
				return errors.New(usageText)
			}

			dstPath := ctx.Args().Second()
			if dstPath == "" {
				return errors.New(usageText)
			}

			return Run(ctx, l, srcPath, dstPath, opts.OptionsFromContext(ctx))
		},
	}

	return cmd
}
