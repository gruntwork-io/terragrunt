package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gruntwork-io/terragrunt/cli/commands/terraform/creds"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform/creds/providers/amazonsts"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/telemetry"

	"github.com/gruntwork-io/terragrunt/terraform"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/hashicorp/go-multierror"
	"github.com/mattn/go-zglob"

	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
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

var TerraformCommandsThatDoNotNeedInit = []string{
	"version",
	"terragrunt-info",
	"graph-dependencies",
}

var ModuleRegex = regexp.MustCompile(`module[[:blank:]]+".+"`)

const TerraformExtensionGlob = "*.tf"

// sourceChangeLocks is a map that keeps track of locks for source changes, to ensure we aren't overriding the generated
// code while another hook (e.g. `tflint`) is running. We use sync.Map to ensure atomic updates during concurrent access.
var sourceChangeLocks = sync.Map{}

// Run downloads terraform source if necessary, then runs terraform with the given options and CLI args.
// This will forward all the args and extra_arguments directly to Terraform.
func Run(ctx context.Context, opts *options.TerragruntOptions) error {
	if opts.TerraformCommand == "" {
		return errors.New(MissingCommand{})
	}

	return runTerraform(ctx, opts, new(Target))
}

func RunWithTarget(ctx context.Context, opts *options.TerragruntOptions, target *Target) error {
	return runTerraform(ctx, opts, target)
}

func runTerraform(ctx context.Context, terragruntOptions *options.TerragruntOptions, target *Target) error {
	// We need to get the credentials from auth-provider-cmd at the very beginning, since the locals block may contain `get_aws_account_id()` func.
	credsGetter := creds.NewGetter()
	if err := credsGetter.ObtainAndUpdateEnvIfNecessary(ctx, terragruntOptions, externalcmd.NewProvider(terragruntOptions)); err != nil {
		return err
	}

	if err := checkVersionConstraints(ctx, terragruntOptions); err != nil {
		return target.runErrorCallback(terragruntOptions, nil, err)
	}

	terragruntConfig, err := config.ReadTerragruntConfig(ctx, terragruntOptions, config.DefaultParserOptions(terragruntOptions))
	if err != nil {
		return target.runErrorCallback(terragruntOptions, terragruntConfig, err)
	}

	if target.isPoint(TargetPointParseConfig) {
		return target.runCallback(ctx, terragruntOptions, terragruntConfig)
	}

	// fetch engine options from the config
	engine, err := terragruntConfig.EngineOptions()
	if err != nil {
		return target.runErrorCallback(terragruntOptions, terragruntConfig, err)
	}

	terragruntOptions.Engine = engine

	errConfig, err := terragruntConfig.ErrorsConfig()
	if err != nil {
		return target.runErrorCallback(terragruntOptions, terragruntConfig, err)
	}

	terragruntOptions.Errors = errConfig

	terragruntOptionsClone, err := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	if err != nil {
		return err
	}

	terragruntOptionsClone.TerraformCommand = CommandNameTerragruntReadConfig

	if err = terragruntOptionsClone.RunWithErrorHandling(ctx, func() error {
		return processHooks(ctx, terragruntConfig.Terraform.GetAfterHooks(), terragruntOptionsClone, terragruntConfig, nil)
	}); err != nil {
		return target.runErrorCallback(terragruntOptions, terragruntConfig, err)
	}

	if terragruntConfig.Skip != nil && *terragruntConfig.Skip {
		terragruntOptions.Logger.Infof(
			"Skipping terragrunt module %s due to skip = true.",
			terragruntOptions.TerragruntConfigPath,
		)

		return nil
	}

	// We merge the OriginalIAMRoleOptions into the one from the config, because the CLI passed IAMRoleOptions has
	// precedence.
	terragruntOptions.IAMRoleOptions = options.MergeIAMRoleOptions(
		terragruntConfig.GetIAMRoleOptions(),
		terragruntOptions.OriginalIAMRoleOptions,
	)

	if err := terragruntOptions.RunWithErrorHandling(ctx, func() error {
		return credsGetter.ObtainAndUpdateEnvIfNecessary(ctx, terragruntOptions, amazonsts.NewProvider(terragruntOptions))
	}); err != nil {
		return err
	}

	// get the default download dir
	_, defaultDownloadDir, err := options.DefaultWorkingAndDownloadDirs(terragruntOptions.TerragruntConfigPath)
	if err != nil {
		return target.runErrorCallback(terragruntOptions, terragruntConfig, err)
	}

	// if the download dir hasn't been changed from default, and is set in the config,
	// then use it
	if terragruntOptions.DownloadDir == defaultDownloadDir && terragruntConfig.DownloadDir != "" {
		terragruntOptions.DownloadDir = terragruntConfig.DownloadDir
	}

	// Override the default value of retryable errors using the value set in the config file
	if terragruntConfig.RetryableErrors != nil {
		terragruntOptions.RetryableErrors = terragruntConfig.RetryableErrors
	}

	if terragruntConfig.RetryMaxAttempts != nil {
		if *terragruntConfig.RetryMaxAttempts < 1 {
			return fmt.Errorf("cannot have less than 1 max retry, but you specified %d", *terragruntConfig.RetryMaxAttempts)
		}

		terragruntOptions.RetryMaxAttempts = *terragruntConfig.RetryMaxAttempts
	}

	if terragruntConfig.RetrySleepIntervalSec != nil {
		if *terragruntConfig.RetrySleepIntervalSec < 0 {
			return fmt.Errorf("cannot sleep for less than 0 seconds, but you specified %d", *terragruntConfig.RetrySleepIntervalSec)
		}

		terragruntOptions.RetrySleepInterval = time.Duration(*terragruntConfig.RetrySleepIntervalSec) * time.Second
	}

	updatedTerragruntOptions := terragruntOptions

	sourceURL, err := config.GetTerraformSourceURL(terragruntOptions, terragruntConfig)
	if err != nil {
		return target.runErrorCallback(terragruntOptions, terragruntConfig, err)
	}

	if sourceURL != "" {
		err = telemetry.Telemetry(ctx, terragruntOptions, "download_terraform_source", map[string]interface{}{
			"sourceUrl": sourceURL,
		}, func(childCtx context.Context) error {
			updatedTerragruntOptions, err = downloadTerraformSource(ctx, sourceURL, terragruntOptions, terragruntConfig)
			return err
		})

		if err != nil {
			return target.runErrorCallback(terragruntOptions, terragruntConfig, err)
		}
	}

	// NOTE: At this point, the terraform source is downloaded to the terragrunt working directory

	if target.isPoint(TargetPointDownloadSource) {
		return target.runCallback(ctx, updatedTerragruntOptions, terragruntConfig)
	}

	// Handle code generation configs, both generate blocks and generate attribute of remote_state.
	// Note that relative paths are relative to the terragrunt working dir (where terraform is called).
	if err = generateConfig(terragruntConfig, updatedTerragruntOptions); err != nil {
		return target.runErrorCallback(terragruntOptions, terragruntConfig, err)
	}

	if target.isPoint(TargetPointGenerateConfig) {
		return target.runCallback(ctx, updatedTerragruntOptions, terragruntConfig)
	}

	// We do the debug file generation here, after all the terragrunt generated terraform files are created so that we
	// can ensure the tfvars json file only includes the vars that are defined in the module.
	if updatedTerragruntOptions.Debug {
		if err := WriteTerragruntDebugFile(updatedTerragruntOptions, terragruntConfig); err != nil {
			return target.runErrorCallback(terragruntOptions, terragruntConfig, err)
		}
	}

	if err := CheckFolderContainsTerraformCode(updatedTerragruntOptions); err != nil {
		return target.runErrorCallback(terragruntOptions, terragruntConfig, err)
	}

	if terragruntOptions.CheckDependentModules {
		allowDestroy := confirmActionWithDependentModules(ctx, terragruntOptions, terragruntConfig)
		if !allowDestroy {
			return nil
		}
	}

	if err := terragruntOptions.RunWithErrorHandling(ctx, func() error {
		return runTerragruntWithConfig(ctx, terragruntOptions, updatedTerragruntOptions, terragruntConfig, target)
	}); err != nil {
		return target.runErrorCallback(terragruntOptions, terragruntConfig, err)
	}

	return nil
}

func generateConfig(terragruntConfig *config.TerragruntConfig, updatedTerragruntOptions *options.TerragruntOptions) error {
	rawActualLock, _ := sourceChangeLocks.LoadOrStore(updatedTerragruntOptions.DownloadDir, &sync.Mutex{})
	actualLock := rawActualLock.(*sync.Mutex)
	defer actualLock.Unlock()
	actualLock.Lock()

	for _, config := range terragruntConfig.GenerateConfigs {
		if err := codegen.WriteToFile(updatedTerragruntOptions, updatedTerragruntOptions.WorkingDir, config); err != nil {
			return err
		}
	}

	if terragruntConfig.RemoteState != nil && terragruntConfig.RemoteState.Generate != nil {
		if err := terragruntConfig.RemoteState.GenerateTerraformCode(updatedTerragruntOptions); err != nil {
			return err
		}
	} else if terragruntConfig.RemoteState != nil {
		// We use else if here because we don't need to check the backend configuration is defined when the remote state
		// block has a `generate` attribute configured.
		if err := checkTerraformCodeDefinesBackend(updatedTerragruntOptions, terragruntConfig.RemoteState.Backend); err != nil {
			return err
		}
	}

	return nil
}

// Runs terraform with the given options and CLI args.
// This will forward all the args and extra_arguments directly to Terraform.
//
// This function takes in the "original" terragrunt options which has the unmodified 'WorkingDir' from before downloading the code from the source URL,
// and the "updated" terragrunt options that will contain the updated 'WorkingDir' into which the code has been downloaded
func runTerragruntWithConfig(ctx context.Context, originalTerragruntOptions *options.TerragruntOptions, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, target *Target) error {
	// Add extra_arguments to the command
	if terragruntConfig.Terraform != nil && terragruntConfig.Terraform.ExtraArgs != nil && len(terragruntConfig.Terraform.ExtraArgs) > 0 {
		args := FilterTerraformExtraArgs(terragruntOptions, terragruntConfig)

		terragruntOptions.InsertTerraformCliArgs(args...)

		for k, v := range filterTerraformEnvVarsFromExtraArgs(terragruntOptions, terragruntConfig) {
			terragruntOptions.Env[k] = v
		}
	}

	if err := SetTerragruntInputsAsEnvVars(terragruntOptions, terragruntConfig); err != nil {
		return err
	}

	if util.FirstArg(terragruntOptions.TerraformCliArgs) == terraform.CommandNameInit {
		if err := prepareInitCommand(ctx, terragruntOptions, terragruntConfig); err != nil {
			return err
		}
	} else {
		if err := prepareNonInitCommand(ctx, originalTerragruntOptions, terragruntOptions, terragruntConfig); err != nil {
			return err
		}
	}

	if !useLegacyNullValues() {
		fileName, err := setTerragruntNullValues(terragruntOptions, terragruntConfig)
		if err != nil {
			return err
		}

		defer func() {
			if fileName != "" {
				if err := os.Remove(fileName); err != nil {
					terragruntOptions.Logger.Debugf("Failed to remove null values file %s: %v", fileName, err)
				}
			}
		}()
	}

	// Now that we've run 'init' and have all the source code locally, we can finally run the patch command
	if target.isPoint(TargetPointInitCommand) {
		return target.runCallback(ctx, terragruntOptions, terragruntConfig)
	}

	if err := checkProtectedModule(terragruntOptions, terragruntConfig); err != nil {
		return err
	}

	return runActionWithHooks(ctx, "terraform", terragruntOptions, terragruntConfig, func(ctx context.Context) error {
		runTerraformError := RunTerraformWithRetry(ctx, terragruntOptions)

		var lockFileError error
		if ShouldCopyLockFile(terragruntOptions.TerraformCliArgs, terragruntConfig.Terraform) {
			// Copy the lock file from the Terragrunt working dir (e.g., .terragrunt-cache/xxx/<some-module>) to the
			// user's working dir (e.g., /live/stage/vpc). That way, the lock file will end up right next to the user's
			// terragrunt.hcl and can be checked into version control. Note that in the past, Terragrunt allowed the
			// user's working dir to be different than the directory where the terragrunt.hcl file lived, so just in
			// case, we are using the user's working dir here, rather than just looking at the parent dir of the
			// terragrunt.hcl. However, the default value for the user's working dir, set in options.go, IS just the
			// parent dir of terragrunt.hcl, so these will likely always be the same.
			lockFileError = config.CopyLockFile(terragruntOptions, terragruntOptions.WorkingDir, originalTerragruntOptions.WorkingDir)
		}

		return multierror.Append(runTerraformError, lockFileError).ErrorOrNil()
	})
}

// confirmActionWithDependentModules - Show warning with list of dependent modules from current module before destroy
func confirmActionWithDependentModules(ctx context.Context, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) bool {
	modules := configstack.FindWhereWorkingDirIsIncluded(ctx, terragruntOptions, terragruntConfig)
	if len(modules) != 0 {
		if _, err := terragruntOptions.ErrWriter.Write([]byte("Detected dependent modules:\n")); err != nil {
			terragruntOptions.Logger.Error(err)
			return false
		}

		for _, module := range modules {
			if _, err := terragruntOptions.ErrWriter.Write([]byte(module.Path + "\n")); err != nil {
				terragruntOptions.Logger.Error(err)
				return false
			}
		}

		prompt := "WARNING: Are you sure you want to continue?"

		shouldRun, err := shell.PromptUserForYesNo(ctx, prompt, terragruntOptions)
		if err != nil {
			terragruntOptions.Logger.Error(err)
			return false
		}

		return shouldRun
	}
	// request user to confirm action in any case
	return true
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
func ShouldCopyLockFile(args []string, terraformConfig *config.TerraformConfig) bool {
	// If the user has explicitly set CopyTerraformLockFile to false, then we should not copy the lock file on any command
	// This is useful for users who want to manage the lock file themselves outside the working directory
	// if the user has not set CopyTerraformLockFile or if they have explicitly defined it to true,
	// then we should copy the lock file on init and providers lock as defined above and not do and early return here
	if terraformConfig != nil && terraformConfig.CopyTerraformLockFile != nil && !*terraformConfig.CopyTerraformLockFile {
		return false
	}

	if util.FirstArg(args) == terraform.CommandNameInit {
		return true
	}

	if util.FirstArg(args) == terraform.CommandNameProviders && util.SecondArg(args) == terraform.CommandNameLock {
		return true
	}

	return false
}

// Run the given action function surrounded by hooks. That is, run the before hooks first, then, if there were no
// errors, run the action, and finally, run the after hooks. Return any errors hit from the hooks or action.
func runActionWithHooks(ctx context.Context, description string, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, action func(ctx context.Context) error) error {
	var allErrors *errors.MultiError
	beforeHookErrors := processHooks(ctx, terragruntConfig.Terraform.GetBeforeHooks(), terragruntOptions, terragruntConfig, allErrors)
	allErrors = allErrors.Append(beforeHookErrors)

	var actionErrors error
	if beforeHookErrors == nil {
		actionErrors = action(ctx)
		allErrors = allErrors.Append(actionErrors)
	} else {
		terragruntOptions.Logger.Errorf("Errors encountered running before_hooks. Not running '%s'.", description)
	}

	postHookErrors := processHooks(ctx, terragruntConfig.Terraform.GetAfterHooks(), terragruntOptions, terragruntConfig, allErrors)
	errorHookErrors := processErrorHooks(ctx, terragruntConfig.Terraform.GetErrorHooks(), terragruntOptions, allErrors)
	allErrors = allErrors.Append(postHookErrors, errorHookErrors)

	return allErrors.ErrorOrNil()
}

// SetTerragruntInputsAsEnvVars sets the inputs from Terragrunt configurations to TF_VAR_* environment variables for
// OpenTofu/Terraform.
func SetTerragruntInputsAsEnvVars(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	asEnvVars, err := ToTerraformEnvVars(terragruntOptions, terragruntConfig.Inputs)
	if err != nil {
		return err
	}

	if terragruntOptions.Env == nil {
		terragruntOptions.Env = map[string]string{}
	}

	for key, value := range asEnvVars {
		// Don't override any env vars the user has already set
		if _, envVarAlreadySet := terragruntOptions.Env[key]; !envVarAlreadySet {
			terragruntOptions.Env[key] = value
		}
	}

	return nil
}

func RunTerraformWithRetry(ctx context.Context, terragruntOptions *options.TerragruntOptions) error {
	// Retry the command configurable time with sleep in between
	for i := 0; i < terragruntOptions.RetryMaxAttempts; i++ {
		if out, err := shell.RunTerraformCommandWithOutput(ctx, terragruntOptions, terragruntOptions.TerraformCliArgs...); err != nil {
			if out == nil || !IsRetryable(terragruntOptions, out) {
				terragruntOptions.Logger.Errorf("%s invocation failed in %s", terragruntOptions.TerraformImplementation, terragruntOptions.WorkingDir)

				return err
			} else {
				terragruntOptions.Logger.Infof("Encountered an error eligible for retrying. Sleeping %v before retrying.\n", terragruntOptions.RetrySleepInterval)
				select {
				case <-time.After(terragruntOptions.RetrySleepInterval):
					// try again
				case <-ctx.Done():
					return errors.New(ctx.Err())
				}
			}
		} else {
			return nil
		}
	}

	return errors.New(MaxRetriesExceeded{terragruntOptions})
}

// IsRetryable checks whether there was an error and if the output matches any of the configured RetryableErrors
func IsRetryable(opts *options.TerragruntOptions, out *util.CmdOutput) bool {
	if !opts.AutoRetry {
		return false
	}
	// When -json is enabled, Terraform will send all output, errors included, to stdout.
	return util.MatchesAny(opts.RetryableErrors, out.Stderr.String()) || util.MatchesAny(opts.RetryableErrors, out.Stdout.String())
}

// Prepare for running 'terraform init' by initializing remote state storage and adding backend configuration arguments
// to the TerraformCliArgs
func prepareInitCommand(ctx context.Context, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	if terragruntConfig.RemoteState != nil {
		// Initialize the remote state if necessary  (e.g. create S3 bucket and DynamoDB table)
		remoteStateNeedsInit, err := remoteStateNeedsInit(terragruntConfig.RemoteState, terragruntOptions)
		if err != nil {
			return err
		}

		if remoteStateNeedsInit {
			if err := terragruntConfig.RemoteState.Initialize(ctx, terragruntOptions); err != nil {
				return err
			}
		}

		// Add backend config arguments to the command
		terragruntOptions.InsertTerraformCliArgs(terragruntConfig.RemoteState.ToTerraformInitArgs()...)
	}

	return nil
}

func CheckFolderContainsTerraformCode(terragruntOptions *options.TerragruntOptions) error {
	files := []string{}

	hclFiles, err := zglob.Glob(terragruntOptions.WorkingDir + "/**/*.tf")
	if err != nil {
		return errors.New(err)
	}

	files = append(files, hclFiles...)

	jsonFiles, err := zglob.Glob(terragruntOptions.WorkingDir + "/**/*.tf.json")
	if err != nil {
		return errors.New(err)
	}

	files = append(files, jsonFiles...)

	if len(files) == 0 {
		return errors.New(NoTerraformFilesFound(terragruntOptions.WorkingDir))
	}

	return nil
}

// Check that the specified Terraform code defines a backend { ... } block and return an error if doesn't
func checkTerraformCodeDefinesBackend(terragruntOptions *options.TerragruntOptions, backendType string) error {
	terraformBackendRegexp, err := regexp.Compile(fmt.Sprintf(`backend[[:blank:]]+"%s"`, backendType))
	if err != nil {
		return errors.New(err)
	}

	definesBackend, err := util.Grep(terraformBackendRegexp, terragruntOptions.WorkingDir+"/**/*.tf")
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

	definesJSONBackend, err := util.Grep(terraformJSONBackendRegexp, terragruntOptions.WorkingDir+"/**/*.tf.json")
	if err != nil {
		return err
	}

	if definesJSONBackend {
		return nil
	}

	return errors.New(BackendNotDefined{Opts: terragruntOptions, BackendType: backendType})
}

// Prepare for running any command other than 'terraform init' by running 'terraform init' if necessary
// This function takes in the "original" terragrunt options which has the unmodified 'WorkingDir' from before downloading the code from the source URL,
// and the "updated" terragrunt options that will contain the updated 'WorkingDir' into which the code has been downloaded
func prepareNonInitCommand(ctx context.Context, originalTerragruntOptions *options.TerragruntOptions, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	needsInit, err := needsInit(terragruntOptions, terragruntConfig)
	if err != nil {
		return err
	}

	if needsInit {
		if err := runTerraformInit(ctx, originalTerragruntOptions, terragruntOptions, terragruntConfig); err != nil {
			return err
		}
	}

	return nil
}

// Determines if 'terraform init' needs to be executed
func needsInit(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) (bool, error) {
	if util.ListContainsElement(TerraformCommandsThatDoNotNeedInit, util.FirstArg(terragruntOptions.TerraformCliArgs)) {
		return false, nil
	}

	if providersNeedInit(terragruntOptions) {
		return true, nil
	}

	modulesNeedsInit, err := modulesNeedInit(terragruntOptions)
	if err != nil {
		return false, err
	}

	if modulesNeedsInit {
		return true, nil
	}

	return remoteStateNeedsInit(terragruntConfig.RemoteState, terragruntOptions)
}

// Returns true if we need to run `terraform init` to download providers
func providersNeedInit(terragruntOptions *options.TerragruntOptions) bool {
	pluginsPath := util.JoinPath(terragruntOptions.DataDir(), "plugins")
	providersPath := util.JoinPath(terragruntOptions.DataDir(), "providers")
	terraformLockPath := util.JoinPath(terragruntOptions.WorkingDir, terraform.TerraformLockFile)

	return !(util.FileExists(pluginsPath) || util.FileExists(providersPath)) || !util.FileExists(terraformLockPath)
}

// Runs the terraform init command to perform what is referred to as Auto-Init in the README.md.
// This is intended to be run when the user runs another terragrunt command (e.g. 'terragrunt apply'),
// but terragrunt determines that 'terraform init' needs to be called prior to running
// the respective terraform command (e.g. 'terraform apply')
//
// The terragruntOptions are assumed to be the options for running the original terragrunt command.
//
// If terraformSource is specified, then arguments to download the terraform source will be appended to the init command.
//
// This method will return an error and NOT run terraform init if the user has disabled Auto-Init.
//
// This method takes in the "original" terragrunt options which has the unmodified 'WorkingDir' from before downloading the code from the source URL,
// and the "updated" terragrunt options that will contain the updated 'WorkingDir' into which the code has been downloaded
func runTerraformInit(ctx context.Context, originalTerragruntOptions *options.TerragruntOptions, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	// Prevent Auto-Init if the user has disabled it
	if util.FirstArg(terragruntOptions.TerraformCliArgs) != terraform.CommandNameInit && !terragruntOptions.AutoInit {
		terragruntOptions.Logger.Warnf("Detected that init is needed, but Auto-Init is disabled. Continuing with further actions, but subsequent terraform commands may fail.")
		return nil
	}

	initOptions, err := prepareInitOptions(terragruntOptions)
	if err != nil {
		return err
	}

	if err := runTerragruntWithConfig(ctx, originalTerragruntOptions, initOptions, terragruntConfig, new(Target)); err != nil {
		return err
	}

	moduleNeedInit := util.JoinPath(terragruntOptions.WorkingDir, ModuleInitRequiredFile)
	if util.FileExists(moduleNeedInit) {
		return os.Remove(moduleNeedInit)
	}

	return nil
}

func prepareInitOptions(terragruntOptions *options.TerragruntOptions) (*options.TerragruntOptions, error) {
	// Need to clone the terragruntOptions, so the TerraformCliArgs can be configured to run the init command
	initOptions, err := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	if err != nil {
		return nil, err
	}

	initOptions.TerraformCliArgs = []string{terraform.CommandNameInit}
	initOptions.WorkingDir = terragruntOptions.WorkingDir
	initOptions.TerraformCommand = terraform.CommandNameInit
	initOptions.Headless = true

	initOutputForCommands := []string{terraform.CommandNamePlan, terraform.CommandNameApply}
	terraformCommand := util.FirstArg(terragruntOptions.TerraformCliArgs)

	if !collections.ListContainsElement(initOutputForCommands, terraformCommand) {
		// Since some command can return a json string, it is necessary to suppress output to stdout of the `terraform init` command.
		initOptions.Writer = io.Discard
	}

	if collections.ListContainsElement(terragruntOptions.TerraformCliArgs, terraform.FlagNameNoColor) {
		initOptions.TerraformCliArgs = append(initOptions.TerraformCliArgs, terraform.FlagNameNoColor)
	}

	return initOptions, nil
}

// Return true if modules aren't already downloaded and the Terraform templates in this project reference modules.
// Note that to keep the logic in this code very simple, this code ONLY detects the case where you haven't downloaded
// modules at all. Detecting if your downloaded modules are out of date (as opposed to missing entirely) is more
// complicated and not something we handle at the moment.
func modulesNeedInit(terragruntOptions *options.TerragruntOptions) (bool, error) {
	modulesPath := util.JoinPath(terragruntOptions.DataDir(), "modules")
	if util.FileExists(modulesPath) {
		return false, nil
	}

	moduleNeedInit := util.JoinPath(terragruntOptions.WorkingDir, ModuleInitRequiredFile)
	if util.FileExists(moduleNeedInit) {
		return true, nil
	}

	return util.Grep(ModuleRegex, fmt.Sprintf("%s/%s", terragruntOptions.WorkingDir, TerraformExtensionGlob))
}

// If the user entered a Terraform command that uses state (e.g. plan, apply), make sure remote state is configured
// before running the command.
func remoteStateNeedsInit(remoteState *remote.RemoteState, terragruntOptions *options.TerragruntOptions) (bool, error) {
	// We only configure remote state for the commands that use the tfstate files. We do not configure it for
	// commands such as "get" or "version".
	if remoteState != nil && util.ListContainsElement(TerraformCommandsThatUseState, util.FirstArg(terragruntOptions.TerraformCliArgs)) {
		return remoteState.NeedsInit(terragruntOptions)
	}

	return false, nil
}

// runAll runs the provided terraform command against all the modules that are found in the directory tree.

// checkProtectedModule checks if module is protected via the "prevent_destroy" flag
func checkProtectedModule(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	var destroyFlag = false
	if util.FirstArg(terragruntOptions.TerraformCliArgs) == terraform.CommandNameDestroy {
		destroyFlag = true
	}

	if util.ListContainsElement(terragruntOptions.TerraformCliArgs, "-"+terraform.CommandNameDestroy) {
		destroyFlag = true
	}

	if !destroyFlag {
		return nil
	}

	if terragruntConfig.PreventDestroy != nil && *terragruntConfig.PreventDestroy {
		return errors.New(ModuleIsProtected{Opts: terragruntOptions})
	}

	return nil
}

func FilterTerraformExtraArgs(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) []string {
	out := []string{}
	cmd := util.FirstArg(terragruntOptions.TerraformCliArgs)

	for _, arg := range terragruntConfig.Terraform.ExtraArgs {
		for _, argCmd := range arg.Commands {
			if cmd == argCmd {
				lastArg := util.LastArg(terragruntOptions.TerraformCliArgs)
				skipVars := (cmd == terraform.CommandNameApply || cmd == terraform.CommandNameDestroy) && util.IsFile(lastArg)

				// The following is a fix for GH-493.
				// If the first argument is "apply" and the second argument is a file (plan),
				// we don't add any -var-file to the command.
				if arg.Arguments != nil {
					if skipVars {
						// If we have to skip vars, we need to iterate over all elements of array...
						for _, a := range *arg.Arguments {
							if !strings.HasPrefix(a, "-var") {
								out = append(out, a)
							}
						}
					} else {
						// ... Otherwise, let's add all the arguments
						out = append(out, *arg.Arguments...)
					}
				}

				if !skipVars {
					varFiles := arg.GetVarFiles(terragruntOptions.Logger)
					for _, file := range varFiles {
						out = append(out, "-var-file="+file)
					}
				}
			}
		}
	}

	return out
}

func filterTerraformEnvVarsFromExtraArgs(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) map[string]string {
	out := map[string]string{}
	cmd := util.FirstArg(terragruntOptions.TerraformCliArgs)

	for _, arg := range terragruntConfig.Terraform.ExtraArgs {
		if arg.EnvVars == nil {
			continue
		}

		for _, argcmd := range arg.Commands {
			if cmd == argcmd {
				for k, v := range *arg.EnvVars {
					out[k] = v
				}
			}
		}
	}

	return out
}

// ToTerraformEnvVars converts the given variables to a map of environment variables that will expose those variables to Terraform. The
// keys will be of the format TF_VAR_xxx and the values will be converted to JSON, which Terraform knows how to read
// natively.
func ToTerraformEnvVars(opts *options.TerragruntOptions, vars map[string]interface{}) (map[string]string, error) {
	if useLegacyNullValues() {
		opts.Logger.Warnf("⚠️ %s is a temporary workaround to bypass the breaking change in #2663.\nThis flag will be removed in the future.\nDo not rely on it.", useLegacyNullValuesEnvVar)
	}

	out := map[string]string{}

	for varName, varValue := range vars {
		if varValue == nil {
			if useLegacyNullValues() {
				opts.Logger.Warnf("⚠️ Input `%s` has value `null`. Quoting due to %s.", varName, useLegacyNullValuesEnvVar)
			} else {
				continue
			}
		}

		envVarName := fmt.Sprintf(terraform.EnvNameTFVarFmt, varName)

		envVarValue, err := util.AsTerraformEnvVarJSONValue(varValue)
		if err != nil {
			return nil, err
		}

		out[envVarName] = envVarValue
	}

	return out, nil
}

// setTerragruntNullValues - Generate a .auto.tfvars.json file with variables which have null values.
func setTerragruntNullValues(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) (string, error) {
	jsonEmptyVars := make(map[string]interface{})

	for varName, varValue := range terragruntConfig.Inputs {
		if varValue == nil {
			jsonEmptyVars[varName] = nil
		}
	}

	// skip generation on empty file
	if len(jsonEmptyVars) == 0 {
		return "", nil
	}

	jsonContents, err := json.MarshalIndent(jsonEmptyVars, "", "  ")
	if err != nil {
		return "", errors.New(err)
	}

	varFile := filepath.Join(terragruntOptions.WorkingDir, NullTFVarsFile)

	const ownerReadWritePermissions = 0600
	if err := os.WriteFile(varFile, jsonContents, os.FileMode(ownerReadWritePermissions)); err != nil {
		return "", errors.New(err)
	}

	return varFile, nil
}

func useLegacyNullValues() bool {
	return os.Getenv(useLegacyNullValuesEnvVar) == "1"
}
