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

func NewListFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	return run.NewFlags(opts, prefix)
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
			target := run.NewTargetWithErrorHandler(run.TargetPointDownloadSource, handleTerragruntContextPrint, handleTerragruntContextPrintWithError)
			return run.RunWithTarget(ctx, opts, target)
		},
	}
}

func handleTerragruntContextPrint(_ context.Context, opts *options.TerragruntOptions, _ *config.TerragruntConfig) error {
	return printTerragruntContext(opts)
}

func handleTerragruntContextPrintWithError(opts *options.TerragruntOptions, _ *config.TerragruntConfig, err error) error {
	opts.Logger.Debugf("Fetching info with error: %v", err)

	if err := printTerragruntContext(opts); err != nil {
		opts.Logger.Errorf("Error printing info: %v", err)
	}

	return nil
}

func printTerragruntContext(opts *options.TerragruntOptions) error {
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
