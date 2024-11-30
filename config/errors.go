package config

import (
	"fmt"
	"strings"

	"github.com/zclconf/go-cty/cty"
)

// ErrorsConfig represents the top-level errors configuration
type ErrorsConfig struct {
	Retry  []*RetryBlock  `cty:"retry"  hcl:"retry,block"`
	Ignore []*IgnoreBlock `cty:"ignore"  hcl:"ignore,block"`
}

// RetryBlock represents a labeled retry block
type RetryBlock struct {
	Label            string   `cty:"name" hcl:"name,label"`
	RetryableErrors  []string `cty:"retryable_errors" hcl:"retryable_errors"`
	MaxAttempts      int      `cty:"max_attempts" hcl:"max_attempts"`
	SleepIntervalSec int      `cty:"sleep_interval_sec" hcl:"sleep_interval_sec"`
}

// IgnoreBlock represents a labeled ignore block
type IgnoreBlock struct {
	Label           string               `cty:"name" hcl:"name,label"`
	IgnorableErrors []string             `cty:"ignorable_errors" hcl:"ignorable_errors"`
	Message         string               `cty:"message" hcl:"message,optional"`
	Signals         map[string]cty.Value `cty:"signals" hcl:"signals,optional"`
}

// Clone creates a deep copy of ErrorsConfig
func (c *ErrorsConfig) Clone() *ErrorsConfig {
	if c == nil {
		return nil
	}

	clone := &ErrorsConfig{
		Retry:  make([]*RetryBlock, len(c.Retry)),
		Ignore: make([]*IgnoreBlock, len(c.Ignore)),
	}

	// Clone Retry blocks
	for i, retry := range c.Retry {
		clone.Retry[i] = retry.Clone()
	}

	// Clone Ignore blocks
	for i, ignore := range c.Ignore {
		clone.Ignore[i] = ignore.Clone()
	}

	return clone
}

// Merge combines the current ErrorsConfig with another one, with the other config taking precedence
func (c *ErrorsConfig) Merge(other *ErrorsConfig) {
	if other == nil {
		return
	}
	if c == nil {
		*c = *other
		return
	}

	retryMap := make(map[string]*RetryBlock)
	for _, block := range c.Retry {
		retryMap[block.Label] = block
	}

	ignoreMap := make(map[string]*IgnoreBlock)
	for _, block := range c.Ignore {
		ignoreMap[block.Label] = block
	}

	// Merge retry blocks
	for _, otherBlock := range other.Retry {
		if existing, exists := retryMap[otherBlock.Label]; exists {
			existing.RetryableErrors = mergeStringSlices(existing.RetryableErrors, otherBlock.RetryableErrors)
			if otherBlock.MaxAttempts > 0 {
				existing.MaxAttempts = otherBlock.MaxAttempts
			}
			if otherBlock.SleepIntervalSec > 0 {
				existing.SleepIntervalSec = otherBlock.SleepIntervalSec
			}
		} else {
			// Add new block
			retryMap[otherBlock.Label] = otherBlock
		}
	}

	// Merge ignore blocks
	for _, otherBlock := range other.Ignore {
		if existing, exists := ignoreMap[otherBlock.Label]; exists {
			existing.IgnorableErrors = mergeStringSlices(existing.IgnorableErrors, otherBlock.IgnorableErrors)
			if otherBlock.Message != "" {
				existing.Message = otherBlock.Message
			}
			if otherBlock.Signals != nil {
				if existing.Signals == nil {
					existing.Signals = make(map[string]cty.Value)
				}
				for k, v := range otherBlock.Signals {
					existing.Signals[k] = v
				}
			}
		} else {
			// Add new block
			ignoreMap[otherBlock.Label] = otherBlock
		}
	}

	// Convert maps back to slices
	c.Retry = make([]*RetryBlock, 0, len(retryMap))
	for _, block := range retryMap {
		c.Retry = append(c.Retry, block)
	}

	c.Ignore = make([]*IgnoreBlock, 0, len(ignoreMap))
	for _, block := range ignoreMap {
		c.Ignore = append(c.Ignore, block)
	}
}

// mergeStringSlices combines two string slices removing duplicates
func mergeStringSlices(a, b []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(a)+len(b))

	// Add all strings from both slices, skipping duplicates
	for _, s := range append(a, b...) {
		if _, exists := seen[s]; !exists {
			seen[s] = struct{}{}
			result = append(result, s)
		}
	}
	return result
}

// Clone creates a deep copy of RetryBlock
func (r *RetryBlock) Clone() *RetryBlock {
	if r == nil {
		return nil
	}

	clone := &RetryBlock{
		Label:            r.Label,
		MaxAttempts:      r.MaxAttempts,
		SleepIntervalSec: r.SleepIntervalSec,
	}

	// Deep copy RetryableErrors slice
	if r.RetryableErrors != nil {
		clone.RetryableErrors = make([]string, len(r.RetryableErrors))
		copy(clone.RetryableErrors, r.RetryableErrors)
	}

	return clone
}

// Clone creates a deep copy of IgnoreBlock
func (i *IgnoreBlock) Clone() *IgnoreBlock {
	if i == nil {
		return nil
	}

	clone := &IgnoreBlock{
		Label:   i.Label,
		Message: i.Message,
	}

	// Deep copy IgnorableErrors slice
	if i.IgnorableErrors != nil {
		clone.IgnorableErrors = make([]string, len(i.IgnorableErrors))
		copy(clone.IgnorableErrors, i.IgnorableErrors)
	}

	// Deep copy Signals map
	if i.Signals != nil {
		clone.Signals = make(map[string]cty.Value, len(i.Signals))
		for k, v := range i.Signals {
			clone.Signals[k] = v
		}
	}

	return clone
}

// Custom error types

type InvalidArgError string

func (e InvalidArgError) Error() string {
	return string(e)
}

type IncludedConfigMissingPathError string

func (err IncludedConfigMissingPathError) Error() string {
	return fmt.Sprintf("The include configuration in %s must specify a 'path' parameter", string(err))
}

type TooManyLevelsOfInheritanceError struct {
	ConfigPath             string
	FirstLevelIncludePath  string
	SecondLevelIncludePath string
}

func (err TooManyLevelsOfInheritanceError) Error() string {
	return fmt.Sprintf("%s includes %s, which itself includes %s. Only one level of includes is allowed.", err.ConfigPath, err.FirstLevelIncludePath, err.SecondLevelIncludePath)
}

type CouldNotResolveTerragruntConfigInFileError string

func (err CouldNotResolveTerragruntConfigInFileError) Error() string {
	return "Could not find Terragrunt configuration settings in " + string(err)
}

type InvalidMergeStrategyTypeError string

func (err InvalidMergeStrategyTypeError) Error() string {
	return fmt.Sprintf(
		"Include merge strategy %s is unknown. Valid strategies are: %s, %s, %s, %s",
		string(err),
		NoMerge,
		ShallowMerge,
		DeepMerge,
		DeepMergeMapOnly,
	)
}

type DependencyDirNotFoundError struct {
	Dir []string
}

func (err DependencyDirNotFoundError) Error() string {
	return fmt.Sprintf(
		"Found paths in the 'dependencies' block that do not exist: %v", err.Dir,
	)
}

type DuplicatedGenerateBlocksError struct {
	BlockName []string
}

func (err DuplicatedGenerateBlocksError) Error() string {
	return fmt.Sprintf(
		"Detected generate blocks with the same name: %v", err.BlockName,
	)
}

type TFVarFileNotFoundError struct {
	File  string
	Cause string
}

func (err TFVarFileNotFoundError) Error() string {
	return fmt.Sprintf("TFVarFileNotFound: Could not find a %s. Cause: %s.", err.File, err.Cause)
}

type WrongNumberOfParamsError struct {
	Func     string
	Expected string
	Actual   int
}

func (err WrongNumberOfParamsError) Error() string {
	return fmt.Sprintf("Expected %s params for function %s, but got %d", err.Expected, err.Func, err.Actual)
}

type InvalidParameterTypeError struct {
	Expected string
	Actual   string
}

func (err InvalidParameterTypeError) Error() string {
	return fmt.Sprintf("Expected param of type %s but got %s", err.Expected, err.Actual)
}

type ParentFileNotFoundError struct {
	Path  string
	File  string
	Cause string
}

func (err ParentFileNotFoundError) Error() string {
	return fmt.Sprintf("ParentFileNotFoundError: Could not find a %s in any of the parent folders of %s. Cause: %s.", err.File, err.Path, err.Cause)
}

type InvalidGetEnvParamsError struct {
	ActualNumParams int
	Example         string
}

func (err InvalidGetEnvParamsError) Error() string {
	return fmt.Sprintf("InvalidGetEnvParamsError: Expected one or two parameters (%s) for get_env but got %d.", err.Example, err.ActualNumParams)
}

type EnvVarNotFoundError struct {
	EnvVar string
}

func (err EnvVarNotFoundError) Error() string {
	return fmt.Sprintf("EnvVarNotFoundError: Required environment variable %s - not found", err.EnvVar)
}

type InvalidEnvParamNameError struct {
	EnvName string
}

func (err InvalidEnvParamNameError) Error() string {
	return fmt.Sprintf("InvalidEnvParamNameError: Invalid environment variable name - (%s) ", err.EnvName)
}

type EmptyStringNotAllowedError string

func (err EmptyStringNotAllowedError) Error() string {
	return "Empty string value is not allowed for " + string(err)
}

type TerragruntConfigNotFoundError struct {
	Path string
}

func (err TerragruntConfigNotFoundError) Error() string {
	return fmt.Sprintf("Terragrunt config %s not found", err.Path)
}

type InvalidSourceURLError struct {
	ModulePath       string
	ModuleSourceURL  string
	TerragruntSource string
}

func (err InvalidSourceURLError) Error() string {
	return fmt.Sprintf("The --terragrunt-source parameter is set to '%s', but the source URL in the module at '%s' is invalid: '%s'. Note that the module URL must have a double-slash to separate the repo URL from the path within the repo!", err.TerragruntSource, err.ModulePath, err.ModuleSourceURL)
}

type InvalidSourceURLWithMapError struct {
	ModulePath      string
	ModuleSourceURL string
}

func (err InvalidSourceURLWithMapError) Error() string {
	return fmt.Sprintf("The --terragrunt-source-map parameter was passed in, but the source URL in the module at '%s' is invalid: '%s'. Note that the module URL must have a double-slash to separate the repo URL from the path within the repo!", err.ModulePath, err.ModuleSourceURL)
}

type ParsingModulePathError struct {
	ModuleSourceURL string
}

func (err ParsingModulePathError) Error() string {
	return fmt.Sprintf("Unable to obtain the module path from the source URL '%s'. Ensure that the URL is in a supported format.", err.ModuleSourceURL)
}

type InvalidSopsFormatError struct {
	SourceFilePath string
}

func (err InvalidSopsFormatError) Error() string {
	return fmt.Sprintf("File %s is not a valid format or encoding. Terragrunt will only decrypt yaml or json files in UTF-8 encoding.", err.SourceFilePath)
}

type InvalidIncludeKeyError struct {
	name string
}

func (err InvalidIncludeKeyError) Error() string {
	return fmt.Sprintf("There is no include block in the current config with the label '%s'", err.name)
}

// Dependency Custom error types

type DependencyConfigNotFound struct {
	Path string
}

func (err DependencyConfigNotFound) Error() string {
	return err.Path + " does not exist"
}

type TerragruntOutputParsingError struct {
	Path string
	Err  error
}

func (err TerragruntOutputParsingError) Error() string {
	return fmt.Sprintf("Could not parse output from terragrunt config %s. Underlying error: %s", err.Path, err.Err)
}

type TerragruntOutputEncodingError struct {
	Path string
	Err  error
}

func (err TerragruntOutputEncodingError) Error() string {
	return fmt.Sprintf("Could not encode output from terragrunt config %s. Underlying error: %s", err.Path, err.Err)
}

type TerragruntOutputListEncodingError struct {
	Paths []string
	Err   error
}

func (err TerragruntOutputListEncodingError) Error() string {
	return fmt.Sprintf("Could not encode output from list of terragrunt configs %v. Underlying error: %s", err.Paths, err.Err)
}

type TerragruntOutputTargetNoOutputs struct {
	targetConfig  string
	currentConfig string
}

func (err TerragruntOutputTargetNoOutputs) Error() string {
	return fmt.Sprintf(
		"%s is a dependency of %s but detected no outputs. Either the target module has not been applied yet, or the module has no outputs. If this is expected, set the skip_outputs flag to true on the dependency block.",
		err.targetConfig,
		err.currentConfig,
	)
}

type DependencyCycleError []string

func (err DependencyCycleError) Error() string {
	return "Found a dependency cycle between modules: " + strings.Join([]string(err), " -> ")
}
