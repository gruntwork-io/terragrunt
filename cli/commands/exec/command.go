// Package exec provides the ability to execute a command using Terragrunt,
// via the `terragrunt exec -- command_name` command.
package exec

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "exec"

	InDownloadDirFlagName = "in-download-dir"
)

func NewFlags(opts *options.TerragruntOptions, cmdOpts *Options) cli.Flags {
	return append(run.NewFlags(opts).Filter(
		run.AuthProviderCmdFlagName,
		run.ConfigFlagName,
		run.DownloadDirFlagName,
		run.DebugInputsFlagName,
		run.IAMAssumeRoleFlagName,
		run.IAMAssumeRoleDurationFlagName,
		run.IAMAssumeRoleSessionNameFlagName,
		run.IAMAssumeRoleWebIdentityTokenFlagName,
	),
		&cli.BoolFlag{
			Name:        InDownloadDirFlagName,
			EnvVars:     flags.EnvVars(InDownloadDirFlagName),
			Destination: &cmdOpts.InDownloadDir,
			Usage:       "Run the provided command in the download directory.",
		},
	)
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmdOpts := NewOptions()

	return &cli.Command{
		Name:      CommandName,
		Usage:     "Execute a command using Terragrunt.",
		UsageText: "terragrunt exec [options] -- <command>",
		Examples: []string{
			"# Utilize the AWS CLI.\nterragrunt exec -- aws s3 ls",
			"# Inspect `main.tf` file of module for Unit\nterragrunt exec -- cat main.tf",
		},
		Flags:                NewFlags(opts, cmdOpts).Sort(),
		ErrorOnUndefinedFlag: true,
		Action: func(ctx *cli.Context) error {
			_, cmdArgs := ctx.Args().Split(cli.BuiltinCmdSep)

			return Run(ctx, opts, cmdOpts, cmdArgs)
		},
	}
}
