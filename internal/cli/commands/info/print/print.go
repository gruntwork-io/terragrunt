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

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/pkg/options"
)

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	// If --all flag is set, use discovery to find all units and print info for each one
	if opts.RunAll {
		return runAll(ctx, l, opts)
	}

	return runPrint(ctx, l, opts)
}

func runPrint(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	ctx, pctx := configbridge.NewParsingContext(ctx, l, opts)

	cfg, err := config.PartialParseConfigFile(
		ctx,
		pctx.WithDecodeList(
			config.TerragruntVersionConstraints,
			config.TerragruntFlags,
			config.TerraformSource,
			config.RemoteStateBlock,
		),
		l,
		opts.TerragruntConfigPath,
		nil,
	)
	if err != nil {
		l.Debugf("Fetching info with error: %v", err)
	}

	if cfg != nil {
		if cfg.TerraformBinary != "" {
			opts.TFPath = cfg.TerraformBinary
		}

		opts.IAMRoleOptions = iam.MergeRoleOptions(
			cfg.GetIAMRoleOptions(),
			opts.OriginalIAMRoleOptions,
		)

		_, defaultDownloadDir := util.DefaultWorkingAndDownloadDirs(opts.TerragruntConfigPath)
		if opts.DownloadDir == defaultDownloadDir && cfg.DownloadDir != "" {
			opts.DownloadDir = cfg.DownloadDir
		}
	}

	return printTerragruntContext(l, opts)
}

func runAll(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	d := discovery.NewDiscovery(opts.WorkingDir)

	components, err := d.Discover(ctx, l, opts)
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

		if err := runPrint(ctx, l, unitOpts); err != nil {
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

	if _, err := fmt.Fprintf(opts.Writers.Writer, "%s\n", b); err != nil {
		return errors.New(err)
	}

	return nil
}
