package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gruntwork-io/go-commons/version"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/tflint"

	"github.com/gruntwork-io/gruntwork-cli/collections"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/pkg/env"
	"github.com/hashicorp/go-multierror"
	"github.com/mattn/go-zglob"

	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/cli/command"
	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	graphdependencies "github.com/gruntwork-io/terragrunt/cli/commands/graph-dependencies"
	"github.com/gruntwork-io/terragrunt/cli/commands/hclfmt"
	renderjson "github.com/gruntwork-io/terragrunt/cli/commands/render-json"
	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	terragruntinfo "github.com/gruntwork-io/terragrunt/cli/commands/terragrunt-info"
	validateinputs "github.com/gruntwork-io/terragrunt/cli/commands/validate-inputs"
	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	CmdInit                 = "init"
	CmdInitFromModule       = "init-from-module"
	CmdPlan                 = "plan"
	CmdApply                = "apply"
	CmdProviders            = "providers"
	CmdLock                 = "lock"
	CmdTerragruntReadConfig = "terragrunt-read-config"
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

// This uses the constraint syntax from https://github.com/hashicorp/go-version
// This version of Terragrunt was tested to work with Terraform 0.12.0 and above only
const DefaultTerraformVersionConstraint = ">= v0.12.0"

const TerraformExtensionGlob = "*.tf"

// sourceChangeLocks is a map that keeps track of locks for source changes, to ensure we aren't overriding the generated
// code while another hook (e.g. `tflint`) is running. We use sync.Map to ensure atomic updates during concurrent access.
var sourceChangeLocks = sync.Map{}

func init() {
	cli.AppHelpTemplate = appHelpTemplate
	cli.CommandHelpTemplate = commandHelpTemplate
}

// Create the Terragrunt CLI App
func CreateTerragruntCli(writer io.Writer, errwriter io.Writer) *cli.App {
	opts := options.NewTerragruntOptions()
	// The env vars are renamed in the flags to "..._NO_AUTO_...". These ones are left for backwards compatibility.
	opts.AutoInit = env.GetBoolEnv("TERRAGRUNT_AUTO_INIT", opts.AutoInit)
	opts.AutoRetry = env.GetBoolEnv("TERRAGRUNT_AUTO_RETRY", opts.AutoRetry)
	opts.RunAllAutoApprove = env.GetBoolEnv("TERRAGRUNT_AUTO_APPROVE", opts.RunAllAutoApprove)

	app := cli.NewApp()
	app.Name = "terragrunt"
	app.Author = "Gruntwork <www.gruntwork.io>"
	app.Version = version.GetVersion()
	app.Writer = writer
	app.ErrWriter = errwriter
	app.AddFlags(newFlags(opts)...)
	app.AddCommands(
		runall.NewCommand(opts),
		terragruntinfo.NewCommand(opts),
		validateinputs.NewCommand(opts),
		graphdependencies.NewCommand(opts),
		hclfmt.NewCommand(opts),
		renderjson.NewCommand(opts),
		awsproviderpatch.NewCommand(opts),
		&cli.Command{
			Name:  "*",
			Usage: "Terragrunt forwards all other commands directly to Terraform",
		},
	)
	app.Before = func(ctx *cli.Context) error {
		showHelp := ctx.Flags.Get(flagHelp).Value().IsSet()
		if showHelp {
			// if app command is specified show the command help.
			if !ctx.Command.IsRoot {
				return cli.ShowCommandHelp(ctx, ctx.Command.Name)
			}

			// if there is no args at all show the app help.
			if !ctx.Args().Present() || ctx.Args().First() == "*" {
				return cli.ShowAppHelp(ctx)
			}

			// in other cases show the Terraform help.
			terraformHelpCmd := append([]string{ctx.Args().First(), "-help"}, ctx.Args().Tail()...)
			return shell.RunTerraformCommand(opts, terraformHelpCmd...)
		}

		var err error
		opts, err = prepareTerragruntOptions(ctx, opts)
		return err
	}
	app.Action = func(ctx *cli.Context) error {
		return runTerraformCommand(opts)
	}

	return app
}

func prepareTerragruntOptions(ctx *cli.Context, opts *options.TerragruntOptions) (*options.TerragruntOptions, error) {
	args := ctx.Args().Normalize(cli.OnePrefixFlag)

	opts.TerraformCommand = args.First()
	opts.TerraformCliArgs = args.Slice()

	if _, found := deprecatedCommands[opts.TerraformCommand]; found {
		opts.TerraformCliArgs = args.Tail()
	}

	if err := opts.Normalize(ctx.App.Version, ctx.App.Writer, ctx.App.ErrWriter); err != nil {
		return nil, err
	}

	// Log the terragrunt version in debug mode. This helps with debugging issues and ensuring a specific version of  terragrunt used.
	opts.Logger.Debugf("Terragrunt Version: %s", opts.TerragruntVersion)

	if opts.TerragruntConfigPath == "" {
		opts.TerragruntConfigPath = config.GetDefaultConfigPath(opts.WorkingDir)
	}
	opts.OriginalTerragruntConfigPath = opts.TerragruntConfigPath
	opts.RunTerragrunt = RunTerragrunt

	opts = checkDeprecated(opts)
	return opts, nil
}

// runTerraformCommand runs one or many terraform commands based on the type of
// terragrunt command
func runTerraformCommand(opts *options.TerragruntOptions) error {
	switch opts.TerraformCommand {
	case command.CmdRunAll:
		return command.RunAll(opts)
	case "destroy":
		opts.CheckDependentModules = true
	}

	return RunTerragrunt(opts)
}

// Downloads terraform source if necessary, then runs terraform with the given options and CLI args.
// This will forward all the args and extra_arguments directly to Terraform.
func RunTerragrunt(terragruntOptions *options.TerragruntOptions) error {
	if err := checkVersionConstraints(terragruntOptions); err != nil {
		return err
	}

	terragruntConfig, err := config.ReadTerragruntConfig(terragruntOptions)
	if err != nil {
		return err
	}

	if shouldRunRenderJSON(terragruntOptions) {
		return runRenderJSON(terragruntOptions, terragruntConfig)
	}

	terragruntOptionsClone := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	terragruntOptionsClone.TerraformCommand = CmdTerragruntReadConfig

	if err := processHooks(terragruntConfig.Terraform.GetAfterHooks(), terragruntOptionsClone, terragruntConfig, nil); err != nil {
		return err
	}

	if terragruntConfig.Skip {
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

	if err := aws_helper.AssumeRoleAndUpdateEnvIfNecessary(terragruntOptions); err != nil {
		return err
	}

	// get the default download dir
	_, defaultDownloadDir, err := options.DefaultWorkingAndDownloadDirs(terragruntOptions.TerragruntConfigPath)
	if err != nil {
		return err
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
			return fmt.Errorf("Cannot have less than 1 max retry, but you specified %d", *terragruntConfig.RetryMaxAttempts)
		}
		terragruntOptions.RetryMaxAttempts = *terragruntConfig.RetryMaxAttempts
	}

	if terragruntConfig.RetrySleepIntervalSec != nil {
		if *terragruntConfig.RetrySleepIntervalSec < 0 {
			return fmt.Errorf("Cannot sleep for less than 0 seconds, but you specified %d", *terragruntConfig.RetrySleepIntervalSec)
		}
		terragruntOptions.RetrySleepIntervalSec = time.Duration(*terragruntConfig.RetrySleepIntervalSec) * time.Second
	}

	updatedTerragruntOptions := terragruntOptions
	sourceUrl, err := config.GetTerraformSourceUrl(terragruntOptions, terragruntConfig)
	if err != nil {
		return err
	}
	if sourceUrl != "" {
		updatedTerragruntOptions, err = downloadTerraformSource(sourceUrl, terragruntOptions, terragruntConfig)
		if err != nil {
			return err
		}
	}

	// NOTE: At this point, the terraform source is downloaded to the terragrunt working directory

	if shouldPrintTerragruntInfo(updatedTerragruntOptions) {
		group := TerragruntInfoGroup{
			ConfigPath:       updatedTerragruntOptions.TerragruntConfigPath,
			DownloadDir:      updatedTerragruntOptions.DownloadDir,
			IamRole:          updatedTerragruntOptions.IAMRoleOptions.RoleARN,
			TerraformBinary:  updatedTerragruntOptions.TerraformPath,
			TerraformCommand: updatedTerragruntOptions.TerraformCommand,
			WorkingDir:       updatedTerragruntOptions.WorkingDir,
		}
		b, err := json.MarshalIndent(group, "", "  ")
		if err != nil {
			updatedTerragruntOptions.Logger.Errorf("JSON error marshalling terragrunt-info")
			return err
		}
		fmt.Fprintf(updatedTerragruntOptions.Writer, "%s\n", b)
		return nil
	}

	// Handle code generation configs, both generate blocks and generate attribute of remote_state.
	// Note that relative paths are relative to the terragrunt working dir (where terraform is called).
	if err = generateConfig(terragruntConfig, updatedTerragruntOptions); err != nil {
		return err
	}

	// We do the terragrunt input validation here, after all the terragrunt generated terraform files are created so
	// that we can ensure the necessary information is available.
	if shouldValidateTerragruntInputs(updatedTerragruntOptions) {
		return validateTerragruntInputs(updatedTerragruntOptions, terragruntConfig)
	}

	// We do the debug file generation here, after all the terragrunt generated terraform files are created so that we
	// can ensure the tfvars json file only includes the vars that are defined in the module.
	if updatedTerragruntOptions.Debug {
		err := writeTerragruntDebugFile(updatedTerragruntOptions, terragruntConfig)
		if err != nil {
			return err
		}
	}

	if err := checkFolderContainsTerraformCode(updatedTerragruntOptions); err != nil {
		return err
	}

	if terragruntOptions.CheckDependentModules {
		allowDestroy := confirmActionWithDependentModules(terragruntOptions, terragruntConfig)
		if !allowDestroy {
			return nil
		}
	}
	return runTerragruntWithConfig(terragruntOptions, updatedTerragruntOptions, terragruntConfig, false)
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

// Check the version constraints of both terragrunt and terraform. Note that as a side effect this will set the
// following settings on terragruntOptions:
// - TerraformPath
// - TerraformVersion
// TODO: Look into a way to refactor this function to avoid the side effect.
func checkVersionConstraints(terragruntOptions *options.TerragruntOptions) error {
	partialTerragruntConfig, err := config.PartialParseConfigFile(
		terragruntOptions.TerragruntConfigPath,
		terragruntOptions,
		nil,
		[]config.PartialDecodeSectionType{config.TerragruntVersionConstraints},
	)
	if err != nil {
		return err
	}

	// Change the terraform binary path before checking the version
	// if the path is not changed from default and set in the config.
	if terragruntOptions.TerraformPath == options.TERRAFORM_DEFAULT_PATH && partialTerragruntConfig.TerraformBinary != "" {
		terragruntOptions.TerraformPath = partialTerragruntConfig.TerraformBinary
	}
	if err := PopulateTerraformVersion(terragruntOptions); err != nil {
		return err
	}

	terraformVersionConstraint := DefaultTerraformVersionConstraint
	if partialTerragruntConfig.TerraformVersionConstraint != "" {
		terraformVersionConstraint = partialTerragruntConfig.TerraformVersionConstraint
	}
	if err := CheckTerraformVersion(terraformVersionConstraint, terragruntOptions); err != nil {
		return err
	}

	if partialTerragruntConfig.TerragruntVersionConstraint != "" {
		if err := CheckTerragruntVersion(partialTerragruntConfig.TerragruntVersionConstraint, terragruntOptions); err != nil {
			return err
		}
	}
	return nil
}

func processErrorHooks(hooks []config.ErrorHook, terragruntOptions *options.TerragruntOptions, previousExecErrors *multierror.Error) error {
	if len(hooks) == 0 || previousExecErrors.ErrorOrNil() == nil {
		return nil
	}

	var errorsOccured *multierror.Error

	terragruntOptions.Logger.Debugf("Detected %d error Hooks", len(hooks))

	customMultierror := multierror.Error{
		Errors: previousExecErrors.Errors,
		ErrorFormat: func(err []error) string {
			result := ""
			for _, e := range err {
				errorMessage := e.Error()
				// Check if is process execution error and try to extract output
				// https://github.com/gruntwork-io/terragrunt/issues/2045
				originalError := errors.Unwrap(e)
				if originalError != nil {
					processError, cast := originalError.(shell.ProcessExecutionError)
					if cast {
						errorMessage = fmt.Sprintf("%s\n%s", processError.StdOut, processError.Stderr)
					}
				}
				result = fmt.Sprintf("%s\n%s", result, errorMessage)
			}
			return result
		},
	}
	errorMessage := customMultierror.Error()

	for _, curHook := range hooks {
		if util.MatchesAny(curHook.OnErrors, errorMessage) && util.ListContainsElement(curHook.Commands, terragruntOptions.TerraformCommand) {
			terragruntOptions.Logger.Infof("Executing hook: %s", curHook.Name)
			workingDir := ""
			if curHook.WorkingDir != nil {
				workingDir = *curHook.WorkingDir
			}

			var suppressStdout bool
			if curHook.SuppressStdout != nil && *curHook.SuppressStdout {
				suppressStdout = true
			}

			actionToExecute := curHook.Execute[0]
			actionParams := curHook.Execute[1:]

			_, possibleError := shell.RunShellCommandWithOutput(
				terragruntOptions,
				workingDir,
				suppressStdout,
				false,
				actionToExecute, actionParams...,
			)
			if possibleError != nil {
				terragruntOptions.Logger.Errorf("Error running hook %s with message: %s", curHook.Name, possibleError.Error())
				errorsOccured = multierror.Append(errorsOccured, possibleError)
			}
		}
	}
	return errorsOccured.ErrorOrNil()
}

func processHooks(hooks []config.Hook, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, previousExecErrors *multierror.Error) error {
	if len(hooks) == 0 {
		return nil
	}

	var errorsOccured *multierror.Error

	terragruntOptions.Logger.Debugf("Detected %d Hooks", len(hooks))

	for _, curHook := range hooks {
		allPreviousErrors := multierror.Append(previousExecErrors, errorsOccured)
		if shouldRunHook(curHook, terragruntOptions, allPreviousErrors) {
			err := runHook(terragruntOptions, terragruntConfig, curHook)
			if err != nil {
				errorsOccured = multierror.Append(errorsOccured, err)
			}
		}
	}

	return errorsOccured.ErrorOrNil()
}

func shouldRunHook(hook config.Hook, terragruntOptions *options.TerragruntOptions, previousExecErrors *multierror.Error) bool {
	//if there's no previous error, execute command
	//OR if a previous error DID happen AND we want to run anyways
	//then execute.
	//Skip execution if there was an error AND we care about errors

	//resolves: https://github.com/gruntwork-io/terragrunt/issues/459
	hasErrors := previousExecErrors.ErrorOrNil() != nil
	isCommandInHook := util.ListContainsElement(hook.Commands, terragruntOptions.TerraformCommand)

	return isCommandInHook && (!hasErrors || (hook.RunOnError != nil && *hook.RunOnError))
}

func runHook(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, curHook config.Hook) error {
	terragruntOptions.Logger.Infof("Executing hook: %s", curHook.Name)
	workingDir := ""
	if curHook.WorkingDir != nil {
		workingDir = *curHook.WorkingDir
	}

	var suppressStdout bool
	if curHook.SuppressStdout != nil && *curHook.SuppressStdout {
		suppressStdout = true
	}

	actionToExecute := curHook.Execute[0]
	actionParams := curHook.Execute[1:]

	if actionToExecute == "tflint" {
		if err := executeTFLint(terragruntOptions, terragruntConfig, curHook, workingDir); err != nil {
			return err
		}
	} else {
		_, possibleError := shell.RunShellCommandWithOutput(
			terragruntOptions,
			workingDir,
			suppressStdout,
			false,
			actionToExecute, actionParams...,
		)
		if possibleError != nil {
			terragruntOptions.Logger.Errorf("Error running hook %s with message: %s", curHook.Name, possibleError.Error())
			return possibleError
		}
	}
	return nil
}

func executeTFLint(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, curHook config.Hook, workingDir string) error {
	// fetching source code changes lock since tflint is not thread safe
	rawActualLock, _ := sourceChangeLocks.LoadOrStore(workingDir, &sync.Mutex{})
	actualLock := rawActualLock.(*sync.Mutex)
	actualLock.Lock()
	defer actualLock.Unlock()
	err := tflint.RunTflintWithOpts(terragruntOptions, terragruntConfig)
	if err != nil {
		terragruntOptions.Logger.Errorf("Error running hook %s with message: %s", curHook.Name, err.Error())
		return err
	}
	return nil
}

// Runs terraform with the given options and CLI args.
// This will forward all the args and extra_arguments directly to Terraform.

// This function takes in the "original" terragrunt options which has the unmodified 'WorkingDir' from before downloading the code from the source URL,
// and the "updated" terragrunt options that will contain the updated 'WorkingDir' into which the code has been downloaded
func runTerragruntWithConfig(originalTerragruntOptions *options.TerragruntOptions, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, allowSourceDownload bool) error {
	// Add extra_arguments to the command
	if terragruntConfig.Terraform != nil && terragruntConfig.Terraform.ExtraArgs != nil && len(terragruntConfig.Terraform.ExtraArgs) > 0 {
		args := filterTerraformExtraArgs(terragruntOptions, terragruntConfig)
		terragruntOptions.InsertTerraformCliArgs(args...)
		for k, v := range filterTerraformEnvVarsFromExtraArgs(terragruntOptions, terragruntConfig) {
			terragruntOptions.Env[k] = v
		}
	}

	if err := setTerragruntInputsAsEnvVars(terragruntOptions, terragruntConfig); err != nil {
		return err
	}

	if util.FirstArg(terragruntOptions.TerraformCliArgs) == CmdInit {
		if err := prepareInitCommand(terragruntOptions, terragruntConfig, allowSourceDownload); err != nil {
			return err
		}
	} else {
		if err := prepareNonInitCommand(originalTerragruntOptions, terragruntOptions, terragruntConfig); err != nil {
			return err
		}
	}

	// Now that we've run 'init' and have all the source code locally, we can finally run the patch command
	if shouldApplyAwsProviderPatch(terragruntOptions) {
		return applyAwsProviderPatch(terragruntOptions)
	}

	if err := checkProtectedModule(terragruntOptions, terragruntConfig); err != nil {
		return err
	}

	return runActionWithHooks("terraform", terragruntOptions, terragruntConfig, func() error {
		runTerraformError := runTerraformWithRetry(terragruntOptions)

		var lockFileError error
		if shouldCopyLockFile(terragruntOptions.TerraformCliArgs) {
			// Copy the lock file from the Terragrunt working dir (e.g., .terragrunt-cache/xxx/<some-module>) to the
			// user's working dir (e.g., /live/stage/vpc). That way, the lock file will end up right next to the user's
			// terragrunt.hcl and can be checked into version control. Note that in the past, Terragrunt allowed the
			// user's working dir to be different than the directory where the terragrunt.hcl file lived, so just in
			// case, we are using the user's working dir here, rather than just looking at the parent dir of the
			// terragrunt.hcl. However, the default value for the user's working dir, set in options.go, IS just the
			// parent dir of terragrunt.hcl, so these will likely always be the same.
			lockFileError = util.CopyLockFile(terragruntOptions.WorkingDir, originalTerragruntOptions.WorkingDir, terragruntOptions.Logger)
		}

		return multierror.Append(runTerraformError, lockFileError).ErrorOrNil()
	})
}

// confirmActionWithDependentModules - Show warning with list of dependent modules from current module before destroy
func confirmActionWithDependentModules(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) bool {
	modules := configstack.FindWhereWorkingDirIsIncluded(terragruntOptions, terragruntConfig)
	if len(modules) != 0 {
		if _, err := terragruntOptions.ErrWriter.Write([]byte("Detected dependent modules:\n")); err != nil {
			terragruntOptions.Logger.Error(err)
			return false
		}
		for _, module := range modules {
			if _, err := terragruntOptions.ErrWriter.Write([]byte(fmt.Sprintf("%s\n", module.Path))); err != nil {
				terragruntOptions.Logger.Error(err)
				return false
			}
		}
		prompt := "WARNING: Are you sure you want to continue?"
		shouldRun, err := shell.PromptUserForYesNo(prompt, terragruntOptions)
		if err != nil {
			terragruntOptions.Logger.Error(err)
			return false
		}
		return shouldRun
	}
	// request user to confirm action in any case
	return true
}

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
func shouldCopyLockFile(args []string) bool {
	if util.FirstArg(args) == CmdInit {
		return true
	}

	if util.FirstArg(args) == CmdProviders && util.SecondArg(args) == CmdLock {
		return true
	}
	return false
}

// Run the given action function surrounded by hooks. That is, run the before hooks first, then, if there were no
// errors, run the action, and finally, run the after hooks. Return any errors hit from the hooks or action.
func runActionWithHooks(description string, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, action func() error) error {
	var allErrors *multierror.Error
	beforeHookErrors := processHooks(terragruntConfig.Terraform.GetBeforeHooks(), terragruntOptions, terragruntConfig, allErrors)
	allErrors = multierror.Append(allErrors, beforeHookErrors)

	var actionErrors error
	if beforeHookErrors == nil {
		actionErrors = action()
		allErrors = multierror.Append(allErrors, actionErrors)
	} else {
		terragruntOptions.Logger.Errorf("Errors encountered running before_hooks. Not running '%s'.", description)
	}
	postHookErrors := processHooks(terragruntConfig.Terraform.GetAfterHooks(), terragruntOptions, terragruntConfig, allErrors)
	errorHookErrors := processErrorHooks(terragruntConfig.Terraform.GetErrorHooks(), terragruntOptions, allErrors)
	allErrors = multierror.Append(allErrors, postHookErrors, errorHookErrors)

	return allErrors.ErrorOrNil()
}

// The Terragrunt configuration can contain a set of inputs to pass to Terraform as environment variables. This method
// sets these environment variables in the given terragruntOptions.
func setTerragruntInputsAsEnvVars(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	asEnvVars, err := toTerraformEnvVars(terragruntConfig.Inputs)
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

func runTerraformWithRetry(terragruntOptions *options.TerragruntOptions) error {
	// Retry the command configurable time with sleep in between
	for i := 0; i < terragruntOptions.RetryMaxAttempts; i++ {
		if out, tferr := shell.RunTerraformCommandWithOutput(terragruntOptions, terragruntOptions.TerraformCliArgs...); tferr != nil {
			if out != nil && isRetryable(out.Stdout, out.Stderr, tferr, terragruntOptions) {
				terragruntOptions.Logger.Infof("Encountered an error eligible for retrying. Sleeping %v before retrying.\n", terragruntOptions.RetrySleepIntervalSec)
				time.Sleep(terragruntOptions.RetrySleepIntervalSec)
			} else {
				terragruntOptions.Logger.Errorf("Terraform invocation failed in %s", terragruntOptions.WorkingDir)
				return tferr
			}
		} else {
			return nil
		}
	}

	return errors.WithStackTrace(MaxRetriesExceeded{terragruntOptions})
}

// Prepare for running 'terraform init' by initializing remote state storage and adding backend configuration arguments
// to the TerraformCliArgs
func prepareInitCommand(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, allowSourceDownload bool) error {
	if terragruntConfig.RemoteState != nil {
		// Initialize the remote state if necessary  (e.g. create S3 bucket and DynamoDB table)
		remoteStateNeedsInit, err := remoteStateNeedsInit(terragruntConfig.RemoteState, terragruntOptions)
		if err != nil {
			return err
		}
		if remoteStateNeedsInit {
			if err := terragruntConfig.RemoteState.Initialize(terragruntOptions); err != nil {
				return err
			}
		}

		// Add backend config arguments to the command
		terragruntOptions.InsertTerraformCliArgs(terragruntConfig.RemoteState.ToTerraformInitArgs()...)
	}
	return nil
}

func checkFolderContainsTerraformCode(terragruntOptions *options.TerragruntOptions) error {
	files := []string{}
	hclFiles, err := zglob.Glob(fmt.Sprintf("%s/**/*.tf", terragruntOptions.WorkingDir))
	if err != nil {
		return errors.WithStackTrace(err)
	}
	files = append(files, hclFiles...)

	jsonFiles, err := zglob.Glob(fmt.Sprintf("%s/**/*.tf.json", terragruntOptions.WorkingDir))
	if err != nil {
		return errors.WithStackTrace(err)
	}
	files = append(files, jsonFiles...)

	if len(files) == 0 {
		return errors.WithStackTrace(NoTerraformFilesFound(terragruntOptions.WorkingDir))
	}

	return nil
}

// Check that the specified Terraform code defines a backend { ... } block and return an error if doesn't
func checkTerraformCodeDefinesBackend(terragruntOptions *options.TerragruntOptions, backendType string) error {
	terraformBackendRegexp, err := regexp.Compile(fmt.Sprintf(`backend[[:blank:]]+"%s"`, backendType))
	if err != nil {
		return errors.WithStackTrace(err)
	}

	definesBackend, err := util.Grep(terraformBackendRegexp, fmt.Sprintf("%s/**/*.tf", terragruntOptions.WorkingDir))
	if err != nil {
		return err
	}
	if definesBackend {
		return nil
	}

	terraformJSONBackendRegexp, err := regexp.Compile(fmt.Sprintf(`(?m)"backend":[[:space:]]*{[[:space:]]*"%s"`, backendType))
	if err != nil {
		return errors.WithStackTrace(err)
	}

	definesJSONBackend, err := util.Grep(terraformJSONBackendRegexp, fmt.Sprintf("%s/**/*.tf.json", terragruntOptions.WorkingDir))
	if err != nil {
		return err
	}
	if definesJSONBackend {
		return nil
	}

	return errors.WithStackTrace(BackendNotDefined{Opts: terragruntOptions, BackendType: backendType})
}

// Prepare for running any command other than 'terraform init' by running 'terraform init' if necessary
// This function takes in the "original" terragrunt options which has the unmodified 'WorkingDir' from before downloading the code from the source URL,
// and the "updated" terragrunt options that will contain the updated 'WorkingDir' into which the code has been downloaded
func prepareNonInitCommand(originalTerragruntOptions *options.TerragruntOptions, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	needsInit, err := needsInit(terragruntOptions, terragruntConfig)
	if err != nil {
		return err
	}

	if needsInit {
		if err := runTerraformInit(originalTerragruntOptions, terragruntOptions, terragruntConfig, nil); err != nil {
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
	providersPath013 := util.JoinPath(terragruntOptions.DataDir(), "plugins")
	providersPath014 := util.JoinPath(terragruntOptions.DataDir(), "providers")
	return !(util.FileExists(providersPath013) || util.FileExists(providersPath014))
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
func runTerraformInit(originalTerragruntOptions *options.TerragruntOptions, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, terraformSource *terraform.Source) error {

	// Prevent Auto-Init if the user has disabled it
	if util.FirstArg(terragruntOptions.TerraformCliArgs) != CmdInit && !terragruntOptions.AutoInit {
		terragruntOptions.Logger.Warnf("Detected that init is needed, but Auto-Init is disabled. Continuing with further actions, but subsequent terraform commands may fail.")
		return nil
	}

	initOptions, err := prepareInitOptions(terragruntOptions, terraformSource)

	if err != nil {
		return err
	}

	err = runTerragruntWithConfig(originalTerragruntOptions, initOptions, terragruntConfig, terraformSource != nil)
	if err != nil {
		return err
	}

	moduleNeedInit := util.JoinPath(terragruntOptions.WorkingDir, moduleInitRequiredFile)
	if util.FileExists(moduleNeedInit) {
		return os.Remove(moduleNeedInit)
	}
	return nil
}

func prepareInitOptions(terragruntOptions *options.TerragruntOptions, terraformSource *terraform.Source) (*options.TerragruntOptions, error) {
	// Need to clone the terragruntOptions, so the TerraformCliArgs can be configured to run the init command
	initOptions := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	initOptions.TerraformCliArgs = []string{CmdInit}
	initOptions.WorkingDir = terragruntOptions.WorkingDir
	initOptions.TerraformCommand = CmdInit

	initOutputForCommands := []string{CmdPlan, CmdApply}
	terraformCommand := util.FirstArg(terragruntOptions.TerraformCliArgs)
	if !collections.ListContainsElement(initOutputForCommands, terraformCommand) {
		// Since some command can return a json string, it is necessary to suppress output to stdout of the `terraform init` command.
		initOptions.Writer = io.Discard
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
	moduleNeedInit := util.JoinPath(terragruntOptions.WorkingDir, moduleInitRequiredFile)
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
	if util.FirstArg(terragruntOptions.TerraformCliArgs) == "destroy" {
		destroyFlag = true
	}
	if util.ListContainsElement(terragruntOptions.TerraformCliArgs, "-destroy") {
		destroyFlag = true
	}
	if !destroyFlag {
		return nil
	}
	if terragruntConfig.PreventDestroy != nil && *terragruntConfig.PreventDestroy {
		return errors.WithStackTrace(ModuleIsProtected{Opts: terragruntOptions})
	}
	return nil
}

// isRetryable checks whether there was an error and if the output matches any of the configured RetryableErrors
func isRetryable(stdout string, stderr string, tferr error, terragruntOptions *options.TerragruntOptions) bool {
	if !terragruntOptions.AutoRetry || tferr == nil {
		return false
	}
	// When -json is enabled, Terraform will send all output, errors included, to stdout.
	return util.MatchesAny(terragruntOptions.RetryableErrors, stderr) || util.MatchesAny(terragruntOptions.RetryableErrors, stdout)
}

func filterTerraformExtraArgs(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) []string {
	out := []string{}
	cmd := util.FirstArg(terragruntOptions.TerraformCliArgs)

	for _, arg := range terragruntConfig.Terraform.ExtraArgs {
		for _, arg_cmd := range arg.Commands {
			if cmd == arg_cmd {
				lastArg := util.LastArg(terragruntOptions.TerraformCliArgs)
				skipVars := (cmd == "apply" || cmd == "destroy") && util.IsFile(lastArg)

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
						out = append(out, fmt.Sprintf("-var-file=%s", file))
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

// Convert the given variables to a map of environment variables that will expose those variables to Terraform. The
// keys will be of the format TF_VAR_xxx and the values will be converted to JSON, which Terraform knows how to read
// natively.
func toTerraformEnvVars(vars map[string]interface{}) (map[string]string, error) {
	out := map[string]string{}

	for varName, varValue := range vars {
		envVarName := fmt.Sprintf("%s_%s", terraform.TFVarPrefix, varName)

		envVarValue, err := util.AsTerraformEnvVarJsonValue(varValue)
		if err != nil {
			return nil, err
		}

		out[envVarName] = string(envVarValue)
	}

	return out, nil
}

// Custom error types

type UnrecognizedCommand string

func (commandName UnrecognizedCommand) Error() string {
	return fmt.Sprintf("Unrecognized command: %s", string(commandName))
}

type ArgumentNotAllowed struct {
	Argument string
	Message  string
}

func (err ArgumentNotAllowed) Error() string {
	return fmt.Sprintf(err.Message, err.Argument)
}

type BackendNotDefined struct {
	Opts        *options.TerragruntOptions
	BackendType string
}

func (err BackendNotDefined) Error() string {
	return fmt.Sprintf("Found remote_state settings in %s but no backend block in the Terraform code in %s. You must define a backend block (it can be empty!) in your Terraform code or your remote state settings will have no effect! It should look something like this:\n\nterraform {\n  backend \"%s\" {}\n}\n\n", err.Opts.TerragruntConfigPath, err.Opts.WorkingDir, err.BackendType)
}

type NoTerraformFilesFound string

func (path NoTerraformFilesFound) Error() string {
	return fmt.Sprintf("Did not find any Terraform files (*.tf) in %s", string(path))
}

type ModuleIsProtected struct {
	Opts *options.TerragruntOptions
}

func (err ModuleIsProtected) Error() string {
	return fmt.Sprintf("Module is protected by the prevent_destroy flag in %s. Set it to false or delete it to allow destroying of the module.", err.Opts.TerragruntConfigPath)
}

type MaxRetriesExceeded struct {
	Opts *options.TerragruntOptions
}

func (err MaxRetriesExceeded) Error() string {
	return fmt.Sprintf("Exhausted retries (%v) for command %v %v", err.Opts.RetryMaxAttempts, err.Opts.TerraformPath, strings.Join(err.Opts.TerraformCliArgs, " "))
}
