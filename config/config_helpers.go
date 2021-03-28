package config

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/go-getter"
	"github.com/hashicorp/hcl/v2"
	tflang "github.com/hashicorp/terraform/lang"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"go.mozilla.org/sops/v3/decrypt"

	"github.com/gruntwork-io/terragrunt/aws_helper"
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
	IsRequired   bool
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
		"read_terragrunt_config":                       readTerragruntConfigAsFuncImpl(terragruntOptions),
		"get_platform":                                 wrapVoidToStringAsFuncImpl(getPlatform, extensions.Include, terragruntOptions),
		"get_terragrunt_dir":                           wrapVoidToStringAsFuncImpl(getTerragruntDir, extensions.Include, terragruntOptions),
		"get_terraform_command":                        wrapVoidToStringAsFuncImpl(getTerraformCommand, extensions.Include, terragruntOptions),
		"get_terraform_cli_args":                       wrapVoidToStringSliceAsFuncImpl(getTerraformCliArgs, extensions.Include, terragruntOptions),
		"get_parent_terragrunt_dir":                    wrapVoidToStringAsFuncImpl(getParentTerragruntDir, extensions.Include, terragruntOptions),
		"get_aws_account_id":                           wrapVoidToStringAsFuncImpl(getAWSAccountID, extensions.Include, terragruntOptions),
		"get_aws_caller_identity_arn":                  wrapVoidToStringAsFuncImpl(getAWSCallerIdentityARN, extensions.Include, terragruntOptions),
		"get_aws_caller_identity_user_id":              wrapVoidToStringAsFuncImpl(getAWSCallerIdentityUserID, extensions.Include, terragruntOptions),
		"get_terraform_commands_that_need_vars":        wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_VARS),
		"get_terraform_commands_that_need_locking":     wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_LOCKING),
		"get_terraform_commands_that_need_input":       wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_INPUT),
		"get_terraform_commands_that_need_parallelism": wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_PARALLELISM),
		"sops_decrypt_file":                            wrapStringSliceToStringAsFuncImpl(sopsDecryptFile, extensions.Include, terragruntOptions),
		"get_terragrunt_source_cli_flag":               wrapVoidToStringAsFuncImpl(getTerragruntSourceCliFlag, extensions.Include, terragruntOptions),
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

// Return the OS platform
func getPlatform(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	return runtime.GOOS, nil
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

	switch len(parameters) {
	case 1:
		envVariable.IsRequired = true
		envVariable.Name = parameters[0]
	case 2:
		envVariable.Name = parameters[0]
		envVariable.DefaultValue = parameters[1]
	default:
		return envVariable, errors.WithStackTrace(InvalidGetEnvParams{ActualNumParams: len(parameters), Example: `getEnv("<NAME>", "[DEFAULT]")`})
	}

	if envVariable.Name == "" {
		return envVariable, errors.WithStackTrace(InvalidEnvParamName{EnvVarName: parameters[0]})
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

	cmdOutput, err := shell.RunShellCommandWithOutput(terragruntOptions, currentPath, suppressOutput, false, args[0], args[1:]...)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	value := strings.TrimSuffix(cmdOutput.Stdout, "\n")

	if suppressOutput {
		terragruntOptions.Logger.Debugf("run_cmd output: [REDACTED]")
	} else {
		terragruntOptions.Logger.Debugf("run_cmd output: [%s]", value)
	}

	return value, nil
}

func getEnvironmentVariable(parameters []string, include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	parameterMap, err := parseGetEnvParameters(parameters)

	if err != nil {
		return "", errors.WithStackTrace(err)
	}
	envValue, exists := terragruntOptions.Env[parameterMap.Name]

	if !exists {
		if parameterMap.IsRequired {
			return "", errors.WithStackTrace(EnvVarNotFound{EnvVar: parameterMap.Name})
		}
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

		fileToFind := GetDefaultConfigPath(currentDir)
		if fileToFindParam != "" {
			fileToFind = util.JoinPath(currentDir, fileToFindParam)
		}

		if util.FileExists(fileToFind) {
			return fileToFind, nil
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

// getTerraformCommand returns the current terraform command in execution
func getTerraformCommand(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	return terragruntOptions.TerraformCommand, nil
}

// getTerraformCliArgs returns cli args for terraform
func getTerraformCliArgs(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) ([]string, error) {
	return terragruntOptions.TerraformCliArgs, nil
}

// Return the AWS account id associated to the current set of credentials
func getAWSAccountID(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	accountID, err := aws_helper.GetAWSAccountID(terragruntOptions)
	if err == nil {
		return accountID, nil
	}
	return "", err
}

// Return the ARN of the AWS identity associated with the current set of credentials
func getAWSCallerIdentityARN(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	identityARN, err := aws_helper.GetAWSIdentityArn(terragruntOptions)
	if err == nil {
		return identityARN, nil
	}
	return "", err
}

// Return the UserID of the AWS identity associated with the current set of credentials
func getAWSCallerIdentityUserID(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	userID, err := aws_helper.GetAWSUserID(terragruntOptions)
	if err == nil {
		return userID, nil
	}
	return "", err
}

// Parse the terragrunt config and return a representation that can be used as a reference. If given a default value,
// this will return the default if the terragrunt config file does not exist.
func readTerragruntConfig(configPath string, defaultVal *cty.Value, terragruntOptions *options.TerragruntOptions) (cty.Value, error) {
	// target config check: make sure the target config exists. If the file does not exist, and there is no default val,
	// return an error. If the file does not exist but there is a default val, return the default val. Otherwise,
	// proceed to parse the file as a terragrunt config file.
	targetConfig := getCleanedTargetConfigPath(configPath, terragruntOptions.TerragruntConfigPath)
	targetConfigFileExists := util.FileExists(targetConfig)
	if !targetConfigFileExists && defaultVal == nil {
		return cty.NilVal, errors.WithStackTrace(TerragruntConfigNotFound{Path: targetConfig})
	} else if !targetConfigFileExists {
		return *defaultVal, nil
	}

	// We update the context of terragruntOptions to the config being read in.
	targetOptions := terragruntOptions.Clone(targetConfig)
	config, err := ParseConfigFile(targetConfig, targetOptions, nil)
	if err != nil {
		return cty.NilVal, err
	}
	// We have to set the rendered outputs here because ParseConfigFile will not do so on the TerragruntConfig. The
	// outputs are stored in a special map that is used only for rendering and thus is not available when we try to
	// serialize the config for consumption.
	// NOTE: this will not call terragrunt output, since all the values are cached from the ParseConfigFile call
	// NOTE: we don't use range here because range will copy the slice, thereby undoing the set attribute.
	for i := 0; i < len(config.TerragruntDependencies); i++ {
		config.TerragruntDependencies[i].setRenderedOutputs(targetOptions)
	}

	return terragruntConfigAsCty(config)
}

// Create a cty Function that can be used to for calling read_terragrunt_config.
func readTerragruntConfigAsFuncImpl(terragruntOptions *options.TerragruntOptions) function.Function {
	return function.New(&function.Spec{
		// Takes one required string param
		Params: []function.Parameter{function.Parameter{Type: cty.String}},
		// And optional param that takes anything
		VarParam: &function.Parameter{Type: cty.DynamicPseudoType},
		// We don't know the return type until we parse the terragrunt config, so we use a dynamic type
		Type: function.StaticReturnType(cty.DynamicPseudoType),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			numParams := len(args)
			if numParams == 0 || numParams > 2 {
				return cty.NilVal, errors.WithStackTrace(WrongNumberOfParams{Func: "read_terragrunt_config", Expected: "1 or 2", Actual: numParams})
			}

			configPath, err := ctySliceToStringSlice(args[:1])
			if err != nil {
				return cty.NilVal, err
			}

			var defaultVal *cty.Value = nil
			if numParams == 2 {
				defaultVal = &args[1]
			}
			return readTerragruntConfig(configPath[0], defaultVal, terragruntOptions)
		},
	})
}

// Returns a cleaned path to the target config (the `terragrunt.hcl` or `terragrunt.hcl.json` file), handling relative
// paths correctly. This will automatically append `terragrunt.hcl` or `terragrunt.hcl.json` to the path if the target
// path is a directory.
func getCleanedTargetConfigPath(configPath string, workingPath string) string {
	cwd := filepath.Dir(workingPath)
	targetConfig := configPath
	if !filepath.IsAbs(targetConfig) {
		targetConfig = util.JoinPath(cwd, targetConfig)
	}
	if util.IsDir(targetConfig) {
		targetConfig = GetDefaultConfigPath(targetConfig)
	}
	return util.CleanPath(targetConfig)
}

// If one of the xxx-all commands is called with the --terragrunt-source parameter, then for each module, we need to
// build its own --terragrunt-source parameter by doing the following:
//
// 1. Read the source URL from the Terragrunt configuration of each module
// 2. Extract the path from that URL (the part after a double-slash)
// 3. Append the path to the --terragrunt-source parameter
//
// Example:
//
// --terragrunt-source: /source/infrastructure-modules
// source param in module's terragrunt.hcl: git::git@github.com:acme/infrastructure-modules.git//networking/vpc?ref=v0.0.1
//
// This method will return: /source/infrastructure-modules//networking/vpc
//
func GetTerragruntSourceForModule(sourcePath string, modulePath string, moduleTerragruntConfig *TerragruntConfig) (string, error) {
	if sourcePath == "" || moduleTerragruntConfig.Terraform == nil || moduleTerragruntConfig.Terraform.Source == nil || *moduleTerragruntConfig.Terraform.Source == "" {
		return "", nil
	}

	// use go-getter to split the module source string into a valid URL and subdirectory (if // is present)
	moduleUrl, moduleSubdir := getter.SourceDirSubdir(*moduleTerragruntConfig.Terraform.Source)

	// if both URL and subdir are missing, something went terribly wrong
	if moduleUrl == "" && moduleSubdir == "" {
		return "", errors.WithStackTrace(InvalidSourceUrl{ModulePath: modulePath, ModuleSourceUrl: *moduleTerragruntConfig.Terraform.Source, TerragruntSource: sourcePath})
	}
	// if only subdir is missing, check if we can obtain a valid module name from the URL portion
	if moduleUrl != "" && moduleSubdir == "" {
		moduleSubdirFromUrl, err := getModulePathFromSourceUrl(moduleUrl)
		if err != nil {
			return moduleSubdirFromUrl, err
		}
		return util.JoinTerraformModulePath(sourcePath, moduleSubdirFromUrl), nil
	}

	return util.JoinTerraformModulePath(sourcePath, moduleSubdir), nil
}

// Parse sourceUrl not containing '//', and attempt to obtain a module path.
// Example:
//
// sourceUrl = "git::ssh://git@ghe.ourcorp.com/OurOrg/module-name.git"
// will return "module-name".

func getModulePathFromSourceUrl(sourceUrl string) (string, error) {

	// Regexp for module name extraction. It assumes that the query string has already been stripped off.
	// Then we simply capture anything after the last slash, and before `.` or end of string.
	var moduleNameRegexp = regexp.MustCompile(`(?:.+/)(.+?)(?:\.|$)`)

	// strip off the query string if present
	sourceUrl = strings.Split(sourceUrl, "?")[0]

	matches := moduleNameRegexp.FindStringSubmatch(sourceUrl)

	// if regexp returns less/more than the full match + 1 capture group, then something went wrong with regex (invalid source string)
	if len(matches) != 2 {
		return "", errors.WithStackTrace(ErrorParsingModulePath{ModuleSourceUrl: sourceUrl})
	}

	return matches[1], nil
}

//
// A map that caches the results of a decrypt operation via sops. Each decryption
// operation can take several seconds, so this cache speeds up terragrunt executions
// where the same sops files are referenced multiple times.
//
// The keys are the canonical paths to the encrypted files, and the values are the
// plain-text result of the decrypt operation.
//
var sopsCache = make(map[string]string)

// decrypts and returns sops encrypted utf-8 yaml or json data as a string
func sopsDecryptFile(params []string, include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	numParams := len(params)

	var sourceFile string

	if numParams > 0 {
		sourceFile = params[0]
	}
	if numParams != 1 {
		return "", errors.WithStackTrace(WrongNumberOfParams{Func: "sops_decrypt_file", Expected: "1", Actual: numParams})
	}

	var format string
	switch ext := path.Ext(sourceFile); ext {
	case ".json":
		format = "json"
	case ".yaml", ".yml":
		format = "yaml"
	default:
		return "", errors.WithStackTrace(InvalidSopsFormat{SourceFilePath: sourceFile})
	}

	canonicalSourceFile, err := util.CanonicalPath(sourceFile, terragruntOptions.WorkingDir)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	if val, ok := sopsCache[canonicalSourceFile]; ok {
		return val, nil
	}

	rawData, err := decrypt.File(sourceFile, format)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	if utf8.Valid(rawData) {
		value := string(rawData)
		sopsCache[canonicalSourceFile] = value
		return value, nil
	}

	return "", errors.WithStackTrace(InvalidSopsFormat{SourceFilePath: sourceFile})
}

// Return the location of the Terraform files provided via --terragrunt-source
func getTerragruntSourceCliFlag(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	return terragruntOptions.Source, nil
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
	ActualNumParams int
	Example         string
}

type EnvVarNotFound struct {
	EnvVar string
}

type InvalidEnvParamName struct {
	EnvVarName string
}

func (err InvalidGetEnvParams) Error() string {
	return fmt.Sprintf("InvalidGetEnvParams: Expected one or two parameters (%s) for get_env but got %d.", err.Example, err.ActualNumParams)
}

func (err InvalidEnvParamName) Error() string {
	return fmt.Sprintf("InvalidEnvParamName: Invalid environment variable name - (%s) ", err.EnvVarName)
}

func (err EnvVarNotFound) Error() string {
	return fmt.Sprintf("EnvVarNotFound: Required environment variable %s - not found", err.EnvVar)
}

type EmptyStringNotAllowed string

func (err EmptyStringNotAllowed) Error() string {
	return fmt.Sprintf("Empty string value is not allowed for %s", string(err))
}

type TerragruntConfigNotFound struct {
	Path string
}

func (err TerragruntConfigNotFound) Error() string {
	return fmt.Sprintf("Terragrunt config %s not found", err.Path)
}

type InvalidSourceUrl struct {
	ModulePath       string
	ModuleSourceUrl  string
	TerragruntSource string
}

func (err InvalidSourceUrl) Error() string {
	return fmt.Sprintf("The --terragrunt-source parameter is set to '%s', but the source URL in the module at '%s' is invalid: '%s'. Note that the module URL must have a double-slash to separate the repo URL from the path within the repo!", err.TerragruntSource, err.ModulePath, err.ModuleSourceUrl)
}

type ErrorParsingModulePath struct {
	ModuleSourceUrl string
}

func (err ErrorParsingModulePath) Error() string {
	return fmt.Sprintf("Unable to obtain the module path from the source URL '%s'. Ensure that the URL is in a supported format.", err.ModuleSourceUrl)
}

type InvalidSopsFormat struct {
	SourceFilePath string
}

func (err InvalidSopsFormat) Error() string {
	return fmt.Sprintf("File %s is not a valid format or encoding. Terragrunt will only decrypt yaml or json files in UTF-8 encoding.", err.SourceFilePath)
}
