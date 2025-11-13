// Package print implements the 'terragrunt info print' command that outputs Terragrunt context
// information in a structured JSON format. This includes configuration paths, working directories,
// IAM roles, and other essential Terragrunt runtime information useful for debugging and
// automation purposes.
package print

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/options"
)

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	// If --all is set, discover components and iterate
	if opts.RunAll {
		return runOnDiscoveredComponents(ctx, l, opts)
	}

	// Otherwise, run on single component
	target := run.NewTargetWithErrorHandler(run.TargetPointDownloadSource, handleTerragruntContextPrint, handleTerragruntContextPrintWithError)

	return run.RunWithTarget(ctx, l, opts, report.NewReport(), target)
}

func runOnDiscoveredComponents(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	// Create discovery
	d, err := discovery.NewForDiscoveryCommand(discovery.DiscoveryCommandOptions{
		WorkingDir:    opts.WorkingDir,
		FilterQueries: opts.FilterQueries,
		Experiments:   opts.Experiments,
		Hidden:        true,
		Dependencies:  false,
		External:      false,
		Exclude:       true,
		Include:       true,
	})
	if err != nil {
		return errors.New(err)
	}

	components, err := d.Discover(ctx, l, opts)
	if err != nil {
		return errors.New(err)
	}

	// Run the print logic on each component
	var errs []error

	for _, c := range components {
		if _, ok := c.(*component.Stack); ok {
			continue // Skip stacks
		}

		componentOpts := opts.Clone()
		componentOpts.WorkingDir = c.Path()

		// Determine config path for this component
		configFilename := config.DefaultTerragruntConfigPath
		if len(opts.TerragruntConfigPath) > 0 {
			configFilename = filepath.Base(opts.TerragruntConfigPath)
		}

		componentOpts.TerragruntConfigPath = filepath.Join(c.Path(), configFilename)

		// Run the print logic for this component
		target := run.NewTargetWithErrorHandler(run.TargetPointDownloadSource, handleTerragruntContextPrint, handleTerragruntContextPrintWithError)
		if err := run.RunWithTarget(ctx, l, componentOpts, report.NewReport(), target); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// InfoOutput represents the structured output of the info command
type InfoOutput struct {
	ConfigPath       string `json:"config_path"`
	DownloadDir      string `json:"download_dir"`
	IAMRole          string `json:"iam_role"`
	TerraformBinary  string `json:"terraform_binary"`
	TerraformCommand string `json:"terraform_command"`
	WorkingDir       string `json:"working_dir"`
}

func handleTerragruntContextPrint(_ context.Context, l log.Logger, opts *options.TerragruntOptions, _ *config.TerragruntConfig) error {
	return printTerragruntContext(l, opts)
}

func handleTerragruntContextPrintWithError(l log.Logger, opts *options.TerragruntOptions, _ *config.TerragruntConfig, err error) error {
	l.Debugf("Fetching info with error: %v", err)

	if err := printTerragruntContext(l, opts); err != nil {
		l.Errorf("Error printing info: %v", err)
	}

	return nil
}

func printTerragruntContext(l log.Logger, opts *options.TerragruntOptions) error {
	group := InfoOutput{
		ConfigPath:       opts.TerragruntConfigPath,
		DownloadDir:      opts.DownloadDir,
		IAMRole:          opts.IAMRoleOptions.RoleARN,
		TerraformBinary:  opts.TFPath,
		TerraformCommand: opts.TerraformCommand,
		WorkingDir:       opts.WorkingDir,
	}

	b, err := json.MarshalIndent(group, "", "  ")
	if err != nil {
		l.Errorf("JSON error marshalling info")
		return errors.New(err)
	}

	if _, err := fmt.Fprintf(opts.Writer, "%s\n", b); err != nil {
		return errors.New(err)
	}

	return nil
}
