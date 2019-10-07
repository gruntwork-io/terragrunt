package config

import (
	"fmt"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/hashicorp/hcl2/hcl"
	tflang "github.com/hashicorp/terraform/lang"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

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

// List of terraform commands that accept -parallelism=
var TERRAFORM_COMMANDS_NEED_PARALLELISM = []string{
	"apply",
	"plan",
	"destroy",
}

type EnvVar struct {
	Name         string
	DefaultValue string
}

// EvalContextExtensions provides various extensions to the evaluation context to enhance the parsing capabilities.
type EvalContextExtensions struct {
	// Include is used to specify another config that should be imported and merged before the final TerragruntConfig is
	// returned.
	Include *IncludeConfig

	// Locals are preevaluated variable bindings that can be used by reference in the code.
	Locals *cty.Value

	// DecodedDependencies are references of other terragrunt config. This contains the following attributes that map to
	// various fields related to that config:
	// - outputs: The map of outputs from the terraform state obtained by running `terragrunt output` on that target
	//            config.
	DecodedDependencies *cty.Value
}

// Create an EvalContext for the HCL2 parser. We can define functions and variables in this context that the HCL2 parser
// will make available to the Terragrunt configuration during parsing.
func CreateTerragruntEvalContext(
	filename string,
	terragruntOptions *options.TerragruntOptions,
	extensions EvalContextExtensions,
) *hcl.EvalContext {
	tfscope := tflang.Scope{
		BaseDir: filepath.Dir(filename),
	}

	terragruntFunctions := map[string]function.Function{
		"find_in_parent_folders":                       wrapStringSliceToStringAsFuncImpl(findInParentFolders, extensions.Include, terragruntOptions),
		"path_relative_to_include":                     wrapVoidToStringAsFuncImpl(pathRelativeToInclude, extensions.Include, terragruntOptions),
		"path_relative_from_include":                   wrapVoidToStringAsFuncImpl(pathRelativeFromInclude, extensions.Include, terragruntOptions),
		"get_env":                                      wrapStringSliceToStringAsFuncImpl(getEnvironmentVariable, extensions.Include, terragruntOptions),
		"run_cmd":                                      wrapStringSliceToStringAsFuncImpl(runCommand, extensions.Include, terragruntOptions),
		"get_terragrunt_dir":                           wrapVoidToStringAsFuncImpl(getTerragruntDir, extensions.Include, terragruntOptions),
		"get_parent_terragrunt_dir":                    wrapVoidToStringAsFuncImpl(getParentTerragruntDir, extensions.Include, terragruntOptions),
		"get_aws_account_id":                           wrapVoidToStringAsFuncImpl(getAWSAccountID, extensions.Include, terragruntOptions),
		"get_aws_caller_identity_arn":                  wrapVoidToStringAsFuncImpl(getAWSCallerIdentityARN, extensions.Include, terragruntOptions),
		"get_terraform_commands_that_need_vars":        wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_VARS),
		"get_terraform_commands_that_need_locking":     wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_LOCKING),
		"get_terraform_commands_that_need_input":       wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_INPUT),
		"get_terraform_commands_that_need_parallelism": wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_PARALLELISM),
	}

	functions := map[string]function.Function{}
	for k, v := range tfscope.Functions() {
		functions[k] = v
	}
	for k, v := range terragruntFunctions {
		functions[k] = v
	}

	ctx := &hcl.EvalContext{
		Functions: functions,
	}
	ctx.Variables = map[string]cty.Value{}
	if extensions.Locals != nil {
		ctx.Variables["local"] = *extensions.Locals
	}
	if extensions.DecodedDependencies != nil {
		ctx.Variables["dependency"] = *extensions.DecodedDependencies
	}
	return ctx
}

// Return the directory where the Terragrunt configuration file lives
func getTerragruntDir(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	terragruntConfigFileAbsPath, err := filepath.Abs(terragruntOptions.TerragruntConfigPath)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return filepath.ToSlash(filepath.Dir(terragruntConfigFileAbsPath)), nil
}

// Return the parent directory where the Terragrunt configuration file lives
func getParentTerragruntDir(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	parentPath, err := pathRelativeFromInclude(include, terragruntOptions)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	currentPath := filepath.Dir(terragruntOptions.TerragruntConfigPath)
	parentPath, err = filepath.Abs(filepath.Join(currentPath, parentPath))
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return filepath.ToSlash(parentPath), nil
}

func parseGetEnvParameters(parameters []string) (EnvVar, error) {
	envVariable := EnvVar{}
	if len(parameters) != 2 {
		return envVariable, errors.WithStackTrace(InvalidGetEnvParams{ExpectedNumParams: 2, ActualNumParams: 2, Example: `getEnv("<NAME>", "<DEFAULT>")`})
	}

	envVariable.Name = parameters[0]
	envVariable.DefaultValue = parameters[1]

	if envVariable.Name == "" {
		return envVariable, errors.WithStackTrace(InvalidGetEnvParams{ExpectedNumParams: 2, ActualNumParams: 2, Example: `getEnv("<NAME>", "<DEFAULT>")`})
	}

	return envVariable, nil
}

// runCommand is a helper function that runs a command and returns the stdout as the interporation
// result
func runCommand(args []string, include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	if len(args) == 0 {
		return "", errors.WithStackTrace(EmptyStringNotAllowed("parameter to the run_cmd function"))
	}

	suppressOutput := false
	if args[0] == "--terragrunt-quiet" {
		suppressOutput = true
		args = append(args[:0], args[1:]...)
	}

	currentPath := filepath.Dir(terragruntOptions.TerragruntConfigPath)

	cmdOutput, err := shell.RunShellCommandWithOutput(terragruntOptions, currentPath, suppressOutput, args[0], args[1:]...)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	if suppressOutput {
		terragruntOptions.Logger.Printf("run_cmd output: [REDACTED]")
	} else {
		terragruntOptions.Logger.Printf("run_cmd output: [%s]", cmdOutput.Stdout)
	}

	return cmdOutput.Stdout, nil
}

func getEnvironmentVariable(parameters []string, include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	parameterMap, err := parseGetEnvParameters(parameters)

	if err != nil {
		return "", errors.WithStackTrace(err)
	}
	envValue, exists := terragruntOptions.Env[parameterMap.Name]

	if !exists {
		envValue = parameterMap.DefaultValue
	}

	return envValue, nil
}

// Find a parent Terragrunt configuration file in the parent folders above the current Terragrunt configuration file
// and return its path
func findInParentFolders(params []string, include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	numParams := len(params)

	var fileToFindParam string
	var fallbackParam string

	if numParams > 0 {
		fileToFindParam = params[0]
	}
	if numParams > 1 {
		fallbackParam = params[1]
	}
	if numParams > 2 {
		return "", errors.WithStackTrace(WrongNumberOfParams{Func: "find_in_parent_folders", Expected: "0, 1, or 2", Actual: numParams})
	}

	previousDir, err := filepath.Abs(filepath.Dir(terragruntOptions.TerragruntConfigPath))
	previousDir = filepath.ToSlash(previousDir)

	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	fileToFindStr := DefaultTerragruntConfigPath
	if fileToFindParam != "" {
		fileToFindStr = fileToFindParam
	}

	// To avoid getting into an accidental infinite loop (e.g. do to cyclical symlinks), set a max on the number of
	// parent folders we'll check
	for i := 0; i < terragruntOptions.MaxFoldersToCheck; i++ {
		currentDir := filepath.ToSlash(filepath.Dir(previousDir))
		if currentDir == previousDir {
			if numParams == 2 {
				return fallbackParam, nil
			}
			return "", errors.WithStackTrace(ParentFileNotFound{Path: terragruntOptions.TerragruntConfigPath, File: fileToFindStr, Cause: "Traversed all the way to the root"})
		}

		fileToFind := DefaultConfigPath(currentDir)
		if fileToFindParam != "" {
			fileToFind = util.JoinPath(currentDir, fileToFindParam)
		}

		if util.FileExists(fileToFind) {
			return util.GetPathRelativeTo(fileToFind, filepath.Dir(terragruntOptions.TerragruntConfigPath))
		}

		previousDir = currentDir
	}

	return "", errors.WithStackTrace(ParentFileNotFound{Path: terragruntOptions.TerragruntConfigPath, File: fileToFindStr, Cause: fmt.Sprintf("Exceeded maximum folders to check (%d)", terragruntOptions.MaxFoldersToCheck)})
}

// Return the relative path between the included Terragrunt configuration file and the current Terragrunt configuration
// file
func pathRelativeToInclude(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	if include == nil {
		return ".", nil
	}

	includePath := filepath.Dir(include.Path)
	currentPath := filepath.Dir(terragruntOptions.TerragruntConfigPath)

	if !filepath.IsAbs(includePath) {
		includePath = util.JoinPath(currentPath, includePath)
	}

	return util.GetPathRelativeTo(currentPath, includePath)
}

// Return the relative path from the current Terragrunt configuration to the included Terragrunt configuration file
func pathRelativeFromInclude(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	if include == nil {
		return ".", nil
	}

	includePath := filepath.Dir(include.Path)
	currentPath := filepath.Dir(terragruntOptions.TerragruntConfigPath)

	if !filepath.IsAbs(includePath) {
		includePath = util.JoinPath(currentPath, includePath)
	}

	return util.GetPathRelativeTo(includePath, currentPath)
}

// Return the AWS caller identity associated with the current set of credentials
func getAWSCallerID(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (sts.GetCallerIdentityOutput, error) {
	sess, err := session.NewSession()
	if err != nil {
		return sts.GetCallerIdentityOutput{}, errors.WithStackTrace(err)
	}

	if terragruntOptions.IamRole != "" {
		sess.Config.Credentials = stscreds.NewCredentials(sess, terragruntOptions.IamRole)
	}

	identity, err := sts.New(sess).GetCallerIdentity(nil)
	if err != nil {
		return sts.GetCallerIdentityOutput{}, errors.WithStackTrace(err)
	}

	return *identity, nil
}

// Return the AWS account id associated to the current set of credentials
func getAWSAccountID(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	identity, err := getAWSCallerID(include, terragruntOptions)
	return *identity.Account, err
}

// Return the ARN of the AWS identity associated with the current set of credentials
func getAWSCallerIdentityARN(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	identity, err := getAWSCallerID(include, terragruntOptions)
	return *identity.Arn, err
}

// Custom error types
type WrongNumberOfParams struct {
	Func     string
	Expected string
	Actual   int
}

func (err WrongNumberOfParams) Error() string {
	return fmt.Sprintf("Expected %s params for function %s, but got %d", err.Expected, err.Func, err.Actual)
}

type InvalidParameterType struct {
	Expected string
	Actual   string
}

func (err InvalidParameterType) Error() string {
	return fmt.Sprintf("Expected param of type %s but got %s", err.Expected, err.Actual)
}

type ParentFileNotFound struct {
	Path  string
	File  string
	Cause string
}

func (err ParentFileNotFound) Error() string {
	return fmt.Sprintf("ParentFileNotFound: Could not find a %s in any of the parent folders of %s. Cause: %s.", err.File, err.Path, err.Cause)
}

type InvalidGetEnvParams struct {
	ExpectedNumParams int
	ActualNumParams   int
	Example           string
}

func (err InvalidGetEnvParams) Error() string {
	return fmt.Sprintf("InvalidGetEnvParams: Expected %d parameters (%s) for get_env but got %d.", err.ExpectedNumParams, err.Example, err.ActualNumParams)
}

type EmptyStringNotAllowed string

func (err EmptyStringNotAllowed) Error() string {
	return fmt.Sprintf("Empty string value is not allowed for %s", string(err))
}
