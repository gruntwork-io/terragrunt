package runcfg

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/hashicorp/go-getter"
)

// DefaultTerragruntConfigPath is the default name of the terragrunt configuration file.
const DefaultTerragruntConfigPath = "terragrunt.hcl"

// DefaultEngineType is the default engine type.
const DefaultEngineType = "rpc"

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

// EngineOptions fetches engine options from the RunConfig.
func (cfg *RunConfig) EngineOptions() (*options.EngineOptions, error) {
	if cfg.Engine == nil {
		return nil, nil
	}
	// in case of Meta is null, set empty meta
	meta := map[string]any{}

	if cfg.Engine.Meta != nil {
		parsedMeta, err := ctyhelper.ParseCtyValueToMap(*cfg.Engine.Meta)
		if err != nil {
			return nil, err
		}

		meta = parsedMeta
	}

	var version, engineType string
	if cfg.Engine.Version != nil {
		version = *cfg.Engine.Version
	}

	if cfg.Engine.Type != nil {
		engineType = *cfg.Engine.Type
	}
	// if type is null or empty, set to "rpc"
	if len(engineType) == 0 {
		engineType = DefaultEngineType
	}

	return &options.EngineOptions{
		Source:  cfg.Engine.Source,
		Version: version,
		Type:    engineType,
		Meta:    meta,
	}, nil
}

// GetIAMRoleOptions returns the IAM role options from the RunConfig.
func (cfg *RunConfig) GetIAMRoleOptions() options.IAMRoleOptions {
	return cfg.IAMRole
}

// ErrorsConfig fetches errors configuration from the RunConfig.
func (cfg *RunConfig) ErrorsConfig() (*options.ErrorsConfig, error) {
	if cfg.Errors == nil {
		return nil, nil
	}

	result := &options.ErrorsConfig{
		Retry:  make(map[string]*options.RetryConfig),
		Ignore: make(map[string]*options.IgnoreConfig),
	}

	for _, retryBlock := range cfg.Errors.Retry {
		if retryBlock == nil {
			continue
		}

		// Validate retry settings
		if retryBlock.MaxAttempts < 1 {
			return nil, errors.Errorf("cannot have less than 1 max retry in errors.retry %q, but you specified %d", retryBlock.Label, retryBlock.MaxAttempts)
		}

		if retryBlock.SleepIntervalSec < 0 {
			return nil, errors.Errorf("cannot sleep for less than 0 seconds in errors.retry %q, but you specified %d", retryBlock.Label, retryBlock.SleepIntervalSec)
		}

		compiledPatterns := make([]*options.ErrorsPattern, 0, len(retryBlock.RetryableErrors))

		for _, pattern := range retryBlock.RetryableErrors {
			value, err := errorsPattern(pattern)
			if err != nil {
				return nil, errors.Errorf("invalid retry pattern %q in block %q: %w",
					pattern, retryBlock.Label, err)
			}

			compiledPatterns = append(compiledPatterns, value)
		}

		result.Retry[retryBlock.Label] = &options.RetryConfig{
			Name:             retryBlock.Label,
			RetryableErrors:  compiledPatterns,
			MaxAttempts:      retryBlock.MaxAttempts,
			SleepIntervalSec: retryBlock.SleepIntervalSec,
		}
	}

	for _, ignoreBlock := range cfg.Errors.Ignore {
		if ignoreBlock == nil {
			continue
		}

		var signals map[string]any

		if ignoreBlock.Signals != nil {
			value := convertValuesMapToCtyVal(ignoreBlock.Signals)

			var err error

			signals, err = ctyhelper.ParseCtyValueToMap(value)
			if err != nil {
				return nil, err
			}
		}

		compiledPatterns := make([]*options.ErrorsPattern, 0, len(ignoreBlock.IgnorableErrors))

		for _, pattern := range ignoreBlock.IgnorableErrors {
			value, err := errorsPattern(pattern)
			if err != nil {
				return nil, errors.Errorf("invalid ignore pattern %q in block %q: %w",
					pattern, ignoreBlock.Label, err)
			}

			compiledPatterns = append(compiledPatterns, value)
		}

		result.Ignore[ignoreBlock.Label] = &options.IgnoreConfig{
			Name:            ignoreBlock.Label,
			IgnorableErrors: compiledPatterns,
			Message:         ignoreBlock.Message,
			Signals:         signals,
		}
	}

	return result, nil
}

// errorsPattern builds an ErrorsPattern from a string pattern.
func errorsPattern(pattern string) (*options.ErrorsPattern, error) {
	isNegative := false
	p := pattern

	if len(p) > 0 && p[0] == '!' {
		isNegative = true
		p = p[1:]
	}

	compiled, err := regexp.Compile(p)
	if err != nil {
		return nil, err
	}

	return &options.ErrorsPattern{
		Pattern:  compiled,
		Negative: isNegative,
	}, nil
}

// convertValuesMapToCtyVal takes a map of name - cty.Value pairs and converts to a single cty.Value object.
func convertValuesMapToCtyVal(valMap map[string]cty.Value) cty.Value {
	if len(valMap) == 0 {
		// Return an empty object instead of NilVal for empty maps.
		return cty.EmptyObjectVal
	}

	// Use cty.ObjectVal directly instead of gocty.ToCtyValue to preserve marks (like sensitive())
	return cty.ObjectVal(valMap)
}
