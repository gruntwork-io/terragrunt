// Package print implements the 'terragrunt info print' command that outputs Terragrunt context
// information in a structured JSON format. This includes configuration paths, working directories,
// IAM roles, and other essential Terragrunt runtime information useful for debugging and
// automation purposes.
package print

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"

	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "print"
)

// InfoOutput represents the structured output of the info command
type InfoOutput struct {
	ConfigPath       string `json:"config_path"`
	DownloadDir      string `json:"download_dir"`
	IAMRole          string `json:"iam_role"`
	TerraformBinary  string `json:"terraform_binary"`
	TerraformCommand string `json:"terraform_command"`
	WorkingDir       string `json:"working_dir"`
}

func NewListFlags(_ *options.TerragruntOptions, _ flags.Prefix) cli.Flags {
	return cli.Flags{}
}

func NewCommand(opts *options.TerragruntOptions, prefix flags.Prefix) *cli.Command {
	prefix = prefix.Append(CommandName)

	return &cli.Command{
		Name:                 CommandName,
		Usage:                "Print out a short description of Terragrunt context.",
		UsageText:            "terragrunt info print",
		Flags:                NewListFlags(opts, prefix),
		ErrorOnUndefinedFlag: true,
		Action: func(ctx *cli.Context) error {
			target := run.NewTargetWithErrorHandler(run.TargetPointDownloadSource, runInfo, runErrorInfo)

			return run.RunWithTarget(ctx, opts, target)
		},
	}
}

func printInfo(opts *options.TerragruntOptions) error {
	group := InfoOutput{
		ConfigPath:       opts.TerragruntConfigPath,
		DownloadDir:      opts.DownloadDir,
		IAMRole:          opts.IAMRoleOptions.RoleARN,
		TerraformBinary:  opts.TerraformPath,
		TerraformCommand: opts.TerraformCommand,
		WorkingDir:       opts.WorkingDir,
	}

	b, err := json.MarshalIndent(group, "", "  ")
	if err != nil {
		opts.Logger.Errorf("JSON error marshalling info")

		return errors.New(err)
	}

	if _, err := fmt.Fprintf(opts.Writer, "%s\n", b); err != nil {
		return errors.New(err)
	}

	return nil
}

func runInfo(_ context.Context, opts *options.TerragruntOptions, _ *config.TerragruntConfig) error {
	return printInfo(opts)
}

func runErrorInfo(opts *options.TerragruntOptions, _ *config.TerragruntConfig, err error) error {
	opts.Logger.Debugf("Fetching info: %v", err)

	if err := printInfo(opts); err != nil {
		opts.Logger.Errorf("Error printing info: %v", err)
	}

	return nil
}
