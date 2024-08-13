package config

import (
	"encoding/json"
	goErrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/go-multierror"

	"github.com/getsops/sops/v3/cmd/sops/formats"
	"github.com/getsops/sops/v3/decrypt"
	"github.com/hashicorp/go-getter"
	"github.com/hashicorp/hcl/v2"
	tflang "github.com/hashicorp/terraform/lang"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/gocty"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	noMatchedPats = 1
	matchedPats   = 2
)

const (
	FuncNameFindInParentFolders                     = "find_in_parent_folders"
	FuncNamePathRelativeToInclude                   = "path_relative_to_include"
	FuncNamePathRelativeFromInclude                 = "path_relative_from_include"
	FuncNameGetEnv                                  = "get_env"
	FuncNameRunCmd                                  = "run_cmd"
	FuncNameReadTerragruntConfig                    = "read_terragrunt_config"
	FuncNameGetPlatform                             = "get_platform"
	FuncNameGetRepoRoot                             = "get_repo_root"
	FuncNameGetPathFromRepoRoot                     = "get_path_from_repo_root"
	FuncNameGetPathToRepoRoot                       = "get_path_to_repo_root"
	FuncNameGetTerragruntDir                        = "get_terragrunt_dir"
	FuncNameGetOriginalTerragruntDir                = "get_original_terragrunt_dir"
	FuncNameGetTerraformCommand                     = "get_terraform_command"
	FuncNameGetTerraformCLIArgs                     = "get_terraform_cli_args"
	FuncNameGetParentTerragruntDir                  = "get_parent_terragrunt_dir"
	FuncNameGetAWSAccountID                         = "get_aws_account_id"
	FuncNameGetAWSCallerIdentityArn                 = "get_aws_caller_identity_arn"
	FuncNameGetAWSCallerIdentityUserID              = "get_aws_caller_identity_user_id"
	FuncNameGetTerraformCommandsThatNeedVars        = "get_terraform_commands_that_need_vars"
	FuncNameGetTerraformCommandsThatNeedLocking     = "get_terraform_commands_that_need_locking"
	FuncNameGetTerraformCommandsThatNeedInput       = "get_terraform_commands_that_need_input"
	FuncNameGetTerraformCommandsThatNeedParallelism = "get_terraform_commands_that_need_parallelism"
	FuncNameSopsDecryptFile                         = "sops_decrypt_file"
	FuncNameGetTerragruntSourceCLIFlag              = "get_terragrunt_source_cli_flag"
	FuncNameGetDefaultRetryableErrors               = "get_default_retryable_errors"
	FuncNameReadTfvarsFile                          = "read_tfvars_file"
	FuncNameGetWorkingDir                           = "get_working_dir"
	FuncNameStartsWith                              = "startswith"
	FuncNameEndsWith                                = "endswith"
	FuncNameStrContains                             = "strcontains"
	FuncNameTimeCmp                                 = "timecmp"

	sopsCacheName = "sopsCache"
)

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

// TrackInclude is used to differentiate between an included config in the current parsing ctx, and an included
// config that was passed through from a previous parsing ctx.
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

// Create an EvalContext for the HCL2 parser. We can define functions and variables in this ctx that the HCL2 parser
// will make available to the Terragrunt configuration during parsing.
func createTerragruntEvalContext(ctx *ParsingContext, configPath string) (*hcl.EvalContext, error) {
	tfscope := tflang.Scope{
		BaseDir: filepath.Dir(configPath),
	}

	terragruntFunctions := map[string]function.Function{
		FuncNameFindInParentFolders:                     wrapStringSliceToStringAsFuncImpl(ctx, FindInParentFolders),
		FuncNamePathRelativeToInclude:                   wrapStringSliceToStringAsFuncImpl(ctx, PathRelativeToInclude),
		FuncNamePathRelativeFromInclude:                 wrapStringSliceToStringAsFuncImpl(ctx, PathRelativeFromInclude),
		FuncNameGetEnv:                                  wrapStringSliceToStringAsFuncImpl(ctx, getEnvironmentVariable),
		FuncNameRunCmd:                                  wrapStringSliceToStringAsFuncImpl(ctx, RunCommand),
		FuncNameReadTerragruntConfig:                    readTerragruntConfigAsFuncImpl(ctx),
		FuncNameGetPlatform:                             wrapVoidToStringAsFuncImpl(ctx, getPlatform),
		FuncNameGetRepoRoot:                             wrapVoidToStringAsFuncImpl(ctx, getRepoRoot),
		FuncNameGetPathFromRepoRoot:                     wrapVoidToStringAsFuncImpl(ctx, getPathFromRepoRoot),
		FuncNameGetPathToRepoRoot:                       wrapVoidToStringAsFuncImpl(ctx, getPathToRepoRoot),
		FuncNameGetTerragruntDir:                        wrapVoidToStringAsFuncImpl(ctx, GetTerragruntDir),
		FuncNameGetOriginalTerragruntDir:                wrapVoidToStringAsFuncImpl(ctx, getOriginalTerragruntDir),
		FuncNameGetTerraformCommand:                     wrapVoidToStringAsFuncImpl(ctx, getTerraformCommand),
		FuncNameGetTerraformCLIArgs:                     wrapVoidToStringSliceAsFuncImpl(ctx, getTerraformCliArgs),
		FuncNameGetParentTerragruntDir:                  wrapStringSliceToStringAsFuncImpl(ctx, GetParentTerragruntDir),
		FuncNameGetAWSAccountID:                         wrapVoidToStringAsFuncImpl(ctx, getAWSAccountID),
		FuncNameGetAWSCallerIdentityArn:                 wrapVoidToStringAsFuncImpl(ctx, getAWSCallerIdentityARN),
		FuncNameGetAWSCallerIdentityUserID:              wrapVoidToStringAsFuncImpl(ctx, getAWSCallerIdentityUserID),
		FuncNameGetTerraformCommandsThatNeedVars:        wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_VARS),
		FuncNameGetTerraformCommandsThatNeedLocking:     wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_LOCKING),
		FuncNameGetTerraformCommandsThatNeedInput:       wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_INPUT),
		FuncNameGetTerraformCommandsThatNeedParallelism: wrapStaticValueToStringSliceAsFuncImpl(TERRAFORM_COMMANDS_NEED_PARALLELISM),
		FuncNameSopsDecryptFile:                         wrapStringSliceToStringAsFuncImpl(ctx, sopsDecryptFile),
		FuncNameGetTerragruntSourceCLIFlag:              wrapVoidToStringAsFuncImpl(ctx, getTerragruntSourceCliFlag),
		FuncNameGetDefaultRetryableErrors:               wrapVoidToStringSliceAsFuncImpl(ctx, getDefaultRetryableErrors),
		FuncNameReadTfvarsFile:                          wrapStringSliceToStringAsFuncImpl(ctx, readTFVarsFile),
		FuncNameGetWorkingDir:                           wrapVoidToStringAsFuncImpl(ctx, getWorkingDir),

		// Map with HCL functions introduced in Terraform after v0.15.3, since upgrade to a later version is not supported
		// https://github.com/gruntwork-io/terragrunt/blob/master/go.mod#L22
		FuncNameStartsWith:  wrapStringSliceToBoolAsFuncImpl(ctx, StartsWith),
		FuncNameEndsWith:    wrapStringSliceToBoolAsFuncImpl(ctx, EndsWith),
		FuncNameStrContains: wrapStringSliceToBoolAsFuncImpl(ctx, StrContains),
		FuncNameTimeCmp:     wrapStringSliceToNumberAsFuncImpl(ctx, TimeCmp),
	}

	functions := map[string]function.Function{}
	for k, v := range tfscope.Functions() {
		functions[k] = v
	}
	for k, v := range terragruntFunctions {
		functions[k] = v
	}
	for k, v := range ctx.PredefinedFunctions {
		functions[k] = v
	}

	evalCtx := &hcl.EvalContext{
		Functions: functions,
	}
	evalCtx.Variables = map[string]cty.Value{}
	if ctx.Locals != nil {
		evalCtx.Variables[MetadataLocal] = *ctx.Locals
	}

	if ctx.DecodedDependencies != nil {
		evalCtx.Variables[MetadataDependency] = *ctx.DecodedDependencies
	}
	if ctx.TrackInclude != nil && len(ctx.TrackInclude.CurrentList) > 0 {
		// For each include block, check if we want to expose the included config, and if so, add under the include
		// variable.
		exposedInclude, err := includeMapAsCtyVal(ctx)
		if err != nil {
			return evalCtx, err
		}
		evalCtx.Variables[MetadataInclude] = exposedInclude
	}
	return evalCtx, nil
}

// Return the OS platform
func getPlatform(ctx *ParsingContext) (string, error) {
	return runtime.GOOS, nil
}

// Return the repository root as an absolute path
func getRepoRoot(ctx *ParsingContext) (string, error) {
	return shell.GitTopLevelDir(ctx, ctx.TerragruntOptions, ctx.TerragruntOptions.WorkingDir)
}

// Return the path from the repository root
func getPathFromRepoRoot(ctx *ParsingContext) (string, error) {
	repoAbsPath, err := shell.GitTopLevelDir(ctx, ctx.TerragruntOptions, ctx.TerragruntOptions.WorkingDir)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	repoRelPath, err := filepath.Rel(repoAbsPath, ctx.TerragruntOptions.WorkingDir)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return filepath.ToSlash(repoRelPath), nil
}

// Return the path to the repository root
func getPathToRepoRoot(ctx *ParsingContext) (string, error) {
	repoAbsPath, err := shell.GitTopLevelDir(ctx, ctx.TerragruntOptions, ctx.TerragruntOptions.WorkingDir)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	repoRootPathAbs, err := filepath.Rel(ctx.TerragruntOptions.WorkingDir, repoAbsPath)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return filepath.ToSlash(strings.TrimSpace(repoRootPathAbs)), nil
}

// Return the directory where the Terragrunt configuration file lives
func GetTerragruntDir(ctx *ParsingContext) (string, error) {
	terragruntConfigFileAbsPath, err := filepath.Abs(ctx.TerragruntOptions.TerragruntConfigPath)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return filepath.ToSlash(filepath.Dir(terragruntConfigFileAbsPath)), nil
}

// Return the directory where the original Terragrunt configuration file lives. This is primarily useful when one
// Terragrunt config is being read from another e.g., if /terraform-code/terragrunt.hcl
// calls read_terragrunt_config("/foo/bar.hcl"), and within bar.hcl, you call get_original_terragrunt_dir(), you'll
// get back /terraform-code.
func getOriginalTerragruntDir(ctx *ParsingContext) (string, error) {
	terragruntConfigFileAbsPath, err := filepath.Abs(ctx.TerragruntOptions.OriginalTerragruntConfigPath)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return filepath.ToSlash(filepath.Dir(terragruntConfigFileAbsPath)), nil
}

// Return the parent directory where the Terragrunt configuration file lives
func GetParentTerragruntDir(ctx *ParsingContext, params []string) (string, error) {
	parentPath, err := PathRelativeFromInclude(ctx, params)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	currentPath := filepath.Dir(ctx.TerragruntOptions.TerragruntConfigPath)
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
		return envVariable, errors.WithStackTrace(InvalidGetEnvParamsError{ActualNumParams: len(parameters), Example: `getEnv("<NAME>", "[DEFAULT]")`})
	}

	if envVariable.Name == "" {
		return envVariable, errors.WithStackTrace(InvalidEnvParamNameError{EnvVarName: parameters[0]})
	}
	return envVariable, nil
}

// RunCommand is a helper function that runs a command and returns the stdout as the interporation
// for each `run_cmd` in locals section, function is called twice
// result
func RunCommand(ctx *ParsingContext, args []string) (string, error) {
	// runCommandCache - cache of evaluated `run_cmd` invocations
	// see: https://github.com/gruntwork-io/terragrunt/issues/1427
	runCommandCache := cache.ContextCache[string](ctx, RunCmdCacheContextKey)

	if len(args) == 0 {
		return "", errors.WithStackTrace(EmptyStringNotAllowedError("parameter to the run_cmd function"))
	}

	suppressOutput := false
	currentPath := filepath.Dir(ctx.TerragruntOptions.TerragruntConfigPath)
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
	cachedValue, foundInCache := runCommandCache.Get(ctx, cacheKey)
	if foundInCache {
		if suppressOutput {
			ctx.TerragruntOptions.Logger.Debugf("run_cmd, cached output: [REDACTED]")
		} else {
			ctx.TerragruntOptions.Logger.Debugf("run_cmd, cached output: [%s]", cachedValue)
		}
		return cachedValue, nil
	}

	cmdOutput, err := shell.RunShellCommandWithOutput(ctx, ctx.TerragruntOptions, currentPath, suppressOutput, false, args[0], args[1:]...)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	value := strings.TrimSuffix(cmdOutput.Stdout, "\n")

	if suppressOutput {
		ctx.TerragruntOptions.Logger.Debugf("run_cmd output: [REDACTED]")
	} else {
		ctx.TerragruntOptions.Logger.Debugf("run_cmd output: [%s]", value)
	}

	// Persisting result in cache to avoid future re-evaluation
	// see: https://github.com/gruntwork-io/terragrunt/issues/1427
	runCommandCache.Put(ctx, cacheKey, value)
	return value, nil
}

func getEnvironmentVariable(ctx *ParsingContext, parameters []string) (string, error) {
	parameterMap, err := parseGetEnvParameters(parameters)

	if err != nil {
		return "", errors.WithStackTrace(err)
	}
	envValue, exists := ctx.TerragruntOptions.Env[parameterMap.Name]

	if !exists {
		if parameterMap.IsRequired {
			return "", errors.WithStackTrace(EnvVarNotFoundError{EnvVar: parameterMap.Name})
		}
		envValue = parameterMap.DefaultValue
	}

	return envValue, nil
}

// Find a parent Terragrunt configuration file in the parent folders above the current Terragrunt configuration file
// and return its path
func FindInParentFolders(
	ctx *ParsingContext,
	params []string,
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
		return "", errors.WithStackTrace(WrongNumberOfParamsError{Func: "find_in_parent_folders", Expected: "0, 1, or 2", Actual: numParams})
	}

	previousDir, err := filepath.Abs(filepath.Dir(ctx.TerragruntOptions.TerragruntConfigPath))
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
	for i := 0; i < ctx.TerragruntOptions.MaxFoldersToCheck; i++ {
		currentDir := filepath.ToSlash(filepath.Dir(previousDir))
		if currentDir == previousDir {
			if numParams == matchedPats {
				return fallbackParam, nil
			}
			return "", errors.WithStackTrace(ParentFileNotFoundError{Path: ctx.TerragruntOptions.TerragruntConfigPath, File: fileToFindStr, Cause: "Traversed all the way to the root"})
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

	return "", errors.WithStackTrace(ParentFileNotFoundError{Path: ctx.TerragruntOptions.TerragruntConfigPath, File: fileToFindStr, Cause: fmt.Sprintf("Exceeded maximum folders to check (%d)", ctx.TerragruntOptions.MaxFoldersToCheck)})
}

// Return the relative path between the included Terragrunt configuration file and the current Terragrunt configuration
// file. Name param is required and used to lookup the relevant import block when called in a child config with multiple
// import blocks.
func PathRelativeToInclude(ctx *ParsingContext, params []string) (string, error) {
	if ctx.TrackInclude == nil {
		return ".", nil
	}

	var included IncludeConfig
	switch {
	case ctx.TrackInclude.Original != nil:
		included = *ctx.TrackInclude.Original
	case len(ctx.TrackInclude.CurrentList) > 0:
		// Called in child ctx, so we need to select the right include file.
		selected, err := getSelectedIncludeBlock(*ctx.TrackInclude, params)
		if err != nil {
			return "", err
		}
		included = *selected
	default:
		return ".", nil
	}

	currentPath := filepath.Dir(ctx.TerragruntOptions.TerragruntConfigPath)
	includePath := filepath.Dir(included.Path)

	if !filepath.IsAbs(includePath) {
		includePath = util.JoinPath(currentPath, includePath)
	}

	relativePath, err := util.GetPathRelativeTo(currentPath, includePath)
	return relativePath, err
}

// Return the relative path from the current Terragrunt configuration to the included Terragrunt configuration file
func PathRelativeFromInclude(ctx *ParsingContext, params []string) (string, error) {
	if ctx.TrackInclude == nil {
		return ".", nil
	}

	included, err := getSelectedIncludeBlock(*ctx.TrackInclude, params)
	if err != nil {
		return "", err
	} else if included == nil {
		return ".", nil
	}

	includePath := filepath.Dir(included.Path)
	currentPath := filepath.Dir(ctx.TerragruntOptions.TerragruntConfigPath)

	if !filepath.IsAbs(includePath) {
		includePath = util.JoinPath(currentPath, includePath)
	}

	return util.GetPathRelativeTo(includePath, currentPath)
}

// getTerraformCommand returns the current terraform command in execution
func getTerraformCommand(ctx *ParsingContext) (string, error) {
	return ctx.TerragruntOptions.TerraformCommand, nil
}

// getWorkingDir returns the current working dir
func getWorkingDir(ctx *ParsingContext) (string, error) {
	ctx.TerragruntOptions.Logger.Debugf("Start processing get_working_dir built-in function")
	defer ctx.TerragruntOptions.Logger.Debugf("Complete processing get_working_dir built-in function")

	// Initialize evaluation ctx extensions from base blocks.
	ctx.PredefinedFunctions = map[string]function.Function{
		FuncNameGetWorkingDir: wrapVoidToEmptyStringAsFuncImpl(),
	}

	terragruntConfig, err := ParseConfigFile(ctx, ctx.TerragruntOptions.TerragruntConfigPath, nil)
	if err != nil {
		return "", err
	}

	sourceUrl, err := GetTerraformSourceUrl(ctx.TerragruntOptions, terragruntConfig)
	if err != nil {
		return "", err
	}

	if sourceUrl == "" {
		return ctx.TerragruntOptions.WorkingDir, nil
	}

	source, err := terraform.NewSource(sourceUrl, ctx.TerragruntOptions.DownloadDir, ctx.TerragruntOptions.WorkingDir, ctx.TerragruntOptions.Logger)
	if err != nil {
		return "", err
	}

	return source.WorkingDir, nil
}

// getTerraformCliArgs returns cli args for terraform
func getTerraformCliArgs(ctx *ParsingContext) ([]string, error) {
	return ctx.TerragruntOptions.TerraformCliArgs, nil
}

// getDefaultRetryableErrors returns default retryable errors
func getDefaultRetryableErrors(ctx *ParsingContext) ([]string, error) {
	return options.DEFAULT_RETRYABLE_ERRORS, nil
}

// Return the AWS account id associated to the current set of credentials
func getAWSAccountID(ctx *ParsingContext) (string, error) {
	accountID, err := aws_helper.GetAWSAccountID(nil, ctx.TerragruntOptions)
	if err == nil {
		return accountID, nil
	}
	return "", err
}

// Return the ARN of the AWS identity associated with the current set of credentials
func getAWSCallerIdentityARN(ctx *ParsingContext) (string, error) {
	identityARN, err := aws_helper.GetAWSIdentityArn(nil, ctx.TerragruntOptions)
	if err == nil {
		return identityARN, nil
	}
	return "", err
}

// Return the UserID of the AWS identity associated with the current set of credentials
func getAWSCallerIdentityUserID(ctx *ParsingContext) (string, error) {
	userID, err := aws_helper.GetAWSUserID(nil, ctx.TerragruntOptions)
	if err == nil {
		return userID, nil
	}
	return "", err
}

// Parse the terragrunt config and return a representation that can be used as a reference. If given a default value,
// this will return the default if the terragrunt config file does not exist.
func ParseTerragruntConfig(ctx *ParsingContext, configPath string, defaultVal *cty.Value) (cty.Value, error) {
	// target config check: make sure the target config exists. If the file does not exist, and there is no default val,
	// return an error. If the file does not exist but there is a default val, return the default val. Otherwise,
	// proceed to parse the file as a terragrunt config file.
	targetConfig := getCleanedTargetConfigPath(configPath, ctx.TerragruntOptions.TerragruntConfigPath)
	targetConfigFileExists := util.FileExists(targetConfig)
	if !targetConfigFileExists && defaultVal == nil {
		return cty.NilVal, errors.WithStackTrace(TerragruntConfigNotFoundError{Path: targetConfig})
	} else if !targetConfigFileExists {
		return *defaultVal, nil
	}

	// We update the ctx of terragruntOptions to the config being read in.
	ctx = ctx.WithTerragruntOptions(ctx.TerragruntOptions.Clone(targetConfig))
	config, err := ParseConfigFile(ctx, targetConfig, nil)
	if err != nil {
		return cty.NilVal, err
	}

	// We have to set the rendered outputs here because ParseConfigFile will not do so on the TerragruntConfig. The
	// outputs are stored in a special map that is used only for rendering and thus is not available when we try to
	// serialize the config for consumption.
	// NOTE: this will not call terragrunt output, since all the values are cached from the ParseConfigFile call
	// NOTE: we don't use range here because range will copy the slice, thereby undoing the set attribute.
	for i := 0; i < len(config.TerragruntDependencies); i++ {
		err := config.TerragruntDependencies[i].setRenderedOutputs(ctx)
		if err != nil {
			return cty.NilVal, errors.WithStackTrace(err)
		}
	}

	return TerragruntConfigAsCty(config)
}

// Create a cty Function that can be used to for calling read_terragrunt_config.
func readTerragruntConfigAsFuncImpl(ctx *ParsingContext) function.Function {
	return function.New(&function.Spec{
		// Takes one required string param
		Params: []function.Parameter{{Type: cty.String}},
		// And optional param that takes anything
		VarParam: &function.Parameter{Type: cty.DynamicPseudoType},
		// We don't know the return type until we parse the terragrunt config, so we use a dynamic type
		Type: function.StaticReturnType(cty.DynamicPseudoType),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			numParams := len(args)
			if numParams == 0 || numParams > 2 {
				return cty.NilVal, errors.WithStackTrace(WrongNumberOfParamsError{Func: "read_terragrunt_config", Expected: "1 or 2", Actual: numParams})
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
			return ParseTerragruntConfig(ctx, targetConfigPath, defaultVal)
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
		return "", errors.WithStackTrace(InvalidSourceUrlError{ModulePath: modulePath, ModuleSourceUrl: *moduleTerragruntConfig.Terraform.Source, TerragruntSource: sourcePath})
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
		return "", errors.WithStackTrace(ParsingModulePathError{ModuleSourceUrl: sourceUrl})
	}

	return matches[1], nil
}

// A cache of the results of a decrypt operation via sops. Each decryption
// operation can take several seconds, so this cache speeds up terragrunt executions
// where the same sops files are referenced multiple times.
//
// The cache keys are the canonical paths to the encrypted files, and the values are the
// plain-text result of the decrypt operation.
var sopsCache = cache.NewCache[string](sopsCacheName)

// decrypts and returns sops encrypted utf-8 yaml or json data as a string
func sopsDecryptFile(ctx *ParsingContext, params []string) (string, error) {
	numParams := len(params)

	var sourceFile string

	if numParams > 0 {
		sourceFile = params[0]
	}
	if numParams != 1 {
		return "", errors.WithStackTrace(WrongNumberOfParamsError{Func: "sops_decrypt_file", Expected: "1", Actual: numParams})
	}
	format, err := getSopsFileFormat(sourceFile)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}
	canonicalSourceFile, err := util.CanonicalPath(sourceFile, filepath.Dir(ctx.TerragruntOptions.TerragruntConfigPath))
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	if val, ok := sopsCache.Get(ctx, canonicalSourceFile); ok {
		return val, nil
	}

	rawData, err := decrypt.File(canonicalSourceFile, format)
	if err != nil {
		return "", errors.WithStackTrace(extractSopsErrors(err))
	}

	if utf8.Valid(rawData) {
		value := string(rawData)
		sopsCache.Put(ctx, canonicalSourceFile, value)
		return value, nil
	}

	return "", errors.WithStackTrace(InvalidSopsFormatError{SourceFilePath: sourceFile})
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
		return "", InvalidSopsFormatError{SourceFilePath: sourceFile}
	}
	return format, nil
}

// Return the location of the Terraform files provided via --terragrunt-source
func getTerragruntSourceCliFlag(ctx *ParsingContext) (string, error) {
	return ctx.TerragruntOptions.Source, nil
}

// Return the selected include block based on a label passed in as a function param. Note that the assumption is that:
//   - If the Original attribute is set, we are in the parent ctx so return that.
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
		return nil, errors.WithStackTrace(WrongNumberOfParamsError{Func: "path_relative_from_include", Expected: "1", Actual: numParams})
	}

	importName := params[0]
	imported, hasKey := importMap[importName]
	if !hasKey {
		return nil, errors.WithStackTrace(InvalidIncludeKeyError{name: importName})
	}
	return &imported, nil
}

// StartsWith Implementation of Terraform's StartsWith function
func StartsWith(ctx *ParsingContext, args []string) (bool, error) {
	if len(args) == 0 {
		return false, errors.WithStackTrace(EmptyStringNotAllowedError("parameter to the startswith function"))
	}
	str := args[0]
	prefix := args[1]

	if strings.HasPrefix(str, prefix) {
		return true, nil
	}

	return false, nil
}

// EndsWith Implementation of Terraform's EndsWith function
func EndsWith(ctx *ParsingContext, args []string) (bool, error) {
	if len(args) == 0 {
		return false, errors.WithStackTrace(EmptyStringNotAllowedError("parameter to the endswith function"))
	}
	str := args[0]
	suffix := args[1]

	if strings.HasSuffix(str, suffix) {
		return true, nil
	}

	return false, nil
}

// TimeCmp implements Terraform's `timecmp` function that compares two timestamps.
func TimeCmp(ctx *ParsingContext, args []string) (int64, error) {
	if len(args) != matchedPats {
		return 0, errors.WithStackTrace(goErrors.New("function can take only two parameters: timestamp_a and timestamp_b"))
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

// StrContains Implementation of Terraform's StrContains function
func StrContains(ctx *ParsingContext, args []string) (bool, error) {
	if len(args) == 0 {
		return false, errors.WithStackTrace(EmptyStringNotAllowedError("parameter to the strcontains function"))
	}
	str := args[0]
	substr := args[1]

	if strings.Contains(str, substr) {
		return true, nil
	}

	return false, nil
}

// readTFVarsFile reads a *.tfvars or *.tfvars.json file and returns the contents as a JSON encoded string
func readTFVarsFile(ctx *ParsingContext, args []string) (string, error) {

	if len(args) != 1 {
		return "", errors.WithStackTrace(WrongNumberOfParamsError{Func: "read_tfvars_file", Expected: "1", Actual: len(args)})
	}

	varFile := args[0]
	varFile, err := util.CanonicalPath(varFile, ctx.TerragruntOptions.WorkingDir)
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
	if err := ParseAndDecodeVarFile(varFile, fileContents, &variables); err != nil {
		return "", err
	}

	data, err := json.Marshal(variables)
	if err != nil {
		return "", errors.WithStackTrace(fmt.Errorf("could not marshal json body of tfvar file: %w", err))
	}

	return string(data), nil
}

// ParseAndDecodeVarFile uses the HCL2 file to parse the given varfile string into an HCL file body, and then decode it
// into the provided output.
func ParseAndDecodeVarFile(varFile string, fileContents []byte, out interface{}) error {
	parser := hclparse.NewParser()
	file, err := parser.ParseFromBytes(fileContents, varFile)
	if err != nil {
		return err
	}

	attrs, err := file.JustAttributes()
	if err != nil {
		return err
	}

	valMap := map[string]cty.Value{}
	for _, attr := range attrs {
		val, err := attr.Value(nil) // nil because no function calls or variable references are allowed here
		if err != nil {
			return err
		}
		valMap[attr.Name] = val
	}

	ctyVal, err := convertValuesMapToCtyVal(valMap)
	if err != nil {
		return err
	}

	if ctyVal.IsNull() {
		// If the file is empty, doesn't make sense to do conversion
		return nil
	}

	typedOut, hasType := out.(*map[string]interface{})
	if hasType {
		genericMap, err := ParseCtyValueToMap(ctyVal)
		if err != nil {
			return err
		}
		*typedOut = genericMap
		return nil
	}
	return gocty.FromCtyValue(ctyVal, out)
}

// extractSopsErrors extracts the original errors from the sops library and returns them as a multierror.Error
func extractSopsErrors(err error) *multierror.Error {
	var errs = &multierror.Error{}

	// workaround to extract original errors from sops library
	// using reflection extract GroupResults from getDataKeyError
	// may not be compatible with future versions
	errValue := reflect.ValueOf(err)
	if errValue.Kind() == reflect.Ptr {
		errValue = errValue.Elem()
	}
	if errValue.Type().Name() == "getDataKeyError" {
		groupResultsField := errValue.FieldByName("GroupResults")
		if groupResultsField.IsValid() && groupResultsField.Kind() == reflect.Slice {
			for i := 0; i < groupResultsField.Len(); i++ {
				groupErr := groupResultsField.Index(i)
				if groupErr.CanInterface() {
					errs = multierror.Append(errs, groupErr.Interface().(error))
				}
			}
		}
	}

	// append the original error if no group results were found
	if errs.Len() == 0 {
		errs = multierror.Append(errs, err)
	}
	return errs
}
