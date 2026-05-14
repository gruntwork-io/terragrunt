package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/getsops/sops/v3/cmd/sops/formats"
	"github.com/getsops/sops/v3/decrypt"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	tflang "github.com/hashicorp/terraform/lang"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/gocty"

	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/glob"
	"github.com/gruntwork-io/terragrunt/internal/locks"
	"github.com/gruntwork-io/terragrunt/internal/retry"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	noMatchedPats = 1
	matchedPats   = 2

	// stringCompParams is the exact number of arguments expected by the
	// startswith, endswith, and strcontains helpers (haystack + needle).
	stringCompParams = 2
)

// RunCmdCacheEntry stores run_cmd results including output for replay.
// This allows the output to be replayed on cache hits, which is necessary
// when the command was first executed during discovery phase (with io.Discard writers)
// but needs to show output during the execution phase (with real writers).
type RunCmdCacheEntry struct {
	// Stdout is the raw stdout of the command.
	Stdout string
	// Stderr is the raw stderr of the command.
	Stderr string
	// replayOnce ensures output is replayed exactly once to a real (non-Discard) writer.
	replayOnce sync.Once
}

// Value returns the whitespace-trimmed stdout, which is the return value of run_cmd.
func (e *RunCmdCacheEntry) Value() string {
	return strings.TrimSuffix(e.Stdout, "\n")
}

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
	FuncNameGetAWSAccountAlias                      = "get_aws_account_alias"
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
	FuncNameMarkAsRead                              = "mark_as_read"
	FuncNameMarkGlobAsRead                          = "mark_glob_as_read"
	FuncNameConstraintCheck                         = "constraint_check"
)

// TerraformCommandsNeedLocking is a list of terraform commands that accept -lock-timeout
var TerraformCommandsNeedLocking = []string{
	"apply",
	"destroy",
	"import",
	"plan",
	"refresh",
	"taint",
	"untaint",
}

// TerraformCommandsNeedVars is a list of terraform commands that accept -var or -var-file
var TerraformCommandsNeedVars = []string{
	"apply",
	"console",
	"destroy",
	"import",
	"plan",
	"push",
	"refresh",
}

// TerraformCommandsNeedInput is list of terraform commands that accept -input=
var TerraformCommandsNeedInput = []string{
	"apply",
	"import",
	"init",
	"plan",
	"refresh",
}

// TerraformCommandsNeedParallelism is a list of terraform commands that accept -parallelism=
var TerraformCommandsNeedParallelism = []string{
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
	// CurrentMap is the map version of CurrentList that maps the block labels to the included config.
	CurrentMap map[string]IncludeConfig
	// Original is used to track the original included config, and is used for resolving the include related
	// functions.
	Original *IncludeConfig
	// CurrentList is used to track the list of configs that should be imported and merged before the final
	// TerragruntConfig is returned. This preserves the order of the blocks as they appear in the config, so that we can
	// merge the included config in the right order.
	CurrentList IncludeConfigs
}

// Create an EvalContext for the HCL2 parser. We can define functions and variables in this ctx that the HCL2 parser
// will make available to the Terragrunt configuration during parsing.
func createTerragruntEvalContext(ctx context.Context, pctx *ParsingContext, l log.Logger, configPath string) (*hcl.EvalContext, error) {
	tfscope := tflang.Scope{
		BaseDir: filepath.Dir(configPath),
	}

	terragruntFunctions := map[string]function.Function{
		FuncNameFindInParentFolders:                     wrapStringSliceToStringAsFuncImpl(ctx, pctx, l, FindInParentFolders),
		FuncNamePathRelativeToInclude:                   wrapStringSliceToStringAsFuncImpl(ctx, pctx, l, PathRelativeToInclude),
		FuncNamePathRelativeFromInclude:                 wrapStringSliceToStringAsFuncImpl(ctx, pctx, l, PathRelativeFromInclude),
		FuncNameGetEnv:                                  wrapStringSliceToStringAsFuncImpl(ctx, pctx, l, getEnvironmentVariable),
		FuncNameRunCmd:                                  wrapStringSliceToStringAsFuncImpl(ctx, pctx, l, RunCommand),
		FuncNameReadTerragruntConfig:                    readTerragruntConfigAsFuncImpl(ctx, pctx, l),
		FuncNameGetPlatform:                             wrapVoidToStringAsFuncImpl(ctx, pctx, l, getPlatform),
		FuncNameGetRepoRoot:                             wrapVoidToStringAsFuncImpl(ctx, pctx, l, getRepoRoot),
		FuncNameGetPathFromRepoRoot:                     wrapVoidToStringAsFuncImpl(ctx, pctx, l, getPathFromRepoRoot),
		FuncNameGetPathToRepoRoot:                       wrapVoidToStringAsFuncImpl(ctx, pctx, l, getPathToRepoRoot),
		FuncNameGetTerragruntDir:                        wrapVoidToStringAsFuncImpl(ctx, pctx, l, GetTerragruntDir),
		FuncNameGetOriginalTerragruntDir:                wrapVoidToStringAsFuncImpl(ctx, pctx, l, getOriginalTerragruntDir),
		FuncNameGetTerraformCommand:                     wrapVoidToStringAsFuncImpl(ctx, pctx, l, getTerraformCommand),
		FuncNameGetTerraformCLIArgs:                     wrapVoidToStringSliceAsFuncImpl(ctx, pctx, l, getTerraformCliArgs),
		FuncNameGetParentTerragruntDir:                  wrapStringSliceToStringAsFuncImpl(ctx, pctx, l, GetParentTerragruntDir),
		FuncNameGetAWSAccountAlias:                      wrapVoidToStringAsFuncImpl(ctx, pctx, l, getAWSAccountAlias),
		FuncNameGetAWSAccountID:                         wrapVoidToStringAsFuncImpl(ctx, pctx, l, getAWSAccountID),
		FuncNameGetAWSCallerIdentityArn:                 wrapVoidToStringAsFuncImpl(ctx, pctx, l, getAWSCallerIdentityARN),
		FuncNameGetAWSCallerIdentityUserID:              wrapVoidToStringAsFuncImpl(ctx, pctx, l, getAWSCallerIdentityUserID),
		FuncNameGetTerraformCommandsThatNeedVars:        wrapStaticValueToStringSliceAsFuncImpl(TerraformCommandsNeedVars),
		FuncNameGetTerraformCommandsThatNeedLocking:     wrapStaticValueToStringSliceAsFuncImpl(TerraformCommandsNeedLocking),
		FuncNameGetTerraformCommandsThatNeedInput:       wrapStaticValueToStringSliceAsFuncImpl(TerraformCommandsNeedInput),
		FuncNameGetTerraformCommandsThatNeedParallelism: wrapStaticValueToStringSliceAsFuncImpl(TerraformCommandsNeedParallelism),
		FuncNameSopsDecryptFile:                         wrapStringSliceToStringAsFuncImpl(ctx, pctx, l, sopsDecryptFile),
		FuncNameGetTerragruntSourceCLIFlag:              wrapVoidToStringAsFuncImpl(ctx, pctx, l, getTerragruntSourceCliFlag),
		FuncNameGetDefaultRetryableErrors:               wrapVoidToStringSliceAsFuncImpl(ctx, pctx, l, getDefaultRetryableErrors),
		FuncNameReadTfvarsFile:                          wrapStringSliceToStringAsFuncImpl(ctx, pctx, l, readTFVarsFile),
		FuncNameGetWorkingDir:                           wrapVoidToStringAsFuncImpl(ctx, pctx, l, getWorkingDir),
		FuncNameMarkAsRead:                              wrapStringSliceToStringAsFuncImpl(ctx, pctx, l, markAsRead),
		FuncNameMarkGlobAsRead:                          wrapStringSliceToStringSliceAsFuncImpl(ctx, pctx, l, markGlobAsRead),
		FuncNameConstraintCheck:                         wrapStringSliceToBoolAsFuncImpl(ctx, pctx, ConstraintCheck),

		// Map with HCL functions introduced in Terraform after v0.15.3, since upgrade to a later version is not supported
		// https://github.com/gruntwork-io/terragrunt/blob/master/go.mod#L22
		FuncNameStartsWith:  wrapStringSliceToBoolAsFuncImpl(ctx, pctx, StartsWith),
		FuncNameEndsWith:    wrapStringSliceToBoolAsFuncImpl(ctx, pctx, EndsWith),
		FuncNameStrContains: wrapStringSliceToBoolAsFuncImpl(ctx, pctx, StrContains),
		FuncNameTimeCmp:     wrapStringSliceToNumberAsFuncImpl(ctx, pctx, l, TimeCmp),
	}

	functions := map[string]function.Function{}

	maps.Copy(functions, tfscope.Functions())
	maps.Copy(functions, terragruntFunctions)
	maps.Copy(functions, pctx.PredefinedFunctions)

	evalCtx := &hcl.EvalContext{
		Functions: functions,
	}

	evalCtx.Variables = map[string]cty.Value{}
	if pctx.Locals != nil {
		evalCtx.Variables[MetadataLocal] = *pctx.Locals
	}

	if pctx.Features != nil {
		evalCtx.Variables[MetadataFeatureFlag] = *pctx.Features
	}

	if pctx.Values != nil {
		evalCtx.Variables[MetadataValues] = *pctx.Values
	}

	if pctx.DecodedDependencies != nil {
		evalCtx.Variables[MetadataDependency] = *pctx.DecodedDependencies
	}

	if pctx.TrackInclude != nil && len(pctx.TrackInclude.CurrentList) > 0 {
		// For each include block, check if we want to expose the included config, and if so, add under the include
		// variable.
		exposedInclude, err := includeMapAsCtyVal(ctx, pctx, l)
		if err != nil && len(pctx.PartialParseDecodeList) == 0 {
			return nil, fmt.Errorf("could not resolve exposed includes for eval context in %s: %w", configPath, err)
		}

		if err != nil {
			// Include resolution can fail during partial parsing of configs in dependency chains,
			// e.g. when an included config has a dependency block whose outputs aren't yet available.
			// This is expected and non-fatal — locals referencing the include will be left unevaluated,
			// and the system will fall back to full parsing when needed.
			l.Debugf("Could not resolve exposed includes for eval context in %s (partial parse): %v", configPath, err)
		}

		if err == nil {
			evalCtx.Variables[MetadataInclude] = exposedInclude
		}
	}

	return evalCtx, nil
}

// Return the OS platform
func getPlatform(ctx context.Context, pctx *ParsingContext, l log.Logger) (string, error) {
	return runtime.GOOS, nil
}

// Return the repository root as an absolute path
func getRepoRoot(ctx context.Context, pctx *ParsingContext, l log.Logger) (string, error) {
	return shell.GitTopLevelDir(ctx, l, pctx.Env, pctx.WorkingDir)
}

// Return the path from the repository root
func getPathFromRepoRoot(ctx context.Context, pctx *ParsingContext, l log.Logger) (string, error) {
	repoAbsPath, err := shell.GitTopLevelDir(ctx, l, pctx.Env, pctx.WorkingDir)
	if err != nil {
		return "", fmt.Errorf("getting git top level dir: %w", err)
	}

	repoRelPath, err := filepath.Rel(repoAbsPath, pctx.WorkingDir)
	if err != nil {
		return "", fmt.Errorf("computing path relative to repo root: %w", err)
	}

	return repoRelPath, nil
}

// Return the path to the repository root
func getPathToRepoRoot(ctx context.Context, pctx *ParsingContext, l log.Logger) (string, error) {
	repoAbsPath, err := shell.GitTopLevelDir(ctx, l, pctx.Env, pctx.WorkingDir)
	if err != nil {
		return "", fmt.Errorf("getting git top level dir: %w", err)
	}

	repoRootPathAbs, err := filepath.Rel(pctx.WorkingDir, repoAbsPath)
	if err != nil {
		return "", fmt.Errorf("computing path to repo root: %w", err)
	}

	return strings.TrimSpace(repoRootPathAbs), nil
}

// GetTerragruntDir returns the directory where the Terragrunt configuration file lives.
func GetTerragruntDir(ctx context.Context, pctx *ParsingContext, l log.Logger) (string, error) {
	return filepath.Dir(pctx.TerragruntConfigPath), nil
}

// Return the directory where the original Terragrunt configuration file lives. This is primarily useful when one
// Terragrunt config is being read from another e.g., if /terraform-code/terragrunt.hcl
// calls read_terragrunt_config("/foo/bar.hcl"), and within bar.hcl, you call get_original_terragrunt_dir(), you'll
// get back /terraform-code.
func getOriginalTerragruntDir(ctx context.Context, pctx *ParsingContext, l log.Logger) (string, error) {
	return filepath.Dir(pctx.OriginalTerragruntConfigPath), nil
}

// GetParentTerragruntDir returns the parent directory where the Terragrunt configuration file lives.
func GetParentTerragruntDir(ctx context.Context, pctx *ParsingContext, l log.Logger, params []string) (string, error) {
	parentPath, err := PathRelativeFromInclude(ctx, pctx, l, params)
	if err != nil {
		return "", fmt.Errorf("getting path relative from include: %w", err)
	}

	currentPath := filepath.Dir(pctx.TerragruntConfigPath)

	return filepath.Clean(filepath.Join(currentPath, parentPath)), nil
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
		return envVariable, errors.New(InvalidGetEnvParamsError{ActualNumParams: len(parameters), Example: `getEnv("<NAME>", "[DEFAULT]")`})
	}

	if envVariable.Name == "" {
		return envVariable, errors.New(InvalidEnvParamNameError{EnvName: parameters[0]})
	}

	return envVariable, nil
}

// RunCommand is a helper function that runs a command and returns the stdout as the interpolation
// for each `run_cmd` in locals section, function is called twice
// result
func RunCommand(ctx context.Context, pctx *ParsingContext, l log.Logger, args []string) (string, error) {
	return runCommandImpl(ctx, pctx, l, args)
}

// runCommandImpl contains the actual implementation of RunCommand
func runCommandImpl(ctx context.Context, pctx *ParsingContext, l log.Logger, args []string) (string, error) {
	// runCommandCache - cache of evaluated `run_cmd` invocations
	// see: https://github.com/gruntwork-io/terragrunt/issues/1427
	runCommandCache := cache.ContextCache[*RunCmdCacheEntry](ctx, RunCmdCacheContextKey)

	if len(args) == 0 {
		return "", errors.New(EmptyStringNotAllowedError("parameter to the run_cmd function"))
	}

	suppressOutput := false
	disableCache := false
	useGlobalCache := false
	currentPath := filepath.Dir(pctx.TerragruntConfigPath)
	cachePath := currentPath

	checkOptions := true
	for checkOptions && len(args) > 0 {
		switch args[0] {
		case "--terragrunt-quiet":
			suppressOutput = true

			args = slices.Delete(args, 0, 1)
		case "--terragrunt-global-cache":
			if disableCache {
				return "", errors.New(ConflictingRunCmdCacheOptionsError{})
			}

			useGlobalCache = true
			cachePath = "_global_"

			args = slices.Delete(args, 0, 1)
		case "--terragrunt-no-cache":
			if useGlobalCache {
				return "", errors.New(ConflictingRunCmdCacheOptionsError{})
			}

			disableCache = true

			args = slices.Delete(args, 0, 1)
		default:
			checkOptions = false
		}
	}

	// To avoid re-run of the same run_cmd command, is used in memory cache for command results, with caching key path + arguments
	// see: https://github.com/gruntwork-io/terragrunt/issues/1427
	cacheKey := fmt.Sprintf("%v-%v", cachePath, args)

	// Skip cache lookup if --terragrunt-no-cache is set
	if !disableCache {
		cachedEntry, foundInCache := runCommandCache.Get(ctx, cacheKey)
		if foundInCache {
			// Replay stdout/stderr to current writers once when we have a real (non-Discard) writer.
			// This is needed because the command may have first run during discovery phase
			// with io.Discard writers, so we need to replay the output during execution phase.
			// We only call Do() when we have a real writer, so it won't fire during discovery.
			if pctx.Writers.Writer != io.Discard {
				cachedEntry.replayOnce.Do(func() {
					if !suppressOutput && cachedEntry.Stdout != "" {
						_, _ = pctx.Writers.Writer.Write([]byte(cachedEntry.Stdout))
					}

					if cachedEntry.Stderr != "" {
						_, _ = pctx.Writers.ErrWriter.Write([]byte(cachedEntry.Stderr))
					}
				})
			}

			if suppressOutput {
				l.Debugf("run_cmd, cached output: [REDACTED]")
			} else {
				l.Debugf("run_cmd, cached output: [%s]", cachedEntry.Value())
			}

			return cachedEntry.Value(), nil
		}
	}

	cmdOutput, err := shell.RunCommandWithOutput(
		ctx,
		l,
		vexec.NewOSExec(),
		shellRunOptsFromPctx(pctx),
		currentPath,
		true,
		false,
		args[0],
		args[1:]...,
	)
	if err != nil {
		return "", fmt.Errorf("running command: %w", err)
	}

	value := strings.TrimSuffix(cmdOutput.Stdout.String(), "\n")

	if suppressOutput {
		l.Debugf("run_cmd output: [REDACTED]")
	} else {
		l.Debugf("run_cmd output: [%s]", value)
	}

	entry := &RunCmdCacheEntry{
		Stdout: cmdOutput.Stdout.String(),
		Stderr: cmdOutput.Stderr.String(),
	}

	if pctx.Writers.Writer != io.Discard {
		entry.replayOnce.Do(func() {
			if !suppressOutput && entry.Stdout != "" {
				_, _ = pctx.Writers.Writer.Write([]byte(entry.Stdout))
			}

			if entry.Stderr != "" {
				_, _ = pctx.Writers.ErrWriter.Write([]byte(entry.Stderr))
			}
		})
	}

	if !disableCache {
		runCommandCache.Put(ctx, cacheKey, entry)
	}

	return value, nil
}

func getEnvironmentVariable(ctx context.Context, pctx *ParsingContext, l log.Logger, parameters []string) (string, error) {
	parameterMap, err := parseGetEnvParameters(parameters)
	if err != nil {
		return "", fmt.Errorf("parsing get_env parameters: %w", err)
	}

	envValue, exists := pctx.Env[parameterMap.Name]
	if !exists {
		if parameterMap.IsRequired {
			return "", errors.New(EnvVarNotFoundError{EnvVar: parameterMap.Name})
		}

		envValue = parameterMap.DefaultValue
	}

	return envValue, nil
}

// FindInParentFolders fings a parent Terragrunt configuration file in the parent
// folders above the current Terragrunt configuration file and return its path.
func FindInParentFolders(
	ctx context.Context,
	pctx *ParsingContext,
	l log.Logger,
	params []string,
) (string, error) {
	return findInParentFoldersImpl(ctx, pctx, l, params)
}

// findInParentFoldersImpl contains the actual implementation of FindInParentFolders
func findInParentFoldersImpl(ctx context.Context, pctx *ParsingContext, l log.Logger, params []string) (string, error) {
	numParams := len(params)

	var (
		fileToFindParam string
		fallbackParam   string
	)

	if numParams > 0 {
		fileToFindParam = params[0]
	}

	if numParams > 1 {
		fallbackParam = params[1]
	}

	if numParams > matchedPats {
		return "", errors.New(WrongNumberOfParamsError{Func: "find_in_parent_folders", Expected: "0, 1, or 2", Actual: numParams})
	}

	previousDir := filepath.Dir(pctx.TerragruntConfigPath)

	if fileToFindParam == "" || fileToFindParam == DefaultTerragruntConfigPath {
		allControls := pctx.StrictControls
		rootTGHCLControl := allControls.FilterByNames(controls.RootTerragruntHCL)
		logger := log.ContextWithLogger(ctx, l)

		if err := rootTGHCLControl.Evaluate(logger); err != nil {
			return "", clihelper.NewExitError(err, clihelper.ExitCodeGeneralError)
		}
	}

	// The strict control above will make this function return an error when no parameter is passed.
	// When this becomes a breaking change, we can remove the strict control and
	// do some validation here to ensure that users aren't using "terragrunt.hcl" as the root of their Terragrunt
	// configurations.
	fileToFindStr := DefaultTerragruntConfigPath
	if fileToFindParam != "" {
		fileToFindStr = fileToFindParam
	}

	// To avoid getting into an accidental infinite loop (e.g. do to cyclical symlinks), set a max on the number of
	// parent folders we'll check
	for range pctx.MaxFoldersToCheck {
		currentDir := filepath.Dir(previousDir)
		if currentDir == previousDir {
			if numParams == matchedPats {
				return fallbackParam, nil
			}

			return "", errors.New(ParentFileNotFoundError{
				Path:  pctx.TerragruntConfigPath,
				File:  fileToFindStr,
				Cause: "Traversed all the way to the root",
			})
		}

		fileToFind := GetDefaultConfigPath(currentDir)
		if fileToFindParam != "" {
			fileToFind = filepath.Join(currentDir, fileToFindParam)
		}

		if util.FileExists(fileToFind) {
			return fileToFind, nil
		}

		previousDir = currentDir
	}

	return "", errors.New(ParentFileNotFoundError{
		Path:  pctx.TerragruntConfigPath,
		File:  fileToFindStr,
		Cause: fmt.Sprintf("Exceeded maximum folders to check (%d)", pctx.MaxFoldersToCheck),
	})
}

// PathRelativeToInclude returns the relative path between the included Terragrunt configuration file
// and the current Terragrunt configuration file. Name param is required and used to lookup the
// relevant import block when called in a child config with multiple import blocks.
func PathRelativeToInclude(ctx context.Context, pctx *ParsingContext, l log.Logger, params []string) (string, error) {
	if pctx.TrackInclude == nil {
		return ".", nil
	}

	var included IncludeConfig

	switch {
	case pctx.TrackInclude.Original != nil:
		included = *pctx.TrackInclude.Original
	case len(pctx.TrackInclude.CurrentList) > 0:
		// Called in child ctx, so we need to select the right include file.
		selected, err := getSelectedIncludeBlock(*pctx.TrackInclude, params)
		if err != nil {
			return "", err
		}

		included = *selected
	default:
		return ".", nil
	}

	currentPath := filepath.Dir(pctx.TerragruntConfigPath)
	includePath := filepath.Dir(included.Path)

	result, err := filepath.Rel(includePath, currentPath)
	if err != nil {
		return "", fmt.Errorf("relativize current path %q against include path %q: %w", currentPath, includePath, err)
	}

	return result, nil
}

// PathRelativeFromInclude returns the relative path from the current Terragrunt configuration to the included Terragrunt configuration file
func PathRelativeFromInclude(ctx context.Context, pctx *ParsingContext, l log.Logger, params []string) (string, error) {
	if pctx.TrackInclude == nil {
		return ".", nil
	}

	included, err := getSelectedIncludeBlock(*pctx.TrackInclude, params)
	if err != nil {
		return "", err
	}

	if included == nil {
		return ".", nil
	}

	includePath := filepath.Dir(included.Path)
	currentPath := filepath.Dir(pctx.TerragruntConfigPath)

	result, err := filepath.Rel(currentPath, includePath)
	if err != nil {
		return "", fmt.Errorf("relativize include path %q against current path %q: %w", includePath, currentPath, err)
	}

	return result, nil
}

// getTerraformCommand returns the current terraform command in execution
func getTerraformCommand(ctx context.Context, pctx *ParsingContext, l log.Logger) (string, error) {
	return pctx.TerraformCommand, nil
}

// getWorkingDir returns the current working dir
func getWorkingDir(ctx context.Context, pctx *ParsingContext, l log.Logger) (string, error) {
	l.Debugf("Start processing get_working_dir built-in function")
	defer l.Debugf("Complete processing get_working_dir built-in function")

	// Initialize evaluation ctx extensions from base blocks.
	pctx.PredefinedFunctions = map[string]function.Function{
		FuncNameGetWorkingDir: wrapVoidToEmptyStringAsFuncImpl(),
	}

	terragruntConfig, err := ParseConfigFile(ctx, pctx, l, pctx.TerragruntConfigPath, nil)
	if err != nil {
		return "", err
	}

	sourceURL, err := GetTerraformSourceURL(pctx.Source, pctx.SourceMap, pctx.OriginalTerragruntConfigPath, terragruntConfig)
	if err != nil {
		return "", err
	}

	// sourceURL will always be at least "." (current directory) to ensure cache is always used
	walkWithSymlinks := pctx.Experiments.Evaluate(experiment.Symlinks)
	// Apply the rewrite so this working-dir computation agrees with the
	// downloader's; otherwise a plain https://www.googleapis.com/storage/...
	// source resolves to a different cache directory.
	sourceURL = tf.RewriteLegacyGCSPublicSource(ctx, l, sourceURL, pctx.StrictControls)

	source, err := tf.NewSource(l, sourceURL, pctx.DownloadDir, pctx.WorkingDir, walkWithSymlinks)
	if err != nil {
		return "", err
	}

	return source.WorkingDir, nil
}

// getTerraformCliArgs returns cli args for terraform
func getTerraformCliArgs(ctx context.Context, pctx *ParsingContext, l log.Logger) ([]string, error) {
	if pctx.TerraformCliArgs == nil {
		return nil, nil
	}

	return pctx.TerraformCliArgs.Slice(), nil
}

// getDefaultRetryableErrors returns default retryable errors for use in errors.retry blocks
func getDefaultRetryableErrors(ctx context.Context, pctx *ParsingContext, l log.Logger) ([]string, error) {
	return retry.DefaultRetryableErrors, nil
}

// getAWSField is a common helper for fetching a single AWS field.
// It builds an AWS config from the parsing context, then calls fetchFn to get the value.
func getAWSField(ctx context.Context, pctx *ParsingContext, l log.Logger, fetchFn func(context.Context, *aws.Config) (string, error)) (string, error) {
	awsConfig, err := awshelper.NewAWSConfigBuilder().
		WithEnv(pctx.Env).
		WithIAMRoleOptions(pctx.IAMRoleOptions).
		Build(ctx, l)
	if err != nil {
		return "", err
	}

	var result string

	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "config_get_aws_field", map[string]any{
		"config_path": pctx.TerragruntConfigPath,
		"role_arn":    pctx.IAMRoleOptions.RoleARN,
	}, func(ctx context.Context) error {
		var fetchErr error

		result, fetchErr = fetchFn(ctx, &awsConfig)

		return fetchErr
	})

	return result, err
}

func getAWSAccountAlias(ctx context.Context, pctx *ParsingContext, l log.Logger) (string, error) {
	return getAWSField(ctx, pctx, l, awshelper.GetAWSAccountAlias)
}

func getAWSAccountID(ctx context.Context, pctx *ParsingContext, l log.Logger) (string, error) {
	return getAWSField(ctx, pctx, l, awshelper.GetAWSAccountID)
}

func getAWSCallerIdentityARN(ctx context.Context, pctx *ParsingContext, l log.Logger) (string, error) {
	return getAWSField(ctx, pctx, l, awshelper.GetAWSIdentityArn)
}

func getAWSCallerIdentityUserID(ctx context.Context, pctx *ParsingContext, l log.Logger) (string, error) {
	return getAWSField(ctx, pctx, l, awshelper.GetAWSUserID)
}

// ParseTerragruntConfig parses the terragrunt config and return a
// representation that can be used as a reference. If given a default value,
// this will return the default if the terragrunt config file does not exist.
func ParseTerragruntConfig(ctx context.Context, pctx *ParsingContext, l log.Logger, configPath string, defaultVal *cty.Value) (cty.Value, error) {
	// target config check: make sure the target config exists. If the file does not exist, and there is no default val,
	// return an error. If the file does not exist but there is a default val, return the default val. Otherwise,
	// proceed to parse the file as a terragrunt config file.
	targetConfig := getCleanedTargetConfigPath(configPath, pctx.TerragruntConfigPath)

	targetConfigFileExists := util.FileExists(targetConfig)

	if !targetConfigFileExists && defaultVal == nil {
		return cty.NilVal, errors.New(TerragruntConfigNotFoundError{Path: targetConfig})
	}

	if !targetConfigFileExists {
		return *defaultVal, nil
	}

	path := targetConfig

	if !filepath.IsAbs(path) {
		path = filepath.Join(pctx.WorkingDir, path)
		path = filepath.Clean(path)
	}

	// Track that this file was read during parsing
	trackFileRead(pctx.FilesRead, path)

	l.Debugf("read_terragrunt_config target=%s caller=%s original=%s workingDir=%s skipOutput=%t skipOutputsResolution=%t decodedDeps=%t",
		targetConfig,
		pctx.TerragruntConfigPath,
		pctx.OriginalTerragruntConfigPath,
		pctx.WorkingDir,
		pctx.SkipOutput,
		pctx.SkipOutputsResolution,
		pctx.DecodedDependencies != nil,
	)

	// We update the ctx of terragruntOptions to the config being read in.
	l, pctx, err := pctx.WithConfigPath(l, targetConfig)
	if err != nil {
		return cty.NilVal, err
	}

	pctx = pctx.WithDiagnosticsSuppressed(l)

	// The parent's decoded dependencies are not the target config's. Reset so the
	// target config decodes its own dependency blocks. Also reset SkipOutputsResolution
	// so that dependency tracing accurately reflects that resolution is happening.
	// See: https://github.com/gruntwork-io/terragrunt/issues/5624
	pctx.DecodedDependencies = nil
	pctx.SkipOutputsResolution = false

	// check if file is stack file, decode as stack file
	if filepath.Base(targetConfig) == DefaultStackFile {
		stackSourceDir := filepath.Dir(targetConfig)

		values, readErr := ReadValues(ctx, pctx, l, stackSourceDir)
		if readErr != nil {
			return cty.NilVal, errors.Errorf("failed to read values from directory %s: %v", stackSourceDir, readErr)
		}

		stackFile, readErr := ReadStackConfigFile(ctx, l, pctx, targetConfig, values)
		if readErr != nil {
			return cty.NilVal, errors.New(readErr)
		}

		return stackConfigAsCty(stackFile)
	}

	// check if file is a values file, decode as values file
	if strings.HasSuffix(targetConfig, valuesFile) {
		unitValues, readErr := ReadValues(ctx, pctx, l, filepath.Dir(targetConfig))
		if readErr != nil {
			return cty.NilVal, errors.New(readErr)
		}

		return *unitValues, nil
	}

	config, err := ParseConfigFile(ctx, pctx, l, targetConfig, nil)
	if err != nil {
		return cty.NilVal, err
	}

	// Surface the target config's dependencies so discovery's DAG sees them (issue #5993).
	recordDependenciesFromRead(pctx, config, targetConfig)

	// We have to set the rendered outputs here because ParseConfigFile will not do so on the TerragruntConfig. The
	// outputs are stored in a special map that is used only for rendering and thus is not available when we try to
	// serialize the config for consumption.
	// NOTE: this will not call terragrunt output, since all the values are cached from the ParseConfigFile call
	// NOTE: we don't use range here because range will copy the slice, thereby undoing the set attribute.
	for i := range len(config.TerragruntDependencies) {
		err := config.TerragruntDependencies[i].setRenderedOutputs(ctx, pctx, l)
		if err != nil {
			return cty.NilVal, errors.New(err)
		}
	}

	return TerragruntConfigAsCty(config)
}

// recordDependenciesFromRead appends `config`'s dependency paths to pctx.dependenciesFromReads.
func recordDependenciesFromRead(pctx *ParsingContext, config *TerragruntConfig, targetConfig string) {
	if pctx == nil || pctx.dependenciesFromReads == nil || config == nil {
		return
	}

	for i := range config.TerragruntDependencies {
		dep := &config.TerragruntDependencies[i]
		if dep.isDisabled() || !IsValidConfigPath(dep.ConfigPath) {
			continue
		}

		appendUniqueResolvedPath(
			pctx.dependenciesFromReads,
			dependencyBlockSourceDir(config, dep, targetConfig),
			dep.ConfigPath.AsString(),
		)
	}

	if config.Dependencies != nil {
		for _, path := range config.Dependencies.Paths {
			appendUniqueResolvedPath(
				pctx.dependenciesFromReads,
				dependencySourceDir(config, MetadataDependencies, path, targetConfig),
				path,
			)
		}
	}
}

func dependencyBlockSourceDir(config *TerragruntConfig, dep *Dependency, fallbackConfig string) string {
	depPath := dep.ConfigPath.AsString()

	if config.Dependencies != nil {
		for _, path := range config.Dependencies.Paths {
			if path == depPath {
				return dependencySourceDir(config, MetadataDependencies, path, fallbackConfig)
			}
		}
	}

	return dependencySourceDir(config, MetadataDependency, dep.Name, fallbackConfig)
}

func dependencySourceDir(config *TerragruntConfig, fieldType, fieldName, fallbackConfig string) string {
	if metadata, found := config.GetMapFieldMetadata(fieldType, fieldName); found {
		if foundInFile := metadata[FoundInFile]; foundInFile != "" {
			return filepath.Dir(foundInFile)
		}
	}

	return filepath.Dir(fallbackConfig)
}

// appendUniqueResolvedPath resolves rawPath against baseDir if relative and appends to *dst when not already present.
func appendUniqueResolvedPath(dst *[]string, baseDir, rawPath string) {
	if rawPath == "" {
		return
	}

	resolved := rawPath
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(baseDir, resolved)
	}

	resolved = filepath.Clean(resolved)

	if slices.Contains(*dst, resolved) {
		return
	}

	*dst = append(*dst, resolved)
}

// Create a cty Function that can be used to for calling read_terragrunt_config.
func readTerragruntConfigAsFuncImpl(ctx context.Context, pctx *ParsingContext, l log.Logger) function.Function {
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
				return cty.NilVal, errors.New(WrongNumberOfParamsError{Func: "read_terragrunt_config", Expected: "1 or 2", Actual: numParams})
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

			return ParseTerragruntConfig(ctx, pctx, l, targetConfigPath, defaultVal)
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
		targetConfig = filepath.Join(cwd, targetConfig)
	}

	if util.IsDir(targetConfig) {
		targetConfig = GetDefaultConfigPath(targetConfig)
	}

	return filepath.Clean(targetConfig)
}

// GetTerragruntSourceForModule returns the source path for a module based on the source path of the parent module and the
// source path specified in the module's terragrunt.hcl file.
//
// If one of the xxx-all commands is called with the --source parameter, then for each module, we need to
// build its own --source parameter by doing the following:
//
// 1. Read the source URL from the Terragrunt configuration of each module
// 2. Extract the path from that URL (the part after a double-slash)
// 3. Append the path to the --source parameter
//
// Example:
//
// --source: /source/infrastructure-modules
// source param in module's terragrunt.hcl: git::git@github.com:acme/infrastructure-modules.git//networking/vpc?ref=v0.0.1
//
// This method will return: /source/infrastructure-modules//networking/vpc
func GetTerragruntSourceForModule(sourcePath string, modulePath string, moduleTerragruntConfig *TerragruntConfig) (string, error) {
	if sourcePath == "" || moduleTerragruntConfig.Terraform == nil || moduleTerragruntConfig.Terraform.Source == nil || *moduleTerragruntConfig.Terraform.Source == "" {
		return "", nil
	}

	// Split the module source string into a valid URL and subdirectory (if // is present).
	moduleURL, moduleSubdir := getter.SourceDirSubdir(*moduleTerragruntConfig.Terraform.Source)

	// if both URL and subdir are missing, something went terribly wrong
	if moduleURL == "" && moduleSubdir == "" {
		return "", errors.New(InvalidSourceURLError{
			ModulePath:       modulePath,
			ModuleSourceURL:  *moduleTerragruntConfig.Terraform.Source,
			TerragruntSource: sourcePath,
		})
	}

	// if only subdir is missing, check if we can obtain a valid module name from the URL portion
	if moduleURL != "" && moduleSubdir == "" {
		moduleSubdirFromURL, err := getModulePathFromSourceURL(moduleURL)
		if err != nil {
			return moduleSubdirFromURL, err
		}

		return util.JoinTerraformModulePath(sourcePath, moduleSubdirFromURL), nil
	}

	return util.JoinTerraformModulePath(sourcePath, moduleSubdir), nil
}

// Parse sourceUrl not containing '//', and attempt to obtain a module path.
// Example:
//
// sourceUrl = "git::ssh://git@ghe.ourcorp.com/OurOrg/module-name.git"
// will return "module-name".
func getModulePathFromSourceURL(sourceURL string) (string, error) {
	// Regexp for module name extraction. It assumes that the query string has already been stripped off.
	// Then we simply capture anything after the last slash, and before `.` or end of string.
	var moduleNameRegexp = regexp.MustCompile(`(?:.+/)(.+?)(?:\.|$)`)

	// strip off the query string if present
	sourceURL = strings.Split(sourceURL, "?")[0]

	matches := moduleNameRegexp.FindStringSubmatch(sourceURL)

	// if regexp returns less/more than the full match + 1 capture group, then something went wrong with regex (invalid source string)
	if len(matches) != matchedPats {
		return "", errors.New(ParsingModulePathError{ModuleSourceURL: sourceURL})
	}

	return matches[1], nil
}

// decrypts and returns sops encrypted utf-8 yaml or json data as a string
func sopsDecryptFile(ctx context.Context, pctx *ParsingContext, l log.Logger, params []string) (string, error) {
	if len(params) != 1 {
		return "", errors.New(WrongNumberOfParamsError{Func: "sops_decrypt_file", Expected: "1", Actual: len(params)})
	}

	sourceFile := params[0]

	format, err := getSopsFileFormat(sourceFile)
	if err != nil {
		return "", fmt.Errorf("determining sops file format: %w", err)
	}

	path := sourceFile

	if !filepath.IsAbs(path) {
		path = filepath.Join(pctx.WorkingDir, path)
		path = filepath.Clean(path)
	}

	trackFileRead(pctx.FilesRead, path)

	return sopsDecryptFileImpl(ctx, pctx, l, path, format, decrypt.File)
}

// sopsDecryptFileImpl contains the actual implementation of sopsDecryptFile
func sopsDecryptFileImpl(ctx context.Context, pctx *ParsingContext, l log.Logger, path string, format string, decryptFn func(string, string) ([]byte, error)) (string, error) {
	sopsCache := cache.ContextCache[string](ctx, SopsCacheContextKey)

	// Fast path: check cache before acquiring lock.
	// Cache has its own sync.RWMutex, safe for concurrent reads.
	if val, ok := sopsCache.Get(ctx, path); ok {
		l.Debugf("sops decrypt: cache hit for %s (len=%d)", path, len(val))

		return val, nil
	}

	// Cache miss: acquire lock for env mutation + decrypt.
	// The lock serializes os.Setenv/os.Unsetenv to prevent race conditions
	// when multiple units decrypt concurrently with different auth credentials.
	// See https://github.com/gruntwork-io/terragrunt/issues/5515
	l.Debugf("sops decrypt: cache miss, acquiring lock for %s (format=%s)", path, format)

	locks.EnvLock.Lock()
	defer locks.EnvLock.Unlock()

	// Double-check: another goroutine may have populated cache while we waited for the lock.
	if val, ok := sopsCache.Get(ctx, path); ok {
		l.Debugf("sops decrypt: cache hit after lock for %s (len=%d)", path, len(val))

		return val, nil
	}

	// Set env vars from opts.Env that are missing from process env.
	// Auth-provider credentials (e.g., AWS_SESSION_TOKEN) may not exist
	// in process env yet — SOPS needs them for KMS auth.
	// Existing process env vars are preserved to avoid overriding real
	// credentials with empty auth-provider values.
	env := pctx.Env

	setKeys := make([]string, 0, len(env))

	for k, v := range env {
		if _, exists := os.LookupEnv(k); exists {
			continue
		}

		os.Setenv(k, v) //nolint:errcheck

		setKeys = append(setKeys, k)
	}

	defer func() {
		for _, k := range setKeys {
			os.Unsetenv(k) //nolint:errcheck
		}
	}()

	l.Debugf("sops decrypt: decrypting %s", path)

	var rawData []byte

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "config_sops_decrypt", map[string]any{
		"path":   path,
		"format": format,
	}, func(ctx context.Context) error {
		var decryptErr error

		rawData, decryptErr = decryptFn(path, format)

		return decryptErr
	})
	if err != nil {
		return "", errors.New(extractSopsErrors(err))
	}

	if utf8.Valid(rawData) {
		value := string(rawData)
		sopsCache.Put(ctx, path, value)

		return value, nil
	}

	return "", errors.New(InvalidSopsFormatError{SourceFilePath: path})
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

// Return the location of the Terraform files provided via --source
func getTerragruntSourceCliFlag(ctx context.Context, pctx *ParsingContext, l log.Logger) (string, error) {
	return pctx.Source, nil
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
		return nil, errors.New(WrongNumberOfParamsError{Func: "path_relative_from_include", Expected: "1", Actual: numParams})
	}

	importName := params[0]

	imported, hasKey := importMap[importName]
	if !hasKey {
		return nil, errors.New(InvalidIncludeKeyError{name: importName})
	}

	return &imported, nil
}

// StartsWith Implementation of Terraform's StartsWith function
//
//nolint:dupl
func StartsWith(ctx context.Context, pctx *ParsingContext, args []string) (bool, error) {
	if len(args) != stringCompParams {
		return false, errors.New(WrongNumberOfParamsError{Func: "startswith", Expected: strconv.Itoa(stringCompParams), Actual: len(args)})
	}

	return strings.HasPrefix(args[0], args[1]), nil
}

// EndsWith Implementation of Terraform's EndsWith function
//
//nolint:dupl
func EndsWith(ctx context.Context, pctx *ParsingContext, args []string) (bool, error) {
	if len(args) != stringCompParams {
		return false, errors.New(WrongNumberOfParamsError{Func: "endswith", Expected: strconv.Itoa(stringCompParams), Actual: len(args)})
	}

	return strings.HasSuffix(args[0], args[1]), nil
}

// TimeCmp implements Terraform's `timecmp` function that compares two timestamps.
func TimeCmp(ctx context.Context, pctx *ParsingContext, l log.Logger, args []string) (int64, error) {
	if len(args) != matchedPats {
		return 0, errors.New(errors.New("function can take only two parameters: timestamp_a and timestamp_b"))
	}

	tsA, err := util.ParseTimestamp(args[0])
	if err != nil {
		return 0, errors.New(fmt.Errorf("could not parse first parameter %q: %w", args[0], err))
	}

	tsB, err := util.ParseTimestamp(args[1])
	if err != nil {
		return 0, errors.New(fmt.Errorf("could not parse second parameter %q: %w", args[1], err))
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
//
//nolint:dupl
func StrContains(ctx context.Context, pctx *ParsingContext, args []string) (bool, error) {
	if len(args) != stringCompParams {
		return false, errors.New(WrongNumberOfParamsError{Func: "strcontains", Expected: strconv.Itoa(stringCompParams), Actual: len(args)})
	}

	return strings.Contains(args[0], args[1]), nil
}

// readTFVarsFile reads a *.tfvars or *.tfvars.json file and returns the contents as a JSON encoded string
func readTFVarsFile(ctx context.Context, pctx *ParsingContext, l log.Logger, args []string) (string, error) {
	return readTFVarsFileImpl(pctx, l, args)
}

// readTFVarsFileImpl contains the actual implementation of readTFVarsFile
func readTFVarsFileImpl(pctx *ParsingContext, l log.Logger, args []string) (string, error) {
	if len(args) != 1 {
		return "", errors.New(WrongNumberOfParamsError{Func: "read_tfvars_file", Expected: "1", Actual: len(args)})
	}

	varFile := args[0]

	if !filepath.IsAbs(varFile) {
		varFile = filepath.Join(pctx.WorkingDir, varFile)
		varFile = filepath.Clean(varFile)
	}

	if !util.FileExists(varFile) {
		return "", errors.New(TFVarFileNotFoundError{File: varFile})
	}

	// Track that this file was read during parsing
	trackFileRead(pctx.FilesRead, varFile)

	fileContents, err := os.ReadFile(varFile)
	if err != nil {
		return "", errors.New(fmt.Errorf("could not read file %q: %w", varFile, err))
	}

	if strings.HasSuffix(varFile, "json") {
		var variables map[string]any
		// just want to be sure that the file is valid json
		if err := json.Unmarshal(fileContents, &variables); err != nil {
			return "", errors.New(fmt.Errorf("could not unmarshal json body of tfvar file: %w", err))
		}

		return string(fileContents), nil
	}

	var variables map[string]any
	if err := ParseAndDecodeVarFile(l, varFile, fileContents, &variables); err != nil {
		return "", err
	}

	data, err := json.Marshal(variables)
	if err != nil {
		return "", errors.New(fmt.Errorf("could not marshal json body of tfvar file: %w", err))
	}

	return string(data), nil
}

// markAsRead marks a file as explicitly read. This is useful for detection via TerragruntUnitsReading flag.
func markAsRead(ctx context.Context, pctx *ParsingContext, l log.Logger, args []string) (string, error) {
	if len(args) != 1 {
		return "", errors.New(WrongNumberOfParamsError{Func: "mark_as_read", Expected: "1", Actual: len(args)})
	}

	file := args[0]

	// Copy the file path to avoid modifying the original.
	// This is necessary so that the HCL function doesn't
	// return a different value than the original file path.
	path := file

	if !filepath.IsAbs(path) {
		path = filepath.Join(pctx.WorkingDir, path)
		path = filepath.Clean(path)
	}

	trackFileRead(pctx.FilesRead, path)

	return file, nil
}

// markGlobAsRead expands the given glob pattern and marks each matched file as
// read. Pattern syntax follows the internal/glob package: '/' is the
// separator, `**` matches any sequence of characters, `*` matches within a
// single segment, and '\' escapes the next metacharacter. `**` only
// collapses the flanking separators when the adjacent segments are literals,
// so "a/**/*.tf" will not match "a/b.tf"; use "a/{*.tf,**/*.tf}" to cover
// both depths. Returns the list of absolute file paths that were marked.
func markGlobAsRead(ctx context.Context, pctx *ParsingContext, l log.Logger, args []string) ([]string, error) {
	if !pctx.Experiments.Evaluate(experiment.MarkManyAsRead) {
		pattern := ""
		if len(args) > 0 {
			pattern = args[0]
		}

		return nil, errors.New(MarkGlobAsReadRequiresExperimentError{
			ConfigPath: pctx.TerragruntConfigPath,
			Pattern:    pattern,
		})
	}

	if len(args) != 1 {
		return nil, errors.New(WrongNumberOfParamsError{Func: FuncNameMarkGlobAsRead, Expected: "1", Actual: len(args)})
	}

	raw := args[0]

	// Keep the pattern in forward-slash space end-to-end. filepath.Join and
	// filepath.Clean would rewrite '/' to '\' on Windows, and a subsequent
	// filepath.ToSlash would then clobber any user-supplied '\' escapes that
	// gobwas relies on to match literal metacharacters.
	var pattern string
	if filepath.IsAbs(raw) {
		pattern = path.Clean(raw)
	} else {
		pattern = path.Clean(filepath.ToSlash(pctx.WorkingDir) + "/" + raw)
	}

	matches, err := glob.Expand(vfs.NewOSFS(), pattern, glob.WithFilesOnly())
	if err != nil {
		return nil, errors.New(fmt.Errorf("could not expand glob %q: %w", raw, err))
	}

	result := make([]string, 0, len(matches))

	for _, match := range matches {
		trackFileRead(pctx.FilesRead, match)
		result = append(result, match)
	}

	return result, nil
}

// warnWhenFileNotMarkedAsRead warns when a file is not being marked as read, even though a user might expect it to be.
// Situations where this is the case include:
// - A user specifies a file in the UnitsReading flag and that file is being read while parsing the inputs attribute.
//
// When the file is not marked as read, the function will return true, otherwise false.

// ParseAndDecodeVarFile uses the HCL2 file to parse the given varfile string into an HCL file body, and then decode it
// into the provided output.
func ParseAndDecodeVarFile(l log.Logger, varFile string, fileContents []byte, out any) error {
	parser := hclparse.NewParser(hclparse.WithLogger(l))

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

	ctyVal, err := ConvertValuesMapToCtyVal(valMap)
	if err != nil {
		return err
	}

	if ctyVal.IsNull() {
		// If the file is empty, doesn't make sense to do conversion
		return nil
	}

	typedOut, hasType := out.(*map[string]any)
	if hasType {
		genericMap, err := ctyhelper.ParseCtyValueToMap(ctyVal)
		if err != nil {
			return err
		}

		*typedOut = genericMap

		return nil
	}

	return gocty.FromCtyValue(ctyVal, out)
}

// extractSopsErrors extracts the original errors from the sops library and returns them as a errors.MultiError.
func extractSopsErrors(err error) *errors.MultiError {
	var errs = &errors.MultiError{}

	// workaround to extract original errors from sops library
	// using reflection extract GroupResults from getDataKeyError
	// may not be compatible with future versions
	errValue := reflect.ValueOf(err)
	if errValue.Kind() == reflect.Pointer {
		errValue = errValue.Elem()
	}

	if errValue.Type().Name() == "getDataKeyError" {
		groupResultsField := errValue.FieldByName("GroupResults")
		if groupResultsField.IsValid() && groupResultsField.Kind() == reflect.Slice {
			for i := range groupResultsField.Len() {
				groupErr := groupResultsField.Index(i)
				if groupErr.CanInterface() {
					if err, ok := groupErr.Interface().(error); ok {
						errs = errs.Append(err)
					}
				}
			}
		}
	}

	// append the original error if no group results were found
	if errs.Len() == 0 {
		errs = errs.Append(err)
	}

	return errs
}

// ConstraintCheck Implementation of Terraform's StartsWith function
func ConstraintCheck(ctx context.Context, pctx *ParsingContext, args []string) (bool, error) {
	if len(args) != matchedPats {
		return false, errors.New(WrongNumberOfParamsError{Func: FuncNameConstraintCheck, Expected: "2", Actual: len(args)})
	}

	v, err := version.NewSemver(args[0])
	if err != nil {
		return false, errors.Errorf("invalid version %s: %w", args[0], err)
	}

	c, err := version.NewConstraint(args[1])
	if err != nil {
		return false, errors.Errorf("invalid constraint %s: %w", args[1], err)
	}

	return c.Check(v), nil
}

// trackFileRead adds a file path to the FilesRead slice if it's not already present.
// This prevents duplicate entries when the same file is read multiple times during parsing.
func trackFileRead(filesRead *[]string, path string) {
	if filesRead == nil {
		return
	}

	if slices.Contains(*filesRead, path) {
		return
	}

	*filesRead = append(*filesRead, path)
}
