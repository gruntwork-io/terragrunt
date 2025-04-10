// Package terragruntinfo provides information about Terragrunt configuration.
// Deprecated: This package is deprecated and will be removed in a future version.
// Use the newer package `print` instead.
package terragruntinfo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/errors"

	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
)

func Run(ctx context.Context, opts *options.TerragruntOptions) error {
	target := run.NewTargetWithErrorHandler(run.TargetPointDownloadSource, runTerragruntInfo, runErrorTerragruntInfo)

	return run.RunWithTarget(ctx, opts, target)
}

// TerragruntInfoGroup is the output emit as JSON by 'terragrunt-info':
type TerragruntInfoGroup struct {
	ConfigPath       string `json:"ConfigPath"`
	DownloadDir      string `json:"DownloadDir"`
	IamRole          string `json:"IamRole"`
	TerraformBinary  string `json:"TerraformBinary"`
	TerraformCommand string `json:"TerraformCommand"`
	WorkingDir       string `json:"WorkingDir"`
}

func printTerragruntInfo(opts *options.TerragruntOptions) error {
	group := TerragruntInfoGroup{
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

		return errors.New(err)
	}

	if _, err := fmt.Fprintf(opts.Writer, "%s\n", b); err != nil {
		return errors.New(err)
	}

	return nil
}

func runTerragruntInfo(ctx context.Context, opts *options.TerragruntOptions, cfg *config.TerragruntConfig) error {
	return printTerragruntInfo(opts)
}

func runErrorTerragruntInfo(opts *options.TerragruntOptions, cfg *config.TerragruntConfig, err error) error {
	opts.Logger.Debugf("Fetching terragrunt-info: %v", err)

	if err := printTerragruntInfo(opts); err != nil {
		opts.Logger.Errorf("Error printing terragrunt-info: %v", err)
	}

	return err
}
