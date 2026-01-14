package runcfg

import (
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/hashicorp/go-getter"
)

// DefaultTerragruntConfigPath is the default name of the terragrunt configuration file.
const DefaultTerragruntConfigPath = "terragrunt.hcl"

// TerraformCommandsNeedInput lists terraform commands that require input handling.
var TerraformCommandsNeedInput = []string{"apply", "destroy", "refresh", "import"}

// CopyLockFile copies the lock file from the source folder to the destination folder.
//
// Terraform 0.14 now generates a lock file when you run `terraform init`.
// If any such file exists, this function will copy the lock file to the destination folder.
func CopyLockFile(l log.Logger, opts *options.TerragruntOptions, sourceFolder, destinationFolder string) error {
	sourceLockFilePath := filepath.Join(sourceFolder, tf.TerraformLockFile)
	destinationLockFilePath := filepath.Join(destinationFolder, tf.TerraformLockFile)

	if util.FileExists(sourceLockFilePath) {
		l.Debugf("Copying lock file from %s to %s", sourceLockFilePath, destinationFolder)
		return util.CopyFile(sourceLockFilePath, destinationLockFilePath)
	}

	return nil
}

// GetTerraformSourceURL returns the source URL for OpenTofu/Terraform configuration.
//
// There are two ways a user can tell Terragrunt that it needs to download Terraform configurations from a specific
// URL: via a command-line option or via an entry in the Terragrunt configuration. If the user used one of these, this
// method returns the source URL or an empty string if there is no source url.
func GetTerraformSourceURL(opts *options.TerragruntOptions, cfg *RunConfig) (string, error) {
	switch {
	case opts.Source != "":
		return opts.Source, nil
	case cfg != nil && cfg.Terraform != nil && cfg.Terraform.Source != nil:
		return adjustSourceWithMap(opts.SourceMap, *cfg.Terraform.Source, opts.OriginalTerragruntConfigPath)
	default:
		return "", nil
	}
}

// adjustSourceWithMap implements the --terragrunt-source-map feature. This function will check if the URL portion of a
// terraform source matches any entry in the provided source map and if it does, replace it with the configured source
// in the map. Note that this only performs literal matches with the URL portion.
func adjustSourceWithMap(sourceMap map[string]string, source string, modulePath string) (string, error) {
	// Skip source map processing if no source map was provided
	if len(sourceMap) == 0 {
		return source, nil
	}

	// use go-getter to split the module source string into a valid URL and subdirectory (if // is present)
	moduleURL, moduleSubdir := getter.SourceDirSubdir(source)

	// Check if there is an entry to replace the URL portion of the source
	mappedURL, hasMappedURL := sourceMap[moduleURL]
	if !hasMappedURL {
		return source, nil
	}

	// Since there is a source mapping, replace the module URL portion with the entry
	moduleSubdir = filepath.Join(mappedURL, moduleSubdir)

	if strings.HasPrefix(moduleSubdir, filepath.VolumeName(moduleSubdir)) {
		return moduleSubdir, nil
	}

	// Check for relative path and if relative, assume it is relative to the terragrunt config path
	if !filepath.IsAbs(moduleSubdir) {
		moduleSubdir = filepath.Join(filepath.Dir(modulePath), moduleSubdir)
	}

	return moduleSubdir, nil
}

// ShouldCopyLockFile determines if the terraform lock file should be copied.
func ShouldCopyLockFile(cfg *TerraformConfig) bool {
	if cfg == nil {
		return true // Default to copying
	}

	if cfg.CopyTerraformLockFile != nil {
		return *cfg.CopyTerraformLockFile
	}

	return true // Default to copying
}
