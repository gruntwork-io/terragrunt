// Package run provides the main entry point for running orchestrated runs.
//
// These runs are typically OpenTofu/Terraform invocations, but they might be other commands as well.
package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/codegen"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/amazonsts"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"

	"github.com/hashicorp/go-multierror"
)

const (
	CommandNameTerragruntReadConfig = "terragrunt-read-config"
	NullTFVarsFile                  = ".terragrunt-null-vars.auto.tfvars.json"

	useLegacyNullValuesEnvVar = "TERRAGRUNT_TEMP_QUOTE_NULL"
)

var TerraformCommandsThatUseState = []string{
	"init",
	"apply",
	"destroy",
	"env",
	"import",
	"graph",
	"output",
	"plan",
	"push",
	"refresh",
	"show",
	"taint",
	"untaint",
	"validate",
	"force-unlock",
	"state",
}

// TerraformCommandsThatDoNotNeedInit is a list of Terraform commands that do not require 'terraform init' to be executed.
var TerraformCommandsThatDoNotNeedInit = []string{
	"version",
}

var ModuleRegex = regexp.MustCompile(`module[[:blank:]]+".+"`)

const (
	TerraformExtensionGlob = "*.tf"
	TofuExtensionGlob      = "*.tofu"
)

// sourceChangeLocks is a map that keeps track of locks for source changes, to ensure we aren't overriding the generated
// code while another hook (e.g. `tflint`) is running. We use sync.Map to ensure atomic updates during concurrent access.
var sourceChangeLocks = sync.Map{}

// Run downloads terraform source if necessary, then runs terraform with the given options and CLI args.
// This will forward all the args and extra_arguments directly to Terraform.
func Run(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	r *report.Report,
	cfg *runcfg.RunConfig,
	credsGetter *creds.Getter,
) error {
	engine, err := cfg.EngineOptions()
	if err != nil {
		return err
	}

	opts.Engine = engine

	errConfig, err := cfg.ErrorsConfig()
	if err != nil {
		return err
	}

	opts.Errors = errConfig

	l, terragruntOptionsClone, err := opts.CloneWithConfigPath(l, opts.TerragruntConfigPath)
	if err != nil {
		return err
	}

	terragruntOptionsClone.TerraformCommand = CommandNameTerragruntReadConfig

	if err = terragruntOptionsClone.RunWithErrorHandling(ctx, l, r, func() error {
		return ProcessHooks(ctx, l, cfg.Terraform.AfterHooks, terragruntOptionsClone, cfg, nil, r)
	}); err != nil {
		return err
	}

	// We merge the OriginalIAMRoleOptions into the one from the config, because the CLI passed IAMRoleOptions has
	// precedence.
	opts.IAMRoleOptions = options.MergeIAMRoleOptions(
		cfg.GetIAMRoleOptions(),
		opts.OriginalIAMRoleOptions,
	)

	if err = opts.RunWithErrorHandling(ctx, l, r, func() error {
		return credsGetter.ObtainAndUpdateEnvIfNecessary(ctx, l, opts, amazonsts.NewProvider(l, opts))
	}); err != nil {
		return err
	}

	// get the default download dir
	_, defaultDownloadDir, err := options.DefaultWorkingAndDownloadDirs(opts.TerragruntConfigPath)
	if err != nil {
		return err
	}

	// if the download dir hasn't been changed from default, and is set in the config,
	// then use it
	if opts.DownloadDir == defaultDownloadDir && cfg.DownloadDir != "" {
		opts.DownloadDir = cfg.DownloadDir
	}

	updatedTerragruntOptions := opts

	sourceURL, err := runcfg.GetTerraformSourceURL(opts, cfg)
	if err != nil {
		return err
	}

	if sourceURL != "" {
		err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "download_terraform_source", map[string]any{
			"sourceUrl": sourceURL,
		}, func(ctx context.Context) error {
			updatedTerragruntOptions, err = DownloadTerraformSource(ctx, l, sourceURL, opts, cfg, r)
			return err
		})
		if err != nil {
			return err
		}
	}

	// Handle code generation configs, both generate blocks and generate attribute of remote_state.
	// Note that relative paths are relative to the terragrunt working dir (where terraform is called).
	if err = GenerateConfig(l, updatedTerragruntOptions, cfg); err != nil {
		return err
	}

	// We do the debug file generation here, after all the terragrunt generated terraform files are created so that we
	// can ensure the tfvars json file only includes the vars that are defined in the module.
	if updatedTerragruntOptions.Debug {
		if err := WriteTerragruntDebugFile(l, updatedTerragruntOptions, cfg); err != nil {
			return err
		}
	}

	if err := CheckFolderContainsTerraformCode(updatedTerragruntOptions); err != nil {
		return err
	}

	if err := opts.RunWithErrorHandling(ctx, l, r, func() error {
		return runTerragruntWithConfig(ctx, l, opts, updatedTerragruntOptions, cfg, r)
	}); err != nil {
		return err
	}

	return nil
}

// GenerateConfig handles code generation using config types (for backwards compatibility).
func GenerateConfig(l log.Logger, opts *options.TerragruntOptions, cfg *runcfg.RunConfig) error {
	rawActualLock, _ := sourceChangeLocks.LoadOrStore(opts.DownloadDir, &sync.Mutex{})

	actualLock := rawActualLock.(*sync.Mutex)
	defer actualLock.Unlock()

	actualLock.Lock()

	for _, genCfg := range cfg.GenerateConfigs {
		if err := codegen.WriteToFile(l, opts, opts.WorkingDir, genCfg); err != nil {
			return err
		}
	}

	if cfg.RemoteState.Config != nil && cfg.RemoteState.Generate != nil {
		if err := cfg.RemoteState.GenerateOpenTofuCode(l, opts); err != nil {
			return err
		}
	} else if cfg.RemoteState.Config != nil {
		// We use else if here because we don't need to check the backend configuration is defined when the remote state
		// block has a `generate` attribute configured.
		if err := checkTerraformCodeDefinesBackend(opts, cfg.RemoteState.BackendName); err != nil {
			return err
		}
	}

	return nil
}

// Runs tofu/terraform with the given options and CLI args.
// This will forward all the args and extra_arguments directly to Terraform.
//
// This function takes in the "original" terragrunt options which has the unmodified 'WorkingDir' from before downloading the code from the source URL,
// and the "updated" terragrunt options that will contain the updated 'WorkingDir' into which the code has been downloaded
func runTerragruntWithConfig(
	ctx context.Context,
	l log.Logger,
	originalOpts *options.TerragruntOptions,
	opts *options.TerragruntOptions,
	cfg *runcfg.RunConfig,
	r *report.Report,
) error {
	if cfg.Exclude.ShouldPreventRun(opts.TerraformCommand) {
		l.Infof("Early exit in terragrunt unit %s due to exclude block with no_run = true", opts.WorkingDir)

		return nil
	}

	if len(cfg.Terraform.ExtraArgs) > 0 {
		args := FilterTerraformExtraArgs(l, opts, cfg)

		opts.InsertTerraformCliArgs(args...)

		maps.Copy(opts.Env, filterTerraformEnvVarsFromExtraArgsRunCfg(opts, cfg))
	}

	if err := SetTerragruntInputsAsEnvVars(l, opts, cfg); err != nil {
		return err
	}

	if opts.TerraformCliArgs.First() == tf.CommandNameInit {
		if err := prepareInitCommandRunCfg(ctx, l, opts, cfg); err != nil {
			return err
		}
	} else {
		if err := PrepareNonInitCommand(ctx, l, originalOpts, opts, cfg, r); err != nil {
			return err
		}
	}

	if !useLegacyNullValues() {
		fileName, err := setTerragruntNullValuesRunCfg(opts, cfg)
		if err != nil {
			return err
		}

		defer func() {
			if fileName != "" {
				if err := os.Remove(fileName); err != nil {
					l.Debugf("Failed to remove null values file %s: %v", fileName, err)
				}
			}
		}()
	}

	// Now that we've run 'init' and have all the source code locally, we can finally run the patch command
	if err := checkProtectedModuleRunCfg(opts, cfg); err != nil {
		return err
	}

	return RunActionWithHooks(ctx, l, "terraform", opts, cfg, r, func(childCtx context.Context) error {
		// Execute the underlying command once; retries and ignores are handled by outer RunWithErrorHandling
		out, runTerraformError := tf.RunCommandWithOutput(childCtx, l, opts, opts.TerraformCliArgs...)

		var lockFileError error
		if ShouldCopyLockFile(opts.TerraformCliArgs, &cfg.Terraform) {
			// Copy the lock file from the Terragrunt working dir (e.g., .terragrunt-cache/xxx/<some-module>) to the
			// user's working dir (e.g., /live/stage/vpc). That way, the lock file will end up right next to the user's
			// terragrunt.hcl and can be checked into version control. Note that in the past, Terragrunt allowed the
			// user's working dir to be different than the directory where the terragrunt.hcl file lived, so just in
			// case, we are using the user's working dir here, rather than just looking at the parent dir of the
			// terragrunt.hcl. However, the default value for the user's working dir, set in options.go, IS just the
			// parent dir of terragrunt.hcl, so these will likely always be the same.
			lockFileError = runcfg.CopyLockFile(l, opts, opts.WorkingDir, originalOpts.WorkingDir)
		}

		// If command failed, log a helpful message
		if runTerraformError != nil {
			if out == nil {
				l.Errorf("%s invocation failed in %s", opts.TofuImplementation, opts.WorkingDir)
			}
		}

		return multierror.Append(runTerraformError, lockFileError).ErrorOrNil()
	})
}

// ShouldCopyLockFile verifies if the lock file should be copied to the user's working directory
// Terraform 0.14 now manages a lock file for providers. This can be updated
// in three ways:
// * `terraform init` in a module where no `.terraform.lock.hcl` exists
// * `terraform init -upgrade`
// * `terraform providers lock`
//
// In any of these cases, terragrunt should attempt to copy the generated
// `.terraform.lock.hcl`
//
// terraform init is not guaranteed to pull all checksums depending on platforms,
// if you already have the provider requested in a cache, or if you are using a mirror.
// There are lots of details at [hashicorp/terraform#27264](https://github.com/hashicorp/terraform/issues/27264#issuecomment-743389837)
// The `providers lock` sub command enables you to ensure that the lock file is
// fully populated.
func ShouldCopyLockFile(args clihelper.Args, terraformConfig *runcfg.TerraformConfig) bool {
	// If the user has explicitly set NoCopyTerraformLockFile to true, then we should not copy the lock file on any command
	// This is useful for users who want to manage the lock file themselves outside the working directory
	if terraformConfig != nil && terraformConfig.NoCopyTerraformLockFile {
		return false
	}

	if args.First() == tf.CommandNameInit {
		return true
	}

	if args.First() == tf.CommandNameProviders && args.Second() == tf.CommandNameLock {
		return true
	}

	return false
}

// RunActionWithHooks runs the given action function surrounded by hooks. That is, run the before hooks first, then, if there were no
// errors, run the action, and finally, run the after hooks. Return any errors hit from the hooks or action.
func RunActionWithHooks(
	ctx context.Context,
	l log.Logger,
	description string,
	opts *options.TerragruntOptions,
	cfg *runcfg.RunConfig,
	r *report.Report,
	action func(ctx context.Context) error,
) error {
	var allErrors *errors.MultiError

	beforeHookErrors := ProcessHooks(ctx, l, cfg.Terraform.BeforeHooks, opts, cfg, allErrors, r)
	allErrors = allErrors.Append(beforeHookErrors)

	var actionErrors error
	if beforeHookErrors == nil {
		actionErrors = action(ctx)
		allErrors = allErrors.Append(actionErrors)
	} else {
		l.Errorf("Errors encountered running before_hooks. Not running '%s'.", description)
	}

	postHookErrors := ProcessHooks(ctx, l, cfg.Terraform.AfterHooks, opts, cfg, allErrors, r)
	errorHookErrors := processErrorHooks(ctx, l, cfg.Terraform.ErrorHooks, opts, allErrors)
	allErrors = allErrors.Append(postHookErrors, errorHookErrors)

	return allErrors.ErrorOrNil()
}

// SetTerragruntInputsAsEnvVars sets the inputs from Terragrunt configurations to TF_VAR_* environment variables for
// OpenTofu/Terraform.
func SetTerragruntInputsAsEnvVars(l log.Logger, opts *options.TerragruntOptions, cfg *runcfg.RunConfig) error {
	asEnvVars, err := ToTerraformEnvVars(l, opts, cfg.Inputs)
	if err != nil {
		return err
	}

	if opts.Env == nil {
		opts.Env = map[string]string{}
	}

	for key, value := range asEnvVars {
		// Don't override any env vars the user has already set
		if _, envVarAlreadySet := opts.Env[key]; !envVarAlreadySet {
			opts.Env[key] = value
		}
	}

	return nil
}

// CheckFolderContainsTerraformCode checks if the folder contains Terraform/OpenTofu code
func CheckFolderContainsTerraformCode(terragruntOptions *options.TerragruntOptions) error {
	found, err := util.DirContainsTFFiles(terragruntOptions.WorkingDir)
	if err != nil {
		return err
	}

	if !found {
		return errors.New(NoTerraformFilesFound(terragruntOptions.WorkingDir))
	}

	return nil
}

// Check that the specified Terraform code defines a backend { ... } block and return an error if doesn't
func checkTerraformCodeDefinesBackend(opts *options.TerragruntOptions, backendType string) error {
	terraformBackendRegexp, err := regexp.Compile(fmt.Sprintf(`backend[[:blank:]]+"%s"`, backendType))
	if err != nil {
		return errors.New(err)
	}

	// Check for backend definitions in .tf and .tofu files using WalkDir
	definesBackend, err := util.RegexFoundInTFFiles(opts.WorkingDir, terraformBackendRegexp)
	if err != nil {
		return err
	}

	if definesBackend {
		return nil
	}

	terraformJSONBackendRegexp, err := regexp.Compile(fmt.Sprintf(`(?m)"backend":[[:space:]]*{[[:space:]]*"%s"`, backendType))
	if err != nil {
		return errors.New(err)
	}

	definesJSONBackend, err := util.Grep(terraformJSONBackendRegexp, opts.WorkingDir+"/**/*.tf.json")
	if err != nil {
		return err
	}

	if definesJSONBackend {
		return nil
	}

	return errors.New(BackendNotDefined{Opts: opts, BackendType: backendType})
}

// Returns true if we need to run `terraform init` to download providers
func providersNeedInit(terragruntOptions *options.TerragruntOptions) bool {
	pluginsPath := filepath.Join(terragruntOptions.DataDir(), "plugins")
	providersPath := filepath.Join(terragruntOptions.DataDir(), "providers")
	terraformLockPath := filepath.Join(terragruntOptions.WorkingDir, tf.TerraformLockFile)

	return (!util.FileExists(pluginsPath) && !util.FileExists(providersPath)) || !util.FileExists(terraformLockPath)
}

func prepareInitOptions(l log.Logger, terragruntOptions *options.TerragruntOptions) (log.Logger, *options.TerragruntOptions, error) {
	// Need to clone the terragruntOptions, so the TerraformCliArgs can be configured to run the init command
	l, initOptions, err := terragruntOptions.CloneWithConfigPath(l, terragruntOptions.TerragruntConfigPath)
	if err != nil {
		return l, nil, err
	}

	initOptions.TerraformCliArgs = []string{tf.CommandNameInit}
	initOptions.WorkingDir = terragruntOptions.WorkingDir
	initOptions.TerraformCommand = tf.CommandNameInit
	initOptions.Headless = true

	initOutputForCommands := []string{tf.CommandNamePlan, tf.CommandNameApply}
	terraformCommand := terragruntOptions.TerraformCliArgs.First()

	if !slices.Contains(initOutputForCommands, terraformCommand) {
		// Since some command can return a json string, it is necessary to suppress output to stdout of the `terraform init` command.
		initOptions.Writer = io.Discard
	}

	if l.Formatter().DisabledColors() || slices.Contains(terragruntOptions.TerraformCliArgs, tf.FlagNameNoColor) {
		initOptions.TerraformCliArgs = append(initOptions.TerraformCliArgs, tf.FlagNameNoColor)
	}

	return l, initOptions, nil
}

// Return true if modules aren't already downloaded and the Terraform templates in this project reference modules.
// Note that to keep the logic in this code very simple, this code ONLY detects the case where you haven't downloaded
// modules at all. Detecting if your downloaded modules are out of date (as opposed to missing entirely) is more
// complicated and not something we handle at the moment.
func modulesNeedInit(terragruntOptions *options.TerragruntOptions) (bool, error) {
	modulesPath := filepath.Join(terragruntOptions.DataDir(), "modules")
	if util.FileExists(modulesPath) {
		return false, nil
	}

	moduleNeedInit := filepath.Join(terragruntOptions.WorkingDir, ModuleInitRequiredFile)
	if util.FileExists(moduleNeedInit) {
		return true, nil
	}

	// Check for module definitions in .tf and .tofu files using WalkDir
	hasModuleDefinition, err := util.RegexFoundInTFFiles(terragruntOptions.WorkingDir, ModuleRegex)
	if err != nil {
		return false, err
	}

	return hasModuleDefinition, nil
}

// remoteStateNeedsInit determines whether remote state initialization is required before running a Terraform command.
// It returns true if:
//   - BackendBootstrap is enabled in options
//   - Remote state configuration is provided
//   - The Terraform command uses state (e.g., plan, apply, destroy, output, etc.)
//   - The remote state backend needs bootstrapping
func remoteStateNeedsInit(
	ctx context.Context,
	l log.Logger,
	remoteState *remotestate.RemoteState,
	opts *options.TerragruntOptions,
) (bool, error) {
	// If backend bootstrap is disabled, we don't need to initialize remote state
	if !opts.BackendBootstrap {
		return false, nil
	}
	// We only configure remote state for the commands that use the tfstate files. We do not configure it for
	// commands such as "get" or "version".
	if remoteState == nil || remoteState.Config == nil || !slices.Contains(
		TerraformCommandsThatUseState,
		opts.TerraformCliArgs.First(),
	) {
		return false, nil
	}

	if ok, err := remoteState.NeedsBootstrap(ctx, l, opts); err != nil || !ok {
		return false, err
	}

	return true, nil
}

// FilterTerraformExtraArgs extracts terraform extra arguments using runcfg types.
func FilterTerraformExtraArgs(l log.Logger, opts *options.TerragruntOptions, cfg *runcfg.RunConfig) []string {
	out := []string{}
	cmd := opts.TerraformCliArgs.First()

	for _, arg := range cfg.Terraform.ExtraArgs {
		for _, argCmd := range arg.Commands {
			if cmd == argCmd {
				lastArg := opts.TerraformCliArgs.Last()
				skipVars := (cmd == tf.CommandNameApply || cmd == tf.CommandNameDestroy) && util.IsFile(lastArg)

				if len(arg.Arguments) > 0 {
					if skipVars {
						for _, a := range arg.Arguments {
							if !strings.HasPrefix(a, "-var") {
								out = append(out, a)
							}
						}
					} else {
						out = append(out, arg.Arguments...)
					}
				}

				if !skipVars {
					for _, file := range arg.VarFiles {
						out = append(out, "-var-file="+file)
					}
				}
			}
		}
	}

	return out
}

// ToTerraformEnvVars converts the given variables to a map of environment variables that will expose those variables to Terraform. The
// keys will be of the format TF_VAR_xxx and the values will be converted to JSON, which Terraform knows how to read
// natively.
func ToTerraformEnvVars(l log.Logger, opts *options.TerragruntOptions, vars map[string]any) (map[string]string, error) {
	if useLegacyNullValues() {
		l.Warnf("⚠️ %s is a temporary workaround to bypass the breaking change in #2663.\nThis flag will be removed in the future.\nDo not rely on it.", useLegacyNullValuesEnvVar)
	}

	out := map[string]string{}

	for varName, varValue := range vars {
		if varValue == nil {
			if useLegacyNullValues() {
				l.Warnf("⚠️ Input `%s` has value `null`. Quoting due to %s.", varName, useLegacyNullValuesEnvVar)
			} else {
				continue
			}
		}

		envVarName := fmt.Sprintf(tf.EnvNameTFVarFmt, varName)

		envVarValue, err := util.AsTerraformEnvVarJSONValue(varValue)
		if err != nil {
			return nil, err
		}

		out[envVarName] = envVarValue
	}

	return out, nil
}

func useLegacyNullValues() bool {
	return os.Getenv(useLegacyNullValuesEnvVar) == "1"
}

// filterTerraformEnvVarsFromExtraArgsRunCfg extracts terraform env vars from extra args using runcfg types.
func filterTerraformEnvVarsFromExtraArgsRunCfg(opts *options.TerragruntOptions, cfg *runcfg.RunConfig) map[string]string {
	out := map[string]string{}
	cmd := opts.TerraformCliArgs.First()

	for _, arg := range cfg.Terraform.ExtraArgs {
		if len(arg.EnvVars) == 0 {
			continue
		}

		for _, argcmd := range arg.Commands {
			if cmd == argcmd {
				maps.Copy(out, arg.EnvVars)
			}
		}
	}

	return out
}

// prepareInitCommandRunCfg prepares for terraform init using runcfg types.
func prepareInitCommandRunCfg(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, cfg *runcfg.RunConfig) error {
	if cfg.RemoteState.Config == nil {
		return nil
	}

	opts.InsertTerraformCliArgs(cfg.RemoteState.GetTFInitArgs()...)

	if !opts.BackendBootstrap {
		return nil
	}

	if err := cfg.RemoteState.Bootstrap(ctx, l, opts); err != nil {
		return err
	}

	return nil
}

// PrepareNonInitCommand prepares for non-init commands using runcfg types.
func PrepareNonInitCommand(
	ctx context.Context,
	l log.Logger,
	originalOpts *options.TerragruntOptions,
	opts *options.TerragruntOptions,
	cfg *runcfg.RunConfig,
	r *report.Report,
) error {
	needsInit, err := needsInitRunCfg(ctx, l, opts, cfg)
	if err != nil {
		return err
	}

	if needsInit {
		if err := runTerraformInitRunCfg(ctx, l, originalOpts, opts, cfg, r); err != nil {
			return err
		}
	}

	return nil
}

// needsInitRunCfg determines if terraform init is needed using runcfg types.
func needsInitRunCfg(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, cfg *runcfg.RunConfig) (bool, error) {
	if slices.Contains(TerraformCommandsThatDoNotNeedInit, opts.TerraformCliArgs.First()) {
		return false, nil
	}

	if providersNeedInit(opts) {
		return true, nil
	}

	modulesNeedsInit, err := modulesNeedInit(opts)
	if err != nil {
		return false, err
	}

	if modulesNeedsInit {
		return true, nil
	}

	if cfg.RemoteState.Config == nil {
		return false, nil
	}

	return remoteStateNeedsInit(ctx, l, &cfg.RemoteState, opts)
}

// runTerraformInitRunCfg runs terraform init using runcfg types.
func runTerraformInitRunCfg(
	ctx context.Context,
	l log.Logger,
	originalOpts *options.TerragruntOptions,
	opts *options.TerragruntOptions,
	cfg *runcfg.RunConfig,
	r *report.Report,
) error {
	if opts.TerraformCliArgs.First() != tf.CommandNameInit && !opts.AutoInit {
		l.Warnf("Detected that init is needed, but Auto-Init is disabled. Continuing with further actions, but subsequent terraform commands may fail.")
		return nil
	}

	l, initOptions, err := prepareInitOptions(l, opts)
	if err != nil {
		return err
	}

	if err := runTerragruntWithConfig(ctx, l, originalOpts, initOptions, cfg, r); err != nil {
		return err
	}

	moduleNeedInit := filepath.Join(opts.WorkingDir, ModuleInitRequiredFile)
	if util.FileExists(moduleNeedInit) {
		return os.Remove(moduleNeedInit)
	}

	return nil
}

// setTerragruntNullValuesRunCfg generates null values tfvars file using runcfg types.
func setTerragruntNullValuesRunCfg(opts *options.TerragruntOptions, cfg *runcfg.RunConfig) (string, error) {
	jsonEmptyVars := make(map[string]any)

	for varName, varValue := range cfg.Inputs {
		if varValue == nil {
			jsonEmptyVars[varName] = nil
		}
	}

	if len(jsonEmptyVars) == 0 {
		return "", nil
	}

	jsonContents, err := json.MarshalIndent(jsonEmptyVars, "", "  ")
	if err != nil {
		return "", errors.New(err)
	}

	varFile := filepath.Join(opts.WorkingDir, NullTFVarsFile)

	const ownerReadWritePermissions = 0600
	if err := os.WriteFile(varFile, jsonContents, os.FileMode(ownerReadWritePermissions)); err != nil {
		return "", errors.New(err)
	}

	return varFile, nil
}

// checkProtectedModuleRunCfg checks if module is protected using runcfg types.
func checkProtectedModuleRunCfg(opts *options.TerragruntOptions, cfg *runcfg.RunConfig) error {
	var destroyFlag = false
	if opts.TerraformCliArgs.First() == tf.CommandNameDestroy {
		destroyFlag = true
	}

	if slices.Contains(opts.TerraformCliArgs, "-"+tf.CommandNameDestroy) {
		destroyFlag = true
	}

	if !destroyFlag {
		return nil
	}

	if cfg.PreventDestroy {
		return errors.New(ModuleIsProtected{Opts: opts})
	}

	return nil
}
