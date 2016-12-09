package config

import (
	"regexp"
	"fmt"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"path/filepath"
	"github.com/gruntwork-io/terragrunt/util"
)

var INTERPOLATION_SYNTAX_REGEX = regexp.MustCompile("\\$\\{.*?\\}")
var HELPER_FUNCTION_SYNTAX_REGEX = regexp.MustCompile("\\$\\{(.*?)\\(\\)\\}")
var MAX_PARENT_FOLDERS_TO_CHECK = 100

// Given a string value from a .terragrunt config file, parse the string, resolve any calls to helper functions using
// the syntax ${...}, and return the final value.
func ResolveTerragruntConfigString(terragruntConfigString string, include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (resolved string, finalErr error) {
	// The function we pass to ReplaceAllStringFunc cannot return an error, so we have to use named error
	// parameters to capture such errors.
	resolved = INTERPOLATION_SYNTAX_REGEX.ReplaceAllStringFunc(terragruntConfigString, func(str string) string {
		out, err := resolveTerragruntInterpolation(str, include, terragruntOptions)
		if err != nil {
			finalErr = err
		}
		return out
	})

	return
}

// Resolve a single call to an interpolation function of the format ${some_function()} in a .terragrunt file
func resolveTerragruntInterpolation(str string, include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	matches := HELPER_FUNCTION_SYNTAX_REGEX.FindStringSubmatch(str)
	if len(matches) == 2 {
		return executeTerragruntHelperFunction(matches[1], include, terragruntOptions)
	} else {
		return "", errors.WithStackTrace(InvalidInterpolationSyntax(str))
	}
}

// Execute a single Terragrunt helper function and return its value as a string
func executeTerragruntHelperFunction(functionName string, include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	switch functionName {
	case "find_in_parent_folders": return findInParentFolders(terragruntOptions)
	case "path_relative_to_include": return pathRelativeToInclude(include, terragruntOptions)
	default: return "", errors.WithStackTrace(UnknownHelperFunction(functionName))
	}
}

// Find a parent .terragrunt file in the parent folders above the current .terragrunt file and return its path
func findInParentFolders(terragruntOptions *options.TerragruntOptions) (string, error) {
	previousDir, err := filepath.Abs(filepath.Dir(terragruntOptions.TerragruntConfigPath))
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	// To avoid getting into an accidental infinite loop (e.g. do to cyclical symlinks), set a max on the number of
	// parent folders we'll check
	for i := 0; i < MAX_PARENT_FOLDERS_TO_CHECK; i++ {
		currentDir := filepath.Dir(previousDir)
		if currentDir == previousDir {
			return "", errors.WithStackTrace(ParentTerragruntConfigNotFound(terragruntOptions.TerragruntConfigPath))
		}

		configPath := filepath.Join(currentDir, DefaultTerragruntConfigPath)
		if util.FileExists(configPath) {
			return util.GetPathRelativeTo(configPath, filepath.Dir(terragruntOptions.TerragruntConfigPath))
		}

		previousDir = currentDir
	}

	return "", errors.WithStackTrace(CheckedTooManyParentFolders(terragruntOptions.TerragruntConfigPath))
}

// Return the relative path between the included .terragrunt file and the current .terragrunt file
func pathRelativeToInclude(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	if include == nil {
		return ".", nil
	}

	includedConfigPath, err := ResolveTerragruntConfigString(include.Path, include, terragruntOptions)
	if err != nil {
		return "", err
	}

	includePath := filepath.Dir(includedConfigPath)
	currentPath := filepath.Dir(terragruntOptions.TerragruntConfigPath)

	if !filepath.IsAbs(includePath) {
		includePath = filepath.Join(currentPath, includePath)
	}

	return util.GetPathRelativeTo(currentPath, includePath)
}

// Custom error types

type InvalidInterpolationSyntax string
func (err InvalidInterpolationSyntax) Error() string {
	return fmt.Sprintf("Invalid interpolation syntax. Expected syntax of the form '${function_name()}', but got '%s'", string(err))
}

type UnknownHelperFunction string
func (err UnknownHelperFunction) Error() string {
	return fmt.Sprintf("Unknown helper function: %s", string(err))
}

type ParentTerragruntConfigNotFound string
func (err ParentTerragruntConfigNotFound) Error() string {
	return fmt.Sprintf("Could not find a %s config file in any of the parent folders of %s", DefaultTerragruntConfigPath, string(err))
}

type CheckedTooManyParentFolders string
func (err CheckedTooManyParentFolders) Error() string {
	return fmt.Sprintf("Could not find a %s config file in a parent folder of %s after checking %d parent folders", DefaultTerragruntConfigPath, string(err), MAX_PARENT_FOLDERS_TO_CHECK)
}