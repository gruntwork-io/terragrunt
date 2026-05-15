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

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/prepare"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/pkg/options"
)

func Run(ctx context.Context, l log.Logger, v *venv.Venv, opts *options.TerragruntOptions) error {
	// If --all flag is set, use discovery to find all units and print info for each one
	if opts.RunAll {
		return runAll(ctx, l, v, opts)
	}

	return runPrint(ctx, l, v, opts)
}

func runPrint(ctx context.Context, l log.Logger, v *venv.Venv, opts *options.TerragruntOptions) error {
	prepared, err := prepare.PrepareConfig(ctx, l, v, opts)
	if err != nil {
		// Even on error, try to print what info we have
		l.Debugf("Fetching info with error: %v", err)

		if printErr := printTerragruntContext(l, v, opts); printErr != nil {
			l.Errorf("Error printing info: %v", printErr)
		}

		return nil
	}

	// Download source
	updatedOpts, err := prepare.PrepareSource(ctx, l, v, prepared.Opts, prepared.Cfg, report.NewReport())
	if err != nil {
		// Even on error, try to print what info we have
		l.Debugf("Fetching info with error: %v", err)

		if printErr := printTerragruntContext(l, v, opts); printErr != nil {
			l.Errorf("Error printing info: %v", printErr)
		}

		return nil
	}

	return printTerragruntContext(l, v, updatedOpts)
}

func runAll(ctx context.Context, l log.Logger, v *venv.Venv, opts *options.TerragruntOptions) error {
	d := discovery.NewDiscovery(opts.WorkingDir)

	components, err := d.Discover(ctx, l, v, opts)
	if err != nil {
		return err
	}

	units := components.Filter(component.UnitKind).Sort()

	var errs []error

	for _, unit := range units {
		unitOpts := opts.Clone()
		unitOpts.WorkingDir = unit.Path()

		configFilename := config.DefaultTerragruntConfigPath
		if len(opts.TerragruntConfigPath) > 0 {
			configFilename = filepath.Base(opts.TerragruntConfigPath)
		}

		unitOpts.TerragruntConfigPath = filepath.Join(unit.Path(), configFilename)

		// Preparation writes obtained credentials into the env, so each
		// unit gets its own clone to keep them from leaking to siblings.
		unitV := v.WithEnvCloned()
		if err := runPrint(ctx, l, &unitV, unitOpts); err != nil {
			if opts.FailFast {
				return err
			}

			l.Errorf("Print failed: %v", err)

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

func printTerragruntContext(l log.Logger, v *venv.Venv, opts *options.TerragruntOptions) error {
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
		return err
	}

	if _, err := fmt.Fprintf(v.Writers.Writer, "%s\n", b); err != nil {
		return err
	}

	return nil
}
