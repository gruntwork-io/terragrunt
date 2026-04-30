// Package exec provides the ability to execute a command using Terragrunt,
// via the `terragrunt exec -- command_name` command.
package exec

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	CommandName = "exec"

	InDownloadDirFlagName = "in-download-dir"
)

func NewFlags(l log.Logger, opts *options.TerragruntOptions, cmdOpts *Options, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	sharedFlags := append(
		clihelper.Flags{
			shared.NewConfigFlag(opts, prefix, CommandName),
			shared.NewDownloadDirFlag(opts, prefix, CommandName),
			shared.NewTFPathFlag(opts),
			shared.NewAuthProviderCmdFlag(opts, prefix, CommandName),
			shared.NewInputsDebugFlag(opts, prefix, CommandName),
		},
		shared.NewIAMAssumeRoleFlags(opts, prefix, CommandName)...,
	)

	return append(sharedFlags,
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        InDownloadDirFlagName,
			EnvVars:     tgPrefix.EnvVars(InDownloadDirFlagName),
			Destination: &cmdOpts.InDownloadDir,
			Usage:       "Run the provided command in the download directory.",
		}),
	)
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	cmdOpts := NewOptions()

	return &clihelper.Command{
		Name:        CommandName,
		Usage:       "Execute an arbitrary command.",
		UsageText:   "terragrunt exec [options] -- <command>",
		Description: "Execute a command using Terragrunt.",
		Examples: []string{
			"# Utilize the AWS CLI.\nterragrunt exec -- aws s3 ls",
			"# Inspect `main.tf` file of module for Unit\nterragrunt exec --in-download-dir -- cat main.tf",
		},
		Flags: NewFlags(l, opts, cmdOpts, nil),
		Action: func(ctx context.Context, cliCtx *clihelper.Context) error {
			tgArgs, cmdArgs := cliCtx.Args().Split(clihelper.BuiltinCmdSep)

			// Use unspecified arguments from the terragrunt command if the user
			// specified the target command without `--`, e.g. `terragrunt exec ls`.
			if len(cmdArgs) == 0 {
				cmdArgs = tgArgs
			}

			if len(cmdArgs) == 0 {
				return clihelper.ShowCommandHelp(ctx, cliCtx)
			}

			return Run(ctx, l, opts, cmdOpts, cmdArgs)
		},
	}
}
