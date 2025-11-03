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

// setupDownloadDir sets the unit's download directory.
// If the stack uses the default dir, compute a per-unit dir; otherwise use the stack's setting.
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

// determineTerragruntConfigFilename returns the config filename to use.
// If a file path is explicitly set, it uses its basename; otherwise, "terragrunt.hcl".
func (r *UnitResolver) determineTerragruntConfigFilename() string {
	fname := config.DefaultTerragruntConfigPath
	if r.Stack.TerragruntOptions.TerragruntConfigPath != "" && !util.IsDir(r.Stack.TerragruntOptions.TerragruntConfigPath) {
		fname = filepath.Base(r.Stack.TerragruntOptions.TerragruntConfigPath)
	}

	return fname
}
