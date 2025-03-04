// Package exec provides the ability to execute a command using Terragrunt,
// via the `terragrunt exec -- command_name` command.
package exec

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "exec"

	InDownloadDirFlagName = "in-download-dir"
)

func NewFlags(opts *options.TerragruntOptions, cmdOpts *Options, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return append(run.NewFlags(opts, prefix).Filter(
		run.AuthProviderCmdFlagName,
		run.ConfigFlagName,
		run.DownloadDirFlagName,
		run.InputsDebugFlagName,
		run.IAMAssumeRoleFlagName,
		run.IAMAssumeRoleDurationFlagName,
		run.IAMAssumeRoleSessionNameFlagName,
		run.IAMAssumeRoleWebIdentityTokenFlagName,
	),
		flags.NewFlag(&cli.BoolFlag{
			Name:        InDownloadDirFlagName,
			EnvVars:     tgPrefix.EnvVars(InDownloadDirFlagName),
			Destination: &cmdOpts.InDownloadDir,
			Usage:       "Run the provided command in the download directory.",
		}),
	)
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
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
		Flags:                NewFlags(opts, cmdOpts, nil).Sort(),
		ErrorOnUndefinedFlag: true,
		Before: func(ctx *cli.Context) error {
			if !opts.Experiments.Evaluate(experiment.CLIRedesign) {
				return cli.NewExitError(errors.Errorf("requires that the %[1]s experiment is enabled. e.g. --experiment %[1]s", experiment.CLIRedesign), cli.ExitCodeGeneralError)
			}

			return nil
		},
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

			return Run(ctx, opts, cmdOpts, cmdArgs)
		},
	}
}
