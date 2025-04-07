package print

import (
	"encoding/json"
	"fmt"

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
		Action:               infoAction(opts),
	}
}

func infoAction(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
	return func(c *cli.Context) error {
		output := InfoOutput{
			ConfigPath:       opts.TerragruntConfigPath,
			DownloadDir:      opts.DownloadDir,
			IAMRole:          opts.IAMRoleOptions.RoleARN,
			TerraformBinary:  opts.TerraformPath,
			TerraformCommand: opts.TerraformCommand,
			WorkingDir:       opts.WorkingDir,
		}

		jsonBytes, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			opts.Logger.Error("failed to marshal info output to JSON")
			return fmt.Errorf("failed to marshal info output: %w", err)
		}

		if _, err := fmt.Fprintln(opts.Writer, string(jsonBytes)); err != nil {
			return fmt.Errorf("failed to write info output: %w", err)
		}

		return nil
	}
}
