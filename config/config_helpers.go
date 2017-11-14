package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hil/ast"
)

// hcl2 "github.com/hashicorp/hcl2/hcl"

var INTERPOLATION_PARAMETERS = `(\s*"[^"]*?"\s*,?\s*)*`
var INTERPOLATION_SYNTAX_REGEX = regexp.MustCompile(fmt.Sprintf(`\$\{\s*[\w|\.]+(\(%s\))*\s*\}`, INTERPOLATION_PARAMETERS))
var INTERPOLATION_SYNTAX_REGEX_SINGLE = regexp.MustCompile(fmt.Sprintf(`"(%s)"`, INTERPOLATION_SYNTAX_REGEX))
var INTERPOLATION_SYNTAX_REGEX_REMAINING = regexp.MustCompile(`\$\{.*?\}`)
var HELPER_VAR_SYNTAX_REGEX = regexp.MustCompile(`^\$\{\s*var\.(.*?)\s*\}$`)
var HELPER_FUNCTION_SYNTAX_REGEX = regexp.MustCompile(`^\$\{\s*(.*?)\((.*?)\)\s*\}$`)
var HELPER_FUNCTION_PARAM_REGEX = regexp.MustCompile(`\s*"(.*?)"\s*`)
var HELPER_FUNCTION_GET_ENV_PARAMETERS_SYNTAX_REGEX = regexp.MustCompile(`^\s*"(?P<env>[^=]+?)"\s*\,\s*"(?P<default>.*?)"\s*$`)
var MAX_PARENT_FOLDERS_TO_CHECK = 100

// List of terraform commands that accept -lock-timeout
var TERRAFORM_COMMANDS_NEED_LOCKING = []string{
	"apply",
	"destroy",
	"import",
	"init",
	"plan",
	"refresh",
	"taint",
	"untaint",
}

// List of terraform commands that accept -var or -var-file
var TERRAFORM_COMMANDS_NEED_VARS = []string{
	"apply",
	"console",
	"destroy",
	"import",
	"plan",
	"push",
	"refresh",
	"validate",
}

// List of terraform commands that accept -input=
var TERRAFORM_COMMANDS_NEED_INPUT = []string{
	"apply",
	"import",
	"init",
	"plan",
	"refresh",
}

type EnvVar struct {
	Name         string
	DefaultValue string
}

type TerragruntInterpolation struct {
	include *IncludeConfig
	Options *options.TerragruntOptions
	Variables map[string]ast.Variable
}

// Given a string value from a Terragrunt configuration, parse the string, resolve any calls to helper functions using
// the syntax ${...}, and return the final value.
func ResolveTerragruntConfigString(terragruntConfigString string, include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	interpolation := &TerragruntInterpolation{include: include, Options: terragruntOptions}

	// First, we replace all single interpolation syntax (i.e. function directly enclosed within quotes "${function()}")
	terragruntConfigString, err := interpolation.processSingle(terragruntConfigString)
	if err != nil {
		return terragruntConfigString, err
	}
	// Then, we replace all other interpolation functions (i.e. functions not directly enclosed within quotes)
	return interpolation.processMultiple(terragruntConfigString)
}

// Execute a single Terragrunt helper function and return the result
func (ti *TerragruntInterpolation) executeTerragruntHelperFunction(functionName string, parameters string) (interface{}, error) {
	switch functionName {
	case "find_in_parent_folders":
		return ti.findInParentFolders(parameters)
	case "path_relative_to_include":
		return ti.pathRelativeToInclude()
	case "path_relative_from_include":
		return ti.pathRelativeFromInclude()
	case "get_env":
		return ti.getEnvironmentVariable(parameters)
	case "get_tfvars_dir":
		return ti.getTfVarsDir()
	case "get_parent_tfvars_dir":
		return ti.getParentTfVarsDir()
	case "get_aws_account_id":
		return ti.getAWSAccountID()
	case "import_parent_tree":
		return ti.importParentTree(parameters)
	case "prepend":
		return ti.prepend(parameters)
	case "get_terraform_commands_that_need_vars":
		return TERRAFORM_COMMANDS_NEED_VARS, nil
	case "get_terraform_commands_that_need_locking":
		return TERRAFORM_COMMANDS_NEED_LOCKING, nil
	case "get_terraform_commands_that_need_input":
		return TERRAFORM_COMMANDS_NEED_INPUT, nil

	default:
		return "", errors.WithStackTrace(UnknownHelperFunction(functionName))
	}
}

// For all interpolation functions that are called using the syntax "${function_name()}" (i.e. single interpolation function within string,
// functions that return a non-string value we have to get rid of the surrounding quotes and convert the output to HCL syntax. For example,
// for an array, we need to return "v1", "v2", "v3".
func (ti *TerragruntInterpolation) processSingle(configStr string) (resolved string, finalErr error) {
	// The function we pass to ReplaceAllStringFunc cannot return an error, so we have to use named error parameters to capture such errors.
	resolved = INTERPOLATION_SYNTAX_REGEX_SINGLE.ReplaceAllStringFunc(configStr, func(str string) string {
		matches := INTERPOLATION_SYNTAX_REGEX_SINGLE.FindStringSubmatch(str)

		out, err := ti.resolveValue(matches[1])
		if err != nil {
			finalErr = err
			return str
		}

		switch out := out.(type) {
		case string:
			return fmt.Sprintf(`"%s"`, out)
		case []string:
			return util.CommaSeparatedStrings(out)
		default:
			return fmt.Sprintf("%v", out)
		}
	})
	return
}

// For all interpolation functions that are called using the syntax "${function_a()}-${function_b()}" (i.e. multiple interpolation function
// within the same string) or "Some text ${function_name()}" (i.e. string composition), we just replace the interpolation function call
// by the string representation of its return.
func (ti *TerragruntInterpolation) processMultiple(configStr string) (resolved string, finalErr error) {
	// The function we pass to ReplaceAllStringFunc cannot return an error, so we have to use named error parameters to capture such errors.
	resolved = INTERPOLATION_SYNTAX_REGEX.ReplaceAllStringFunc(configStr, func(str string) string {
		out, err := ti.resolveValue(str)
		if err != nil {
			finalErr = err
			return str
		}
		return fmt.Sprintf("%v", out)
	})

	if finalErr == nil {
		// If there is no error, we check if there are remaining look-a-like interpolation strings
		// that have not been considered. If so, they are certainly malformed.
		remaining := INTERPOLATION_SYNTAX_REGEX_REMAINING.FindAllString(resolved, -1)
		if len(remaining) > 0 {
			finalErr = InvalidInterpolationSyntax(strings.Join(remaining, ", "))
		}
	}

	return
}

// Given a string value from a Terragrunt configuration, parse the string, resolve any calls to helper functions using
// Resolve a single call to an interpolation function of the format ${some_function()} in a Terragrunt configuration
// resolveTerragruntInterpolation -> resolveValue
func (ti *TerragruntInterpolation) resolveValue(str string) (interface{}, error) {

	if ok, funcName, funcParams := extractFunction(str); ok {
		return ti.executeTerragruntHelperFunction(funcName, funcParams)
	}

	if ok, variable := extractVariable(str); ok {
		// we need resolution here
		return variable, nil
	}

	return "", errors.WithStackTrace(InvalidInterpolationSyntax(str))
}

func extractFunction(str string) (bool, string, string) {
	matches := HELPER_FUNCTION_SYNTAX_REGEX.FindStringSubmatch(str)
	if len(matches) == 3 {
		return true, matches[1], matches[2]
	}
	return false, "", ""
}

func extractVariable(str string) (bool, string) {
	matches := HELPER_VAR_SYNTAX_REGEX.FindStringSubmatch(str)
	if len(matches) == 2 {
		return true, matches[1]
	}
	return false, ""
}

// Return the directory where the Terragrunt configuration file lives
func (ti *TerragruntInterpolation) getTfVarsDir() (string, error) {
	terragruntConfigFileAbsPath, err := filepath.Abs(ti.Options.TerragruntConfigPath)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return filepath.ToSlash(filepath.Dir(terragruntConfigFileAbsPath)), nil
}

// Return the parent directory where the Terragrunt configuration file lives
func (ti *TerragruntInterpolation) getParentTfVarsDir() (string, error) {
	parentPath, err := ti.pathRelativeFromInclude()
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	currentPath := filepath.Dir(ti.Options.TerragruntConfigPath)
	parentPath, err = filepath.Abs(filepath.Join(currentPath, parentPath))
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return filepath.ToSlash(parentPath), nil
}

func parseGetEnvParameters(parameters string) (EnvVar, error) {
	envVariable := EnvVar{}
	matches := HELPER_FUNCTION_GET_ENV_PARAMETERS_SYNTAX_REGEX.FindStringSubmatch(parameters)
	if len(matches) < 2 {
		return envVariable, errors.WithStackTrace(InvalidGetEnvParams(parameters))
	}

	for index, name := range HELPER_FUNCTION_GET_ENV_PARAMETERS_SYNTAX_REGEX.SubexpNames() {
		if name == "env" {
			envVariable.Name = strings.TrimSpace(matches[index])
		}
		if name == "default" {
			envVariable.DefaultValue = strings.TrimSpace(matches[index])
		}
	}

	return envVariable, nil
}

func (ti *TerragruntInterpolation) getEnvironmentVariable(parameters string) (string, error) {
	parameterMap, err := parseGetEnvParameters(parameters)

	if err != nil {
		return "", errors.WithStackTrace(err)
	}
	envValue, exists := ti.Options.Env[parameterMap.Name]

	if !exists {
		envValue = parameterMap.DefaultValue
	}

	return envValue, nil
}

// Find a parent Terragrunt configuration file in the parent folders above the current Terragrunt configuration file
// and return its path
func (ti *TerragruntInterpolation) findInParentFolders(parameters string) (string, error) {
	fileToFindParam, fallbackParam, numParams, err := parseOptionalQuotedParam(parameters)
	if err != nil {
		return "", err
	}
	if numParams > 0 && fileToFindParam == "" {
		return "", errors.WithStackTrace(EmptyStringNotAllowed("parameter to the find_in_parent_folders_function"))
	}

	previousDir, err := filepath.Abs(filepath.Dir(ti.Options.TerragruntConfigPath))
	previousDir = filepath.ToSlash(previousDir)

	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	// To avoid getting into an accidental infinite loop (e.g. do to cyclical symlinks), set a max on the number of
	// parent folders we'll check
	for i := 0; i < MAX_PARENT_FOLDERS_TO_CHECK; i++ {
		currentDir := filepath.ToSlash(filepath.Dir(previousDir))
		if currentDir == previousDir {
			if numParams == 2 {
				return fallbackParam, nil
			}
			file := fmt.Sprintf("%s or %s", DefaultTerragruntConfigPath, OldTerragruntConfigPath)
			if fileToFindParam != "" {
				file = fileToFindParam
			}
			return "", errors.WithStackTrace(ParentFileNotFound{Path: ti.Options.TerragruntConfigPath, File: file})
		}

		fileToFind := DefaultConfigPath(currentDir)
		if fileToFindParam != "" {
			fileToFind = util.JoinPath(currentDir, fileToFindParam)
		}

		if util.FileExists(fileToFind) {
			return util.GetPathRelativeTo(fileToFind, filepath.Dir(ti.Options.TerragruntConfigPath))
		}

		previousDir = currentDir
	}

	return "", errors.WithStackTrace(CheckedTooManyParentFolders(ti.Options.TerragruntConfigPath))
}

var oneQuotedParamRegex = regexp.MustCompile(`^"([^"]*?)"$`)
var twoQuotedParamsRegex = regexp.MustCompile(`^"([^"]*?)"\s*,\s*"([^"]*?)"$`)

// Parse two optional parameters, wrapped in quotes, passed to a function, and return the parameter values and how many
// of the parameters were actually set. For example, if you have a function foo(bar, baz), where bar and baz are
// optional string parameters, this function will behave as follows:
//
// foo() -> return "", "", 0, nil
// foo("a") -> return "a", "", 1, nil
// foo("a", "b") -> return "a", "b", 2, nil
//
func parseOptionalQuotedParam(parameters string) (string, string, int, error) {
	trimmedParameters := strings.TrimSpace(parameters)
	if trimmedParameters == "" {
		return "", "", 0, nil
	}

	matches := oneQuotedParamRegex.FindStringSubmatch(trimmedParameters)
	if len(matches) == 2 {
		return matches[1], "", 1, nil
	}

	matches = twoQuotedParamsRegex.FindStringSubmatch(trimmedParameters)
	if len(matches) == 3 {
		return matches[1], matches[2], 2, nil
	}

	return "", "", 0, errors.WithStackTrace(InvalidStringParams(parameters))
}

// Return the relative path between the included Terragrunt configuration file and the current Terragrunt configuration
// file
func (ti *TerragruntInterpolation) pathRelativeToInclude() (string, error) {
	if ti.include == nil {
		return ".", nil
	}

	includedConfigPath, err := ResolveTerragruntConfigString(ti.include.Path, ti.include, ti.Options)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	includePath := filepath.Dir(includedConfigPath)
	currentPath := filepath.Dir(ti.Options.TerragruntConfigPath)

	if !filepath.IsAbs(includePath) {
		includePath = util.JoinPath(currentPath, includePath)
	}

	return util.GetPathRelativeTo(currentPath, includePath)
}

// Return the relative path from the current Terragrunt configuration to the included Terragrunt configuration file
func (ti *TerragruntInterpolation) pathRelativeFromInclude() (string, error) {
	if ti.include == nil {
		return ".", nil
	}

	includedConfigPath, err := ResolveTerragruntConfigString(ti.include.Path, ti.include, ti.Options)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	includePath := filepath.Dir(includedConfigPath)
	currentPath := filepath.Dir(ti.Options.TerragruntConfigPath)

	if !filepath.IsAbs(includePath) {
		includePath = util.JoinPath(currentPath, includePath)
	}

	return util.GetPathRelativeTo(includePath, currentPath)
}

func (ti *TerragruntInterpolation) importParentTree(parameters string) ([]string, error) {
	var fileglob string
	retval := []string{}

	previousDir, err := filepath.Abs(filepath.Dir(ti.Options.TerragruntConfigPath))
	previousDir = filepath.ToSlash(previousDir)

	if err != nil {
		return retval, errors.WithStackTrace(err)
	}

	globmatch := HELPER_FUNCTION_PARAM_REGEX.FindStringSubmatch(parameters)
	if len(globmatch) == 2 {
		fileglob = globmatch[1]
	} else {
		return []string{}, nil
	}

	for i := 0; i < MAX_PARENT_FOLDERS_TO_CHECK; i++ {
		currentDir := filepath.ToSlash(filepath.Dir(previousDir))
		if currentDir == previousDir {
			return retval, nil
		}
		pathglob := filepath.Join(currentDir, fileglob)
		matches, _ := filepath.Glob(pathglob)

		if len(matches) > 0 {
			prefixed := util.PrefixListItems("-var-file=", matches)
			// Variables imported from higher level directories have lower precedence
			retval = append(prefixed, retval...)
		}

		previousDir = currentDir
	}

	return retval, nil
}

func (ti *TerragruntInterpolation) prepend(parameters string) ([]string, error) {
	var retval []string

	matches := HELPER_FUNCTION_PARAM_REGEX.FindAllStringSubmatch(parameters, -1)
	if len(matches) < 2 {
		return retval, nil
	}

	prefix := matches[0][1]

	for _, i := range matches[1:] {
		retval = append(retval, prefix+i[1])
	}

	return retval, nil
}

// Return the AWS account id associated to the current set of credentials
func (ti *TerragruntInterpolation) getAWSAccountID() (string, error) {
	sess, err := session.NewSession()
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	identity, err := sts.New(sess).GetCallerIdentity(nil)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return *identity.Account, nil
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

type ParentFileNotFound struct {
	Path string
	File string
}

func (err ParentFileNotFound) Error() string {
	return fmt.Sprintf("Could not find a %s in any of the parent folders of %s", err.File, err.Path)
}

type CheckedTooManyParentFolders string

func (err CheckedTooManyParentFolders) Error() string {
	return fmt.Sprintf("Could not find a Terragrunt config file in a parent folder of %s after checking %d parent folders", string(err), MAX_PARENT_FOLDERS_TO_CHECK)
}

type InvalidGetEnvParams string

func (err InvalidGetEnvParams) Error() string {
	return fmt.Sprintf("Invalid parameters. Expected syntax of the form '${get_env(\"env\", \"default\")}', but got '%s'", string(err))
}

type InvalidStringParams string

func (err InvalidStringParams) Error() string {
	return fmt.Sprintf("Invalid parameters. Expected one string parameter (e.g., ${foo(\"xxx\")}), two string parameters (e.g. ${foo(\"xxx\", \"yyy\")}), or no parameters (e.g., ${foo()}) but got '%s'.", string(err))
}

type EmptyStringNotAllowed string

func (err EmptyStringNotAllowed) Error() string {
	return fmt.Sprintf("Empty string value is not allowed for %s", string(err))
}

type MissingRequiredParams string

func (err MissingRequiredParams) Error() string {
	return fmt.Sprintf("Missing required parameters for %s", string(err))
}
