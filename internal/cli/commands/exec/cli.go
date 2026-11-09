// Package exec provides the ability to execute a command using Terragrunt,
// via the `terragrunt exec -- command_name` command.
package exec

import (
	"context"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// CommandName is the name of the `terragrunt exec` command.
	CommandName = "exec"

	// InDownloadDirFlagName is the name of the flag that runs the command in
	// the unit's download directory after init.
	InDownloadDirFlagName = "in-download-dir"
)

// NewFlags returns the flags of `terragrunt exec`, bound to opts and cmdOpts.
func NewFlags(
	l log.Logger,
	opts *options.TerragruntOptions,
	cmdOpts *Options,
	prefix flags.Prefix,
) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	sharedFlags := append(
		clihelper.Flags{
			shared.NewConfigFlag(opts, prefix),
			shared.NewNoStackGenerateFlag(opts, prefix),
			shared.NewDownloadDirFlag(opts, prefix),
			shared.NewTFPathFlag(opts),
			shared.NewAuthProviderCmdFlag(opts, prefix),
			shared.NewInputsDebugFlag(opts, prefix),
			shared.NewNoAutoInitFlag(opts, prefix),
			shared.NewSourceFlag(opts, prefix),
			shared.NewSourceMapFlag(opts, prefix),
		},
		shared.NewIAMAssumeRoleFlags(opts, prefix)...,
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

// NewCommand returns the `terragrunt exec` command. It runs the arguments
// after `--` as the command, or every argument when `--` is absent, and prints
// help when there are none.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv, s *rewrite.Store) *clihelper.Command {
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

			return Run(ctx, l, v, s, opts, cmdOpts, cmdArgs)
		},
	}
}
