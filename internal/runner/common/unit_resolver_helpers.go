package common

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
)

// resolveUnitPath converts a Terragrunt configuration file path to its corresponding unit path.
// Returns the canonical path of the directory containing the config file.
func (r *UnitResolver) resolveUnitPath(terragruntConfigPath string) (string, error) {
	return util.CanonicalPath(filepath.Dir(terragruntConfigPath), ".")
}

// setupDownloadDir configures the download directory for a Terragrunt unit.
//
// The method determines the appropriate download directory based on:
//  1. If the stack's download dir is the default, compute a unit-specific download dir
//  2. Otherwise, use the stack's configured download dir
//
// This ensures each unit has its own isolated download directory when using default settings,
// preventing conflicts between units when downloading Terraform modules.
//
// Returns an error if the download directory setup fails.
func (r *UnitResolver) setupDownloadDir(terragruntConfigPath string, opts *options.TerragruntOptions, l log.Logger) error {
	_, defaultDownloadDir, err := options.DefaultWorkingAndDownloadDirs(r.Stack.TerragruntOptions.TerragruntConfigPath)
	if err != nil {
		return err
	}

	if r.Stack.TerragruntOptions.DownloadDir == defaultDownloadDir {
		_, downloadDir, err := options.DefaultWorkingAndDownloadDirs(terragruntConfigPath)
		if err != nil {
			return err
		}

		l.Debugf("Setting download directory for unit %s to %s", filepath.Dir(opts.TerragruntConfigPath), downloadDir)
		opts.DownloadDir = downloadDir
	}

	return nil
}

// determineTerragruntConfigFilename determines the appropriate Terragrunt config file name.
//
// Logic:
//   - If TerragruntConfigPath is set and points to a file (not a directory), use its basename
//   - Otherwise, use the default "terragrunt.hcl"
//
// This allows users to specify custom config file names (e.g., "terragrunt-prod.hcl") while
// defaulting to the standard "terragrunt.hcl" when not specified.
func (r *UnitResolver) determineTerragruntConfigFilename() string {
	fname := config.DefaultTerragruntConfigPath
	if r.Stack.TerragruntOptions.TerragruntConfigPath != "" && !util.IsDir(r.Stack.TerragruntOptions.TerragruntConfigPath) {
		fname = filepath.Base(r.Stack.TerragruntOptions.TerragruntConfigPath)
	}

	return fname
}
