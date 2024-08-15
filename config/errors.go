package config

import (
	"fmt"
	"strings"
)

// Custom error types

// InvalidArgError represents an error that occurs when a function is called with an invalid argument
// (e.g., the wrong number of arguments, or an argument of the wrong type)
// The string value of this error is the error message to display to the user.
type InvalidArgError string

func (e InvalidArgError) Error() string {
	return string(e)
}

// IncludedConfigMissingPathError represents an error that occurs when a Terragrunt configuration file includes another
// configuration file that does not specify a 'path' parameter.
type IncludedConfigMissingPathError string

func (err IncludedConfigMissingPathError) Error() string {
	return fmt.Sprintf("The include configuration in %s must specify a 'path' parameter", string(err))
}

// TooManyLevelsOfInheritanceError represents an error that occurs when a Terragrunt configuration file includes another
// configuration file that itself includes another configuration file. Terragrunt only supports one level of
// inheritance.
type TooManyLevelsOfInheritanceError struct {
	ConfigPath             string
	FirstLevelIncludePath  string
	SecondLevelIncludePath string
}

func (err TooManyLevelsOfInheritanceError) Error() string {
	return fmt.Sprintf(
		"%s includes %s, which itself includes %s. Only one level of includes is allowed.",
		err.ConfigPath,
		err.FirstLevelIncludePath,
		err.SecondLevelIncludePath,
	)
}

// CouldNotResolveTerragruntConfigInFileError represents an error that occurs when a Terragrunt configuration file does
// not contain any Terragrunt configuration settings.
type CouldNotResolveTerragruntConfigInFileError string

func (err CouldNotResolveTerragruntConfigInFileError) Error() string {
	return "Could not find Terragrunt configuration settings in " + string(err)
}

// InvalidMergeStrategyTypeError represents an error that occurs when a user specifies an invalid merge strategy in the
// terragrunt configuration file.
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

// DependencyDirNotFoundError represents an error that occurs when a dependency block
// specifies a directory that does not exist.
type DependencyDirNotFoundError struct {
	Dir []string
}

func (err DependencyDirNotFoundError) Error() string {
	return fmt.Sprintf(
		"Found paths in the 'dependencies' block that do not exist: %v", err.Dir,
	)
}

// DuplicatedGenerateBlocksError represents an error that occurs when a terragrunt configuration file contains
// multiple generate blocks with the same name.
type DuplicatedGenerateBlocksError struct {
	BlockName []string
}

func (err DuplicatedGenerateBlocksError) Error() string {
	return fmt.Sprintf(
		"Detected generate blocks with the same name: %v", err.BlockName,
	)
}

// TFVarFileNotFoundError represents an error that occurs when a terragrunt configuration file specifies a tfvars file
// that does not exist.
type TFVarFileNotFoundError struct {
	File  string
	Cause string
}

func (err TFVarFileNotFoundError) Error() string {
	return fmt.Sprintf("TFVarFileNotFound: Could not find a %s. Cause: %s.", err.File, err.Cause)
}

// WrongNumberOfParamsError represents an error that occurs when a function is
// called with the wrong number of parameters.
type WrongNumberOfParamsError struct {
	Func     string
	Expected string
	Actual   int
}

func (err WrongNumberOfParamsError) Error() string {
	return fmt.Sprintf("Expected %s params for function %s, but got %d", err.Expected, err.Func, err.Actual)
}

// InvalidParameterTypeError represents an error that occurs when a function is called with a parameter of the wrong
// type.
type InvalidParameterTypeError struct {
	Expected string
	Actual   string
}

func (err InvalidParameterTypeError) Error() string {
	return fmt.Sprintf("Expected param of type %s but got %s", err.Expected, err.Actual)
}

// ParentFileNotFoundError represents an error that occurs when a parent file is not found
// in the parent directories of a given path.
type ParentFileNotFoundError struct {
	Path  string
	File  string
	Cause string
}

func (err ParentFileNotFoundError) Error() string {
	return fmt.Sprintf(
		"ParentFileNotFoundError: Could not find a %s in any of the parent folders of %s. Cause: %s.",
		err.File,
		err.Path,
		err.Cause,
	)
}

// InvalidGetEnvParamsError represents an error that occurs when the
// get_env function is called with an invalid number of
// parameters.
type InvalidGetEnvParamsError struct {
	ActualNumParams int
	Example         string
}

func (err InvalidGetEnvParamsError) Error() string {
	return fmt.Sprintf(
		"InvalidGetEnvParamsError: Expected one or two parameters (%s) for get_env but got %d.",
		err.Example,
		err.ActualNumParams,
	)
}

// EnvVarNotFoundError represents an error that occurs when an environment variable is not found.
type EnvVarNotFoundError struct {
	EnvVar string
}

func (err EnvVarNotFoundError) Error() string {
	return fmt.Sprintf("EnvVarNotFoundError: Required environment variable %s - not found", err.EnvVar)
}

// InvalidEnvParamNameError represents an error that occurs when an environment variable name is invalid.
type InvalidEnvParamNameError struct {
	EnvVarName string
}

func (err InvalidEnvParamNameError) Error() string {
	return fmt.Sprintf("InvalidEnvParamNameError: Invalid environment variable name - (%s) ", err.EnvVarName)
}

// EmptyStringNotAllowedError represents an error that occurs when an empty string is not allowed.
type EmptyStringNotAllowedError string

func (err EmptyStringNotAllowedError) Error() string {
	return "Empty string value is not allowed for " + string(err)
}

// TerragruntConfigNotFoundError represents an error that occurs when a Terragrunt configuration file is not found.
type TerragruntConfigNotFoundError struct {
	Path string
}

func (err TerragruntConfigNotFoundError) Error() string {
	return fmt.Sprintf("Terragrunt config %s not found", err.Path)
}

// InvalidSourceURLError represents an error that occurs when the source URL in a module is invalid.
type InvalidSourceURLError struct {
	ModulePath       string
	ModuleSourceURL  string
	TerragruntSource string
}

func (err InvalidSourceURLError) Error() string {
	return fmt.Sprintf("The --terragrunt-source parameter is set to '%s', but the source URL in the module at '%s' is invalid: '%s'. Note that the module URL must have a double-slash to separate the repo URL from the path within the repo!", err.TerragruntSource, err.ModulePath, err.ModuleSourceURL) //nolint:lll
}

// InvalidSourceURLWithMapError represents an error that occurs when the source URL in a module is invalid.
type InvalidSourceURLWithMapError struct {
	ModulePath      string
	ModuleSourceURL string
}

func (err InvalidSourceURLWithMapError) Error() string {
	return fmt.Sprintf("The --terragrunt-source-map parameter was passed in, but the source URL in the module at '%s' is invalid: '%s'. Note that the module URL must have a double-slash to separate the repo URL from the path within the repo!", err.ModulePath, err.ModuleSourceURL) //nolint:lll
}

// ParsingModulePathError represents an error that occurs when the module path cannot be parsed from the source URL.
type ParsingModulePathError struct {
	ModuleSourceURL string
}

func (err ParsingModulePathError) Error() string {
	return fmt.Sprintf(
		"Unable to obtain the module path from the source URL '%s'. Ensure that the URL is in a supported format.",
		err.ModuleSourceURL,
	)
}

// InvalidSopsFormatError represents an error that occurs when a file is not a valid format or encoding.
type InvalidSopsFormatError struct {
	SourceFilePath string
}

func (err InvalidSopsFormatError) Error() string {
	return fmt.Sprintf(
		"File %s is not a valid format or encoding. Terragrunt will only decrypt yaml or json files in UTF-8 encoding.",
		err.SourceFilePath,
	)
}

// InvalidIncludeKeyError represents an error that occurs when an include block is not found in the current config.
type InvalidIncludeKeyError struct {
	name string
}

func (err InvalidIncludeKeyError) Error() string {
	return fmt.Sprintf("There is no include block in the current config with the label '%s'", err.name)
}

// Dependency Custom error types

// DependencyConfigNotFoundError represents an error that occurs when a
// dependency block specifies a terragrunt configuration file that does not exist.
type DependencyConfigNotFoundError struct {
	Path string
}

func (err DependencyConfigNotFoundError) Error() string {
	return err.Path + " does not exist"
}

// TerragruntOutputParsingError represents an error that occurs when terragrunt output parsing fails.
type TerragruntOutputParsingError struct {
	Path string
	Err  error
}

func (err TerragruntOutputParsingError) Error() string {
	return fmt.Sprintf("Could not parse output from terragrunt config %s. Underlying error: %s", err.Path, err.Err)
}

// TerragruntOutputEncodingError represents an error that occurs when terragrunt output encoding fails.
type TerragruntOutputEncodingError struct {
	Path string
	Err  error
}

func (err TerragruntOutputEncodingError) Error() string {
	return fmt.Sprintf("Could not encode output from terragrunt config %s. Underlying error: %s", err.Path, err.Err)
}

// TerragruntOutputListEncodingError represents an error that occurs when terragrunt output list encoding fails.
type TerragruntOutputListEncodingError struct {
	Paths []string
	Err   error
}

func (err TerragruntOutputListEncodingError) Error() string {
	return fmt.Sprintf(
		"Could not encode output from list of terragrunt configs %v. Underlying error: %s",
		err.Paths,
		err.Err,
	)
}

// TerragruntOutputTargetNoOutputsError represents an error that occurs when terragrunt output target has no outputs.
type TerragruntOutputTargetNoOutputsError struct {
	targetConfig  string
	currentConfig string
}

func (err TerragruntOutputTargetNoOutputsError) Error() string {
	return fmt.Sprintf(
		"%s is a dependency of %s but detected no outputs. Either the target module has not been applied yet, or the module has no outputs. If this is expected, set the skip_outputs flag to true on the dependency block.", //nolint:lll
		err.targetConfig,
		err.currentConfig,
	)
}

// DependencyCycleErrors represents a slice of errors that occur when a dependency cycle is detected.
type DependencyCycleErrors []string

func (err DependencyCycleErrors) Error() string {
	return "Found a dependency cycle between modules: " + strings.Join([]string(err), " -> ")
}
