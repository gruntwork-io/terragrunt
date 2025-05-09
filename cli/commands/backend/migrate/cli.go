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

	usageText = "terragrunt backend migrate [options] <src-unit> <dst-unit>"
)

func NewFlags(opts *options.TerragruntOptions, cmdPrefix flags.Name) cli.Flags {
	flags := cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        ForceBackendMigrateFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(ForceBackendMigrateFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(ForceBackendMigrateFlagName),
			Usage:       "Force the backend to be migrated, even if the bucket is not versioned.",
			Destination: &opts.ForceBackendMigrate,
		}),
	}

	return append(flags, run.NewFlags(opts).Filter(run.ConfigFlagName, run.DownloadDirFlagName)...)
}

func NewCommand(opts *options.TerragruntOptions, cmdPrefix flags.Name) *cli.Command {
	cmdPrefix = cmdPrefix.Append(CommandName)

	cmd := &cli.Command{
		Name:      CommandName,
		Usage:     "Migrate OpenTofu/Terraform state from one location to another.",
		UsageText: usageText,
		Flags:     NewFlags(opts, cmdPrefix),
		Action: func(ctx *cli.Context) error {
			srcPath := ctx.Args().First()
			if srcPath == "" {
				return errors.New(usageText)
			}

			dstPath := ctx.Args().Second()
			if dstPath == "" {
				return errors.New(usageText)
			}

			return Run(ctx, srcPath, dstPath, opts.OptionsFromContext(ctx))
		},
	}

	return cmd
}
