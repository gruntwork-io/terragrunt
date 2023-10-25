package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"

	"go.mozilla.org/sops/v3/cmd/sops/formats"

	"github.com/hashicorp/go-getter"
	"github.com/hashicorp/hcl/v2"
	tflang "github.com/hashicorp/terraform/lang"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"go.mozilla.org/sops/v3/decrypt"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

const noMatchedPats = 1
const matchedPats = 2

// List of terraform commands that accept -lock-timeout
var TERRAFORM_COMMANDS_NEED_LOCKING = []string{
	"apply",
	"destroy",
	"import",
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

// TrackInclude is used to differentiate between an included config in the current parsing context, and an included
// config that was passed through from a previous parsing context.
type TrackInclude struct {
	// CurrentList is used to track the list of configs that should be imported and merged before the final
	// TerragruntConfig is returned. This preserves the order of the blocks as they appear in the config, so that we can
	// merge the included config in the right order.
	CurrentList []IncludeConfig

	// CurrentMap is the map version of CurrentList that maps the block labels to the included config.
	CurrentMap map[string]IncludeConfig

	// Original is used to track the original included config, and is used for resolving the include related
	// functions.
	Original *IncludeConfig
}

// EvalContextExtensions provides various extensions to the evaluation context to enhance the parsing capabilities.
type EvalContextExtensions struct {
	// TrackInclude represents contexts of included configurations.
	TrackInclude *TrackInclude

	// Locals are preevaluated variable bindings that can be used by reference in the code.
	Locals *cty.Value

	// DecodedDependencies are references of other terragrunt config. This contains the following attributes that map to
	// various fields related to that config:
	// - outputs: The map of outputs from the terraform state obtained by running `terragrunt output` on that target
	//            config.
	DecodedDependencies *cty.Value

	// PartialParseDecodeList is the list of sections that are being decoded in the current config. This can be used to
	// indicate/detect that the current parsing context is partial, meaning that not all configuration values are
	// expected to be available.
	PartialParseDecodeList []PartialDecodeSectionType
}

// Create an EvalContext for the HCL2 parser. We can define functions and variables in this context that the HCL2 parser
// will make available to the Terragrunt configuration during parsing.
func CreateTerragruntEvalContext(
	filename string,
	terragruntOptions *options.TerragruntOptions,
	extensions EvalContextExtensions,
) (*hcl.EvalContext, error) {
	tfscope := tflang.Scope{
		BaseDir: filepath.Dir(filename),
	}

	terragruntFunctions := map[string]function.Function{
		"find_in_parent_folders":                       wrapStringSliceToStringAsFuncImpl(findInParentFolders, extensions.TrackInclude, terragruntOptions),
		"path_relative_to_include":                     wrapStringSliceToStringAsFuncImpl(pathRelativeToInclude, extensions.TrackInclude, terragruntOptions),
		"path_relative_from_include":                   wrapStringSliceToStringAsFuncImpl(pathRelativeFromInclude, extensions.TrackInclude, terragruntOptions),
		"get_env":                                      wrapStringSliceToStringAsFuncImpl(getEnvironmentVariable, extensions.TrackInclude, terragruntOptions),
		"run_cmd":                                      wrapStringSliceToStringAsFuncImpl(runCommand, extensions.TrackInclude, terragruntOptions),
		"read_terragrunt_config":                       readTerragruntConfigAsFuncImpl(terragruntOptions),
		"get_platform":                                 wrapVoidToStringAsFuncImpl(getPlatform, extensions.TrackInclude, terragruntOptions),
		"get_repo_root":                                wrapVoidToStringAsFuncImpl(getRepoRoot, extensions.TrackInclude, terragruntOptions),
		"get_path_from_repo_root":                      wrapVoidToStringAsFuncImpl(getPathFromRepoRoot, extensions.TrackInclude, terragruntOptions),
		"get_path_to_repo_root":                        wrapVoidToStringAsFuncImpl(getPathToRepoRoot, extensions.TrackInclude, terragruntOptions),
		"get_terragrunt_dir":                           wrapVoidToStringAsFuncImpl(getTerragruntDir, extensions.TrackInclude, terragruntOptions),
		"get_original_terragrunt_dir":                  wrapVoidToStringAsFuncImpl(getOriginalTerragruntDir, extensions.TrackInclude, terragruntOptions),
		"get_terraform_command":                        wrapVoidToStringAsFuncImpl(getTerraformCommand, extensions.TrackInclude, terragruntOptions),
		"get_terraform_cli_args":                       wrapVoidToStringSliceAsFuncImpl(getTerraformCliArgs, extensions.TrackInclude, terragruntOptions),
		"get_parent_terragrunt_dir":                    wrapStringSliceToStringAsFuncImpl(getParentTerragruntDir, extensions.TrackInclude, terragruntOptions),
		"get_aws_account_id":                           wrapVoidToStringAsFuncImpl(getAWSAccountID, extensions.TrackInclude, terragruntOptions),
		"get_aws_caller_identity_arn":                  wrapVoidToStringAsFuncImpl(getAWSCallerIdentityARN, extensions.TrackInclude, terragruntOptions),
		"get_aws_caller_identity_user_id":              wrapVoidToStringAsFuncImpl(getAWSCallerIdentityUserID, extensions.TrackInclude, terragruntOptions),
		"get_terraform_commands_that_need_vars":        wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_VARS),
		"get_terraform_commands_that_need_locking":     wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_LOCKING),
		"get_terraform_commands_that_need_input":       wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_INPUT),
		"get_terraform_commands_that_need_parallelism": wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_PARALLELISM),
		"sops_decrypt_file":                            wrapStringSliceToStringAsFuncImpl(sopsDecryptFile, extensions.TrackInclude, terragruntOptions),
		"get_terragrunt_source_cli_flag":               wrapVoidToStringAsFuncImpl(getTerragruntSourceCliFlag, extensions.TrackInclude, terragruntOptions),
		"get_default_retryable_errors":                 wrapVoidToStringSliceAsFuncImpl(getDefaultRetryableErrors, extensions.TrackInclude, terragruntOptions),
		"read_tfvars_file":                             wrapStringSliceToStringAsFuncImpl(readTFVarsFile, extensions.TrackInclude, terragruntOptions),
	}

	// Map with HCL functions introduced in Terraform after v0.15.3, since upgrade to a later version is not supported
	// https://github.com/gruntwork-io/terragrunt/blob/master/go.mod#L22
	terraformCompatibilityFunctions := map[string]function.Function{
		"startswith":  wrapStringSliceToBoolAsFuncImpl(startsWith, extensions.TrackInclude, terragruntOptions),
		"endswith":    wrapStringSliceToBoolAsFuncImpl(endsWith, extensions.TrackInclude, terragruntOptions),
		"strcontains": wrapStringSliceToBoolAsFuncImpl(strContains, extensions.TrackInclude, terragruntOptions),
		"timecmp":     wrapStringSliceToNumberAsFuncImpl(timeCmp, extensions.TrackInclude, terragruntOptions),
	}

	functions := map[string]function.Function{}
	for k, v := range tfscope.Functions() {
		functions[k] = v
	}
	for k, v := range terragruntFunctions {
		functions[k] = v
	}
	for k, v := range terraformCompatibilityFunctions {
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
	if extensions.TrackInclude != nil && len(extensions.TrackInclude.CurrentList) > 0 {
		// For each include block, check if we want to expose the included config, and if so, add under the include
		// variable.
		exposedInclude, err := includeMapAsCtyVal(extensions.TrackInclude.CurrentMap, terragruntOptions, extensions.DecodedDependencies, extensions.PartialParseDecodeList)
		if err != nil {
			return ctx, err
		}
		ctx.Variables["include"] = exposedInclude
	}
	return ctx, nil
}

// Return the OS platform
func getPlatform(trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
	return runtime.GOOS, nil
}

// Return the repository root as an absolute path
func getRepoRoot(trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
	return shell.GitTopLevelDir(terragruntOptions, terragruntOptions.WorkingDir)
}

// Return the path from the repository root
func getPathFromRepoRoot(trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
	repoAbsPath, err := shell.GitTopLevelDir(terragruntOptions, terragruntOptions.WorkingDir)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	repoRelPath, err := filepath.Rel(repoAbsPath, terragruntOptions.WorkingDir)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return filepath.ToSlash(repoRelPath), nil
}

// Return the path to the repository root
func getPathToRepoRoot(trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
	repoAbsPath, err := shell.GitTopLevelDir(terragruntOptions, terragruntOptions.WorkingDir)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	repoRootPathAbs, err := filepath.Rel(terragruntOptions.WorkingDir, repoAbsPath)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return filepath.ToSlash(strings.TrimSpace(repoRootPathAbs)), nil
}

// Return the directory where the Terragrunt configuration file lives
func getTerragruntDir(trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
	terragruntConfigFileAbsPath, err := filepath.Abs(terragruntOptions.TerragruntConfigPath)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return filepath.ToSlash(filepath.Dir(terragruntConfigFileAbsPath)), nil
}

// Return the directory where the original Terragrunt configuration file lives. This is primarily useful when one
// Terragrunt config is being read from another e.g., if /terraform-code/terragrunt.hcl
// calls read_terragrunt_config("/foo/bar.hcl"), and within bar.hcl, you call get_original_terragrunt_dir(), you'll
// get back /terraform-code.
func getOriginalTerragruntDir(trackIncude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
	terragruntConfigFileAbsPath, err := filepath.Abs(terragruntOptions.OriginalTerragruntConfigPath)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return filepath.ToSlash(filepath.Dir(terragruntConfigFileAbsPath)), nil
}

// Return the parent directory where the Terragrunt configuration file lives
func getParentTerragruntDir(params []string, trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
	parentPath, err := pathRelativeFromInclude(params, trackInclude, terragruntOptions)
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
	case noMatchedPats:
		envVariable.IsRequired = true
		envVariable.Name = parameters[0]
	case matchedPats:
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

// runCommandCache - cache of evaluated `run_cmd` invocations
// see: https://github.com/gruntwork-io/terragrunt/issues/1427
var runCommandCache = NewStringCache()

// runCommand is a helper function that runs a command and returns the stdout as the interporation
// for each `run_cmd` in locals section, function is called twice
// result
func runCommand(args []string, trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
	if len(args) == 0 {
		return "", errors.WithStackTrace(EmptyStringNotAllowed("parameter to the run_cmd function"))
	}

	suppressOutput := false
	currentPath := filepath.Dir(terragruntOptions.TerragruntConfigPath)
	cachePath := currentPath

	checkOptions := true
	for checkOptions && len(args) > 0 {
		switch args[0] {
		case "--terragrunt-quiet":
			suppressOutput = true
			args = append(args[:0], args[1:]...)
		case "--terragrunt-global-cache":
			cachePath = "_global_"
			args = append(args[:0], args[1:]...)
		default:
			checkOptions = false
		}
	}

	// To avoid re-run of the same run_cmd command, is used in memory cache for command results, with caching key path + arguments
	// see: https://github.com/gruntwork-io/terragrunt/issues/1427
	cacheKey := fmt.Sprintf("%v-%v", cachePath, args)
	cachedValue, foundInCache := runCommandCache.Get(cacheKey)
	if foundInCache {
		if suppressOutput {
			terragruntOptions.Logger.Debugf("run_cmd, cached output: [REDACTED]")
		} else {
			terragruntOptions.Logger.Debugf("run_cmd, cached output: [%s]", cachedValue)
		}
		return cachedValue, nil
	}

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

	// Persisting result in cache to avoid future re-evaluation
	// see: https://github.com/gruntwork-io/terragrunt/issues/1427
	runCommandCache.Put(cacheKey, value)
	return value, nil
}

func getEnvironmentVariable(parameters []string, trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
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
func findInParentFolders(
	params []string,
	trackInclude *TrackInclude,
	terragruntOptions *options.TerragruntOptions,
) (string, error) {
	numParams := len(params)

	var fileToFindParam string
	var fallbackParam string

	if numParams > 0 {
		fileToFindParam = params[0]
	}
	if numParams > 1 {
		fallbackParam = params[1]
	}
	if numParams > matchedPats {
		return "", errors.WithStackTrace(WrongNumberOfParams{Func: "find_in_parent_folders", Expected: "0, 1, or 2", Actual: numParams})
	}

	previousDir, err := filepath.Abs(filepath.Dir(terragruntOptions.TerragruntConfigPath))
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	previousDir = filepath.ToSlash(previousDir)

	fileToFindStr := DefaultTerragruntConfigPath
	if fileToFindParam != "" {
		fileToFindStr = fileToFindParam
	}

	// To avoid getting into an accidental infinite loop (e.g. do to cyclical symlinks), set a max on the number of
	// parent folders we'll check
	for i := 0; i < terragruntOptions.MaxFoldersToCheck; i++ {
		currentDir := filepath.ToSlash(filepath.Dir(previousDir))
		if currentDir == previousDir {
			if numParams == matchedPats {
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
// file. Name param is required and used to lookup the relevant import block when called in a child config with multiple
// import blocks.
func pathRelativeToInclude(params []string, trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
	if trackInclude == nil {
		return ".", nil
	}

	var included IncludeConfig
	switch {
	case trackInclude.Original != nil:
		included = *trackInclude.Original
	case len(trackInclude.CurrentList) > 0:
		// Called in child context, so we need to select the right include file.
		selected, err := getSelectedIncludeBlock(*trackInclude, params)
		if err != nil {
			return "", err
		}
		included = *selected
	default:
		return ".", nil
	}

	currentPath := filepath.Dir(terragruntOptions.TerragruntConfigPath)
	includePath := filepath.Dir(included.Path)

	if !filepath.IsAbs(includePath) {
		includePath = util.JoinPath(currentPath, includePath)
	}

	relativePath, err := util.GetPathRelativeTo(currentPath, includePath)
	return relativePath, err
}

// Return the relative path from the current Terragrunt configuration to the included Terragrunt configuration file
func pathRelativeFromInclude(params []string, trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
	if trackInclude == nil {
		return ".", nil
	}

	included, err := getSelectedIncludeBlock(*trackInclude, params)
	if err != nil {
		return "", err
	} else if included == nil {
		return ".", nil
	}

	includePath := filepath.Dir(included.Path)
	currentPath := filepath.Dir(terragruntOptions.TerragruntConfigPath)

	if !filepath.IsAbs(includePath) {
		includePath = util.JoinPath(currentPath, includePath)
	}

	return util.GetPathRelativeTo(includePath, currentPath)
}

// getTerraformCommand returns the current terraform command in execution
func getTerraformCommand(trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
	return terragruntOptions.TerraformCommand, nil
}

// getTerraformCliArgs returns cli args for terraform
func getTerraformCliArgs(trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) ([]string, error) {
	return terragruntOptions.TerraformCliArgs, nil
}

// getDefaultRetryableErrors returns default retryable errors
func getDefaultRetryableErrors(trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) ([]string, error) {
	return options.DEFAULT_RETRYABLE_ERRORS, nil
}

// Return the AWS account id associated to the current set of credentials
func getAWSAccountID(trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
	accountID, err := aws_helper.GetAWSAccountID(nil, terragruntOptions)
	if err == nil {
		return accountID, nil
	}
	return "", err
}

// Return the ARN of the AWS identity associated with the current set of credentials
func getAWSCallerIdentityARN(trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
	identityARN, err := aws_helper.GetAWSIdentityArn(nil, terragruntOptions)
	if err == nil {
		return identityARN, nil
	}
	return "", err
}

// Return the UserID of the AWS identity associated with the current set of credentials
func getAWSCallerIdentityUserID(trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
	userID, err := aws_helper.GetAWSUserID(nil, terragruntOptions)
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
	config, err := ParseConfigFile(targetConfig, targetOptions, nil, nil)
	if err != nil {
		return cty.NilVal, err
	}
	// We have to set the rendered outputs here because ParseConfigFile will not do so on the TerragruntConfig. The
	// outputs are stored in a special map that is used only for rendering and thus is not available when we try to
	// serialize the config for consumption.
	// NOTE: this will not call terragrunt output, since all the values are cached from the ParseConfigFile call
	// NOTE: we don't use range here because range will copy the slice, thereby undoing the set attribute.
	for i := 0; i < len(config.TerragruntDependencies); i++ {
		err := config.TerragruntDependencies[i].setRenderedOutputs(targetOptions)
		if err != nil {
			return cty.NilVal, errors.WithStackTrace(err)
		}
	}

	return TerragruntConfigAsCty(config)
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

			strArgs, err := ctySliceToStringSlice(args[:1])
			if err != nil {
				return cty.NilVal, err
			}

			var defaultVal *cty.Value = nil
			if numParams == matchedPats {
				defaultVal = &args[1]
			}

			targetConfigPath := strArgs[0]

			relativePath, err := readTerragruntConfig(targetConfigPath, defaultVal, terragruntOptions)
			return relativePath, err
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
	if len(matches) != matchedPats {
		return "", errors.WithStackTrace(ErrorParsingModulePath{ModuleSourceUrl: sourceUrl})
	}

	return matches[1], nil
}

// A cache of the results of a decrypt operation via sops. Each decryption
// operation can take several seconds, so this cache speeds up terragrunt executions
// where the same sops files are referenced multiple times.
//
// The cache keys are the canonical paths to the encrypted files, and the values are the
// plain-text result of the decrypt operation.
var sopsCache = NewStringCache()

// decrypts and returns sops encrypted utf-8 yaml or json data as a string
func sopsDecryptFile(params []string, trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
	numParams := len(params)

	var sourceFile string

	if numParams > 0 {
		sourceFile = params[0]
	}
	if numParams != 1 {
		return "", errors.WithStackTrace(WrongNumberOfParams{Func: "sops_decrypt_file", Expected: "1", Actual: numParams})
	}
	format, err := getSopsFileFormat(sourceFile)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}
	canonicalSourceFile, err := util.CanonicalPath(sourceFile, terragruntOptions.WorkingDir)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	if val, ok := sopsCache.Get(canonicalSourceFile); ok {
		return val, nil
	}

	rawData, err := decrypt.File(sourceFile, format)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	if utf8.Valid(rawData) {
		value := string(rawData)
		sopsCache.Put(canonicalSourceFile, value)
		return value, nil
	}

	return "", errors.WithStackTrace(InvalidSopsFormat{SourceFilePath: sourceFile})
}

// Mapping of SOPS format to string
var sopsFormatToString = map[formats.Format]string{
	formats.Binary: "binary",
	formats.Dotenv: "dotenv",
	formats.Ini:    "ini",
	formats.Json:   "json",
	formats.Yaml:   "yaml",
}

// getSopsFileFormat - Return file format for SOPS library
func getSopsFileFormat(sourceFile string) (string, error) {
	fileFormat := formats.FormatForPath(sourceFile)
	format, found := sopsFormatToString[fileFormat]
	if !found {
		return "", InvalidSopsFormat{SourceFilePath: sourceFile}
	}
	return format, nil
}

// Return the location of the Terraform files provided via --terragrunt-source
func getTerragruntSourceCliFlag(trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {
	return terragruntOptions.Source, nil
}

// Return the selected include block based on a label passed in as a function param. Note that the assumption is that:
//   - If the Original attribute is set, we are in the parent context so return that.
//   - If there are no include blocks, no param is required and nil is returned.
//   - If there is only one include block, no param is required and that is automatically returned.
//   - If there is more than one include block, 1 param is required to use as the label name to lookup the include block
//     to use.
func getSelectedIncludeBlock(trackInclude TrackInclude, params []string) (*IncludeConfig, error) {
	importMap := trackInclude.CurrentMap

	if trackInclude.Original != nil {
		return trackInclude.Original, nil
	}

	if len(importMap) == 0 {
		return nil, nil
	}

	if len(importMap) == 1 {
		for _, val := range importMap {
			return &val, nil
		}
	}

	numParams := len(params)
	if numParams != 1 {
		return nil, errors.WithStackTrace(WrongNumberOfParams{Func: "path_relative_from_include", Expected: "1", Actual: numParams})
	}

	importName := params[0]
	imported, hasKey := importMap[importName]
	if !hasKey {
		return nil, errors.WithStackTrace(InvalidIncludeKey{name: importName})
	}
	return &imported, nil
}

// startsWith Implementation of Terraform's startsWith function
func startsWith(args []string, trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if len(args) == 0 {
		return false, errors.WithStackTrace(EmptyStringNotAllowed("parameter to the startswith function"))
	}
	str := args[0]
	prefix := args[1]

	if strings.HasPrefix(str, prefix) {
		return true, nil
	}

	return false, nil
}

// endsWith Implementation of Terraform's endsWith function
func endsWith(args []string, trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if len(args) == 0 {
		return false, errors.WithStackTrace(EmptyStringNotAllowed("parameter to the endswith function"))
	}
	str := args[0]
	suffix := args[1]

	if strings.HasSuffix(str, suffix) {
		return true, nil
	}

	return false, nil
}

// timeCmp implements Terraform's `timecmp` function that compares two timestamps.
func timeCmp(args []string, trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (int64, error) {
	if len(args) != matchedPats {
		return 0, errors.WithStackTrace(fmt.Errorf("function can take only two parameters: timestamp_a and timestamp_b"))
	}

	tsA, err := util.ParseTimestamp(args[0])
	if err != nil {
		return 0, errors.WithStackTrace(fmt.Errorf("could not parse first parameter %q: %w", args[0], err))
	}
	tsB, err := util.ParseTimestamp(args[1])
	if err != nil {
		return 0, errors.WithStackTrace(fmt.Errorf("could not parse second parameter %q: %w", args[1], err))
	}

	switch {
	case tsA.Equal(tsB):
		return 0, nil
	case tsA.Before(tsB):
		return -1, nil
	default:
		// By elimination, tsA must be after tsB.
		return 1, nil
	}
}

// strContains Implementation of Terraform's strContains function
func strContains(args []string, trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if len(args) == 0 {
		return false, errors.WithStackTrace(EmptyStringNotAllowed("parameter to the strcontains function"))
	}
	str := args[0]
	substr := args[1]

	if strings.Contains(str, substr) {
		return true, nil
	}

	return false, nil
}

// readTFVarsFile reads a *.tfvars or *.tfvars.json file and returns the contents as a JSON encoded string
func readTFVarsFile(args []string, trackInclude *TrackInclude, terragruntOptions *options.TerragruntOptions) (string, error) {

	if len(args) != 1 {
		return "", errors.WithStackTrace(WrongNumberOfParams{Func: "read_tfvars_file", Expected: "1", Actual: len(args)})
	}

	varFile := args[0]
	varFile, err := util.CanonicalPath(varFile, terragruntOptions.WorkingDir)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	if !util.FileExists(varFile) {
		return "", errors.WithStackTrace(TFVarFileNotFoundError{File: varFile})
	}

	fileContents, err := os.ReadFile(varFile)
	if err != nil {
		return "", errors.WithStackTrace(fmt.Errorf("could not read file %q: %w", varFile, err))
	}

	if strings.HasSuffix(varFile, "json") {
		var variables map[string]interface{}
		// just want to be sure that the file is valid json
		if err := json.Unmarshal(fileContents, &variables); err != nil {
			return "", errors.WithStackTrace(fmt.Errorf("could not unmarshal json body of tfvar file: %w", err))
		}
		return string(fileContents), nil
	}

	var variables map[string]interface{}
	if err := ParseAndDecodeVarFile(string(fileContents), varFile, &variables); err != nil {
		return "", err
	}

	data, err := json.Marshal(variables)
	if err != nil {
		return "", errors.WithStackTrace(fmt.Errorf("could not marshal json body of tfvar file: %w", err))
	}

	return string(data), nil
}

// Custom error types

type TFVarFileNotFoundError struct {
	File  string
	Cause string
}

func (err TFVarFileNotFoundError) Error() string {
	return fmt.Sprintf("TFVarFileNotFound: Could not find a %s. Cause: %s.", err.File, err.Cause)
}

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

type InvalidSourceUrlWithMap struct {
	ModulePath      string
	ModuleSourceUrl string
}

func (err InvalidSourceUrlWithMap) Error() string {
	return fmt.Sprintf("The --terragrunt-source-map parameter was passed in, but the source URL in the module at '%s' is invalid: '%s'. Note that the module URL must have a double-slash to separate the repo URL from the path within the repo!", err.ModulePath, err.ModuleSourceUrl)
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

type InvalidIncludeKey struct {
	name string
}

func (err InvalidIncludeKey) Error() string {
	return fmt.Sprintf("There is no include block in the current config with the label '%s'", err.name)
}
