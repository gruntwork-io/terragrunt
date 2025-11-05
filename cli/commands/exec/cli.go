// Package exec provides the ability to execute a command using Terragrunt,
// via the `terragrunt exec -- command_name` command.
package exec

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "exec"

	InDownloadDirFlagName = "in-download-dir"
	TFPathFlagName        = "tf-path"
)

func NewFlags(l log.Logger, opts *options.TerragruntOptions, cmdOpts *Options, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	sharedFlags := append(
		cli.Flags{
			shared.NewConfigFlag(opts, prefix, CommandName),
			shared.NewDownloadDirFlag(opts, prefix, CommandName),
			shared.NewTFPathFlag(opts),
			shared.NewAuthProviderCmdFlag(opts, prefix, CommandName),
			shared.NewInputsDebugFlag(opts, prefix, CommandName),
		},
		shared.NewIAMAssumeRoleFlags(opts, prefix, CommandName)...,
	)

	return append(sharedFlags,
		flags.NewFlag(&cli.BoolFlag{
			Name:        InDownloadDirFlagName,
			EnvVars:     tgPrefix.EnvVars(InDownloadDirFlagName),
			Destination: &cmdOpts.InDownloadDir,
			Usage:       "Run the provided command in the download directory.",
		}),
	)
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	cmdOpts := NewOptions()

	return &cli.Command{
		Name:        CommandName,
		Usage:       "Execute an arbitrary command.",
		UsageText:   "terragrunt exec [options] -- <command>",
		Description: "Execute a command using Terragrunt.",
		Examples: []string{
			"# Utilize the AWS CLI.\nterragrunt exec -- aws s3 ls",
			"# Inspect `main.tf` file of module for Unit\nterragrunt exec --in-download-dir -- cat main.tf",
		},
		Flags: NewFlags(l, opts, cmdOpts, nil),
		Action: func(ctx *cli.Context) error {
			tgArgs, cmdArgs := ctx.Args().Split(cli.BuiltinCmdSep)

			// Use unspecified arguments from the terragrunt command if the user
			// specified the target command without `--`, e.g. `terragrunt exec ls`.
			if len(cmdArgs) == 0 {
				cmdArgs = tgArgs
			}

			if len(cmdArgs) == 0 {
				return cli.ShowCommandHelp(ctx)
			}

			return Run(ctx, l, opts, cmdOpts, cmdArgs)
		},
	}
}
