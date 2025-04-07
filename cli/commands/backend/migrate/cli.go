package migrate

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "migrate"

	ForceBackendMigrateFlagName = "force"
)

func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	flags := cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        ForceBackendMigrateFlagName,
			EnvVars:     tgPrefix.EnvVars(ForceBackendMigrateFlagName),
			Usage:       "Force the backend to be migrated, even if the bucket is not versioned.",
			Destination: &opts.ForceBackendMigrate,
		}),
	}

	return append(flags, run.NewFlags(opts, nil).Filter(run.ConfigFlagName, run.DownloadDirFlagName)...)
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmd := &cli.Command{
		Name:                 CommandName,
		Usage:                "Migrate OpenTofu/Terraform state from one location to another.",
		UsageText:            "terragrunt backend migrage [options] <src-path> <dst-path>",
		Flags:                NewFlags(opts, nil),
		ErrorOnUndefinedFlag: true,
		Action: func(ctx *cli.Context) error {
			srcPath := ctx.Args().First()
			if srcPath == "" {
				return errors.New("not specified src-path")
			}

			dstPath := ctx.Args().Second()
			if dstPath == "" {
				return errors.New("not specified dst-path")
			}

			return Run(ctx, srcPath, dstPath, opts.OptionsFromContext(ctx))
		},
	}

	return cmd
}
