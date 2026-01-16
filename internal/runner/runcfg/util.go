package runcfg

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
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
	case cfg != nil && cfg.Terraform.Source != "":
		return AdjustSourceWithMap(opts.SourceMap, cfg.Terraform.Source, opts.OriginalTerragruntConfigPath)
	default:
		return "", nil
	}
}

// AdjustSourceWithMap implements the --terragrunt-source-map feature. This function will check if the URL portion of a
// terraform source matches any entry in the provided source map and if it does, replace it with the configured source
// in the map. Note that this only performs literal matches with the URL portion.
//
// Example:
// Suppose terragrunt is called with:
//
//	--terragrunt-source-map git::ssh://git@github.com/gruntwork-io/i-dont-exist.git=/path/to/local-modules
//
// and the terraform source is:
//
//	git::ssh://git@github.com/gruntwork-io/i-dont-exist.git//fixtures/source-map/modules/app?ref=master
//
// This function will take that source and transform it to:
//
//	/path/to/local-modules//fixtures/source-map/modules/app
func AdjustSourceWithMap(sourceMap map[string]string, source string, modulePath string) (string, error) {
	// Skip logic if source map is not configured
	if len(sourceMap) == 0 {
		return source, nil
	}

	// use go-getter to split the module source string into a valid URL and subdirectory (if // is present)
	moduleURL, moduleSubdir := getter.SourceDirSubdir(source)

	// if both URL and subdir are missing, something went terribly wrong
	if moduleURL == "" && moduleSubdir == "" {
		return "", errors.New(InvalidSourceURLWithMapError{ModulePath: modulePath, ModuleSourceURL: source})
	}

	// If module URL is missing, return the source as is as it will not match anything in the map.
	if moduleURL == "" {
		return source, nil
	}

	// Before looking up in sourceMap, make sure to drop any query parameters.
	moduleURLParsed, err := url.Parse(moduleURL)
	if err != nil {
		return source, err
	}

	moduleURLParsed.RawQuery = ""
	moduleURLQuery := moduleURLParsed.String()

	// Check if there is an entry to replace the URL portion in the map. Return the source as is if there is no entry in
	// the map.
	sourcePath, hasKey := sourceMap[moduleURLQuery]
	if !hasKey {
		return source, nil
	}

	// Since there is a source mapping, replace the module URL portion with the entry in the map, and join with the
	// subdir.
	// If subdir is missing, check if we can obtain a valid module name from the URL portion.
	if moduleSubdir == "" {
		moduleSubdirFromURL, err := GetModulePathFromSourceURL(moduleURL)
		if err != nil {
			return moduleSubdirFromURL, err
		}

		moduleSubdir = moduleSubdirFromURL
	}

	return util.JoinTerraformModulePath(sourcePath, moduleSubdir), nil
}

// InvalidSourceURLWithMapError is an error type for invalid source URLs when using source map.
type InvalidSourceURLWithMapError struct {
	ModulePath      string
	ModuleSourceURL string
}

func (err InvalidSourceURLWithMapError) Error() string {
	return fmt.Sprintf("The --source-map parameter was passed in, but the source URL in the module at '%s' is invalid: '%s'. Note that the module URL must have a double-slash to separate the repo URL from the path within the repo!", err.ModulePath, err.ModuleSourceURL)
}

// ParsingModulePathError is an error type for when module path cannot be parsed from source URL.
type ParsingModulePathError struct {
	ModuleSourceURL string
}

func (err ParsingModulePathError) Error() string {
	return fmt.Sprintf("Unable to obtain the module path from the source URL '%s'. Ensure that the URL is in a supported format.", err.ModuleSourceURL)
}

// Regexp for module name extraction. It assumes that the query string has already been stripped off.
// Then we simply capture anything after the last slash, and before `.` or end of string.
var moduleNameRegexp = regexp.MustCompile(`(?:.+/)(.+?)(?:\.|$)`)

// GetModulePathFromSourceURL parses sourceUrl not containing '//', and attempt to obtain a module path.
// Example:
//
// sourceUrl = "git::ssh://git@ghe.ourcorp.com/OurOrg/module-name.git"
// will return "module-name".
func GetModulePathFromSourceURL(sourceURL string) (string, error) {
	// strip off the query string if present
	sourceURL = strings.Split(sourceURL, "?")[0]

	matches := moduleNameRegexp.FindStringSubmatch(sourceURL)

	// if regexp returns less/more than the full match + 1 capture group, then something went wrong with regex (invalid source string)
	const matchedPats = 2
	if len(matches) != matchedPats {
		return "", errors.New(ParsingModulePathError{ModuleSourceURL: sourceURL})
	}

	return matches[1], nil
}

// ShouldCopyLockFile determines if the terraform lock file should be copied.
func ShouldCopyLockFile(cfg *TerraformConfig) bool {
	if cfg == nil {
		return true // Default to copying
	}

	return !cfg.NoCopyTerraformLockFile
}

// EngineOptions fetches engine options from the RunConfig.
func (cfg *RunConfig) EngineOptions() (*options.EngineOptions, error) {
	// in case of Meta is null, set empty meta
	meta := map[string]any{}

	if cfg.Engine.Meta != nil {
		parsedMeta, err := ctyhelper.ParseCtyValueToMap(*cfg.Engine.Meta)
		if err != nil {
			return nil, err
		}

		meta = parsedMeta
	}

	version := cfg.Engine.Version
	engineType := cfg.Engine.Type
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

// Exclude action constants
const (
	AllActions              = "all"
	AllExcludeOutputActions = "all_except_output"
	TgOutput                = "output"
)

// IsActionListedInExclude checks if the action is listed in the exclude block actions.
// This is a shared utility function that provides a single source of truth for exclude action matching logic.
// It handles special action values:
//   - "all": matches any action
//   - "all_except_output": matches any action except "output"
//   - Case-insensitive matching for regular actions
func IsActionListedInExclude(actions []string, action string) bool {
	if len(actions) == 0 {
		return false
	}

	actionLower := strings.ToLower(action)

	for _, checkAction := range actions {
		if checkAction == AllActions {
			return true
		}

		if checkAction == AllExcludeOutputActions && actionLower != TgOutput {
			return true
		}

		if strings.ToLower(checkAction) == actionLower {
			return true
		}
	}

	return false
}

// ShouldPreventRunBasedOnExclude determines if execution should be prevented based on exclude configuration.
// This is a shared utility function that provides a single source of truth for exclude run prevention logic.
// Parameters:
//   - actions: list of actions in the exclude block
//   - noRun: pointer to no_run flag (nil means not set)
//   - ifCondition: the if condition value
//   - command: the command/action to check
func ShouldPreventRunBasedOnExclude(actions []string, noRun *bool, ifCondition bool, command string) bool {
	if !ifCondition {
		return false
	}

	if noRun != nil && *noRun {
		return IsActionListedInExclude(actions, command)
	}

	return slices.Contains(actions, command)
}
