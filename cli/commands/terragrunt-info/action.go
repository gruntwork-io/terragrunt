package terragruntinfo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gruntwork-io/go-commons/errors"

	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
)

// Run emits limited terragrunt state on stdout and exits.
func Run(ctx context.Context, opts *options.TerragruntOptions) error {
	target := terraform.NewTargetWithErrorHandler(
		terraform.TargetPointDownloadSource,
		runTerragruntInfo,
		runErrorTerragruntInfo,
	)

	err := terraform.RunWithTarget(ctx, opts, target)
	if err != nil {
		return fmt.Errorf("encountered error while running terragrunt-info: %w", err)
	}

	return nil
}

// Group is a struct that represents the output of the terragrunt-info command.
type Group struct {
	ConfigPath       string `json:"ConfigPath"`
	DownloadDir      string `json:"DownloadDir"`
	IamRole          string `json:"IamRole"`
	TerraformBinary  string `json:"TerraformBinary"`
	TerraformCommand string `json:"TerraformCommand"`
	WorkingDir       string `json:"WorkingDir"`
}

func printTerragruntInfo(opts *options.TerragruntOptions) error {
	group := Group{
		ConfigPath:       opts.TerragruntConfigPath,
		DownloadDir:      opts.DownloadDir,
		IamRole:          opts.IAMRoleOptions.RoleARN,
		TerraformBinary:  opts.TerraformPath,
		TerraformCommand: opts.TerraformCommand,
		WorkingDir:       opts.WorkingDir,
	}

	b, err := json.MarshalIndent(group, "", "  ")
	if err != nil {
		opts.Logger.Errorf("JSON error marshalling terragrunt-info")

		return fmt.Errorf("error marshalling terragrunt-info: %w", errors.WithStackTrace(err))
	}

	if _, err := fmt.Fprintf(opts.Writer, "%s\n", b); err != nil {
		return fmt.Errorf("error writing terragrunt-info: %w", errors.WithStackTrace(err))
	}

	return nil
}

func runTerragruntInfo(_ context.Context, opts *options.TerragruntOptions, _ *config.TerragruntConfig) error {
	return printTerragruntInfo(opts)
}

func runErrorTerragruntInfo(opts *options.TerragruntOptions, _ *config.TerragruntConfig, err error) error {
	opts.Logger.Debugf("Fetching terragrunt-info: %v", err)

	if err := printTerragruntInfo(opts); err != nil {
		opts.Logger.Errorf("Error printing terragrunt-info: %v", err)
	}

	return err
}
