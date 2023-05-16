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

	"github.com/gruntwork-io/terragrunt/tflint"

	"github.com/gruntwork-io/gruntwork-cli/collections"
	"github.com/hashicorp/go-multierror"
	"github.com/mattn/go-zglob"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/cli/tfsource"
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
	optTerragruntConfig                         = "terragrunt-config"
	optTerragruntTFPath                         = "terragrunt-tfpath"
	optTerragruntNoAutoInit                     = "terragrunt-no-auto-init"
	optTerragruntNoAutoRetry                    = "terragrunt-no-auto-retry"
	optTerragruntNoAutoApprove                  = "terragrunt-no-auto-approve"
	optNonInteractive                           = "terragrunt-non-interactive"
	optWorkingDir                               = "terragrunt-working-dir"
	optDownloadDir                              = "terragrunt-download-dir"
	optTerragruntSource                         = "terragrunt-source"
	optTerragruntSourceMap                      = "terragrunt-source-map"
	optTerragruntSourceUpdate                   = "terragrunt-source-update"
	optTerragruntIAMRole                        = "terragrunt-iam-role"
	optTerragruntIAMAssumeRoleDuration          = "terragrunt-iam-assume-role-duration"
	optTerragruntIAMAssumeRoleSessionName       = "terragrunt-iam-assume-role-session-name"
	optTerragruntIgnoreDependencyErrors         = "terragrunt-ignore-dependency-errors"
	optTerragruntIgnoreDependencyOrder          = "terragrunt-ignore-dependency-order"
	optTerragruntIgnoreExternalDependencies     = "terragrunt-ignore-external-dependencies"
	optTerragruntIncludeExternalDependencies    = "terragrunt-include-external-dependencies"
	optTerragruntExcludeDir                     = "terragrunt-exclude-dir"
	optTerragruntIncludeDir                     = "terragrunt-include-dir"
	optTerragruntStrictInclude                  = "terragrunt-strict-include"
	optTerragruntParallelism                    = "terragrunt-parallelism"
	optTerragruntCheck                          = "terragrunt-check"
	optTerragruntHCLFmt                         = "terragrunt-hclfmt-file"
	optTerragruntDebug                          = "terragrunt-debug"
	optTerragruntOverrideAttr                   = "terragrunt-override-attr"
	optTerragruntLogLevel                       = "terragrunt-log-level"
	optTerragruntStrictValidate                 = "terragrunt-strict-validate"
	optTerragruntJSONOut                        = "terragrunt-json-out"
	optTerragruntModulesThatInclude             = "terragrunt-modules-that-include"
	optTerragruntFetchDependencyOutputFromState = "terragrunt-fetch-dependency-output-from-state"
	optTerragruntUsePartialParseConfigCache     = "terragrunt-use-partial-parse-config-cache"
	optTerragruntIncludeModulePrefix            = "terragrunt-include-module-prefix"
	optTerragruntOutputWithMetadata             = "with-metadata"
)

var allTerragruntBooleanOpts = []string{
	optNonInteractive,
	optTerragruntSourceUpdate,
	optTerragruntIgnoreDependencyErrors,
	optTerragruntIgnoreDependencyOrder,
	optTerragruntIgnoreExternalDependencies,
	optTerragruntIncludeExternalDependencies,
	optTerragruntNoAutoInit,
	optTerragruntNoAutoRetry,
	optTerragruntNoAutoApprove,
	optTerragruntCheck,
	optTerragruntStrictInclude,
	optTerragruntDebug,
	optTerragruntFetchDependencyOutputFromState,
	optTerragruntUsePartialParseConfigCache,
	optTerragruntOutputWithMetadata,
	optTerragruntIncludeModulePrefix,
}
var allTerragruntStringOpts = []string{
	optTerragruntConfig,
	optTerragruntTFPath,
	optWorkingDir,
	optDownloadDir,
	optTerragruntSource,
	optTerragruntSourceMap,
	optTerragruntIAMRole,
	optTerragruntIAMAssumeRoleDuration,
	optTerragruntIAMAssumeRoleSessionName,
	optTerragruntExcludeDir,
	optTerragruntIncludeDir,
	optTerragruntParallelism,
	optTerragruntHCLFmt,
	optTerragruntOverrideAttr,
	optTerragruntLogLevel,
	optTerragruntStrictValidate,
	optTerragruntJSONOut,
	optTerragruntModulesThatInclude,
}

const (
	CMD_INIT                          = "init"
	CMD_INIT_FROM_MODULE              = "init-from-module"
	CMD_PLAN                          = "plan"
	CMD_APPLY                         = "apply"
	CMD_PROVIDERS                     = "providers"
	CMD_LOCK                          = "lock"
	CMD_TERRAGRUNT_INFO               = "terragrunt-info"
	CMD_TERRAGRUNT_VALIDATE_INPUTS    = "validate-inputs"
	CMD_TERRAGRUNT_GRAPH_DEPENDENCIES = "graph-dependencies"
	CMD_TERRAGRUNT_READ_CONFIG        = "terragrunt-read-config"
	CMD_HCLFMT                        = "hclfmt"
	CMD_AWS_PROVIDER_PATCH            = "aws-provider-patch"
	CMD_RENDER_JSON                   = "render-json"
)

// START: Constants useful for multimodule command handling
const CMD_RUN_ALL = "run-all"

// Known terraform commands that are explicitly not supported in run-all due to the nature of the command. This is
// tracked as a map that maps the terraform command to the reasoning behind disallowing the command in run-all.
var runAllDisabledCommands = map[string]string{
	"import":       "terraform import should only be run against a single state representation to avoid injecting the wrong object in the wrong state representation.",
	"taint":        "terraform taint should only be run against a single state representation to avoid using the wrong state address.",
	"untaint":      "terraform untaint should only be run against a single state representation to avoid using the wrong state address.",
	"console":      "terraform console requires stdin, which is shared across all instances of run-all when multiple modules run concurrently.",
	"force-unlock": "lock IDs are unique per state representation and thus should not be run with run-all.",

	// MAINTAINER'S NOTE: There are a few other commands that might not make sense, but we deliberately allow it for
	// certain use cases that are documented here:
	// - state          : Supporting `state` with run-all could be useful for a mass pull and push operation, which can
	//                    be done en masse with the use of relative pathing.
	// - login / logout : Supporting `login` with run-all could be useful when used in conjunction with tfenv and
	//                    multi-terraform version setups, where multiple terraform versions need to be configured.
	// - version        : Supporting `version` with run-all could be useful for sanity checking a multi-version setup.
}

var MULTI_MODULE_COMMANDS = []string{
	CMD_RUN_ALL,

	// The rest of the commands are deprecated, and are only here for legacy reasons to ensure that terragrunt knows to
	// filter them out during arg parsing.
	CMD_APPLY_ALL,
	CMD_DESTROY_ALL,
	CMD_OUTPUT_ALL,
	CMD_PLAN_ALL,
	CMD_VALIDATE_ALL,
}

// END: Constants useful for multimodule command handling

// The following commands are DEPRECATED
const (
	CMD_SPIN_UP      = "spin-up"
	CMD_TEAR_DOWN    = "tear-down"
	CMD_PLAN_ALL     = "plan-all"
	CMD_APPLY_ALL    = "apply-all"
	CMD_DESTROY_ALL  = "destroy-all"
	CMD_OUTPUT_ALL   = "output-all"
	CMD_VALIDATE_ALL = "validate-all"
)

// deprecatedCommands is a map of deprecated commands to a handler that knows how to convert the command to the known
// alternative. The handler should return the new TerragruntOptions (if any modifications are needed) and command
// string.
var deprecatedCommands = map[string]func(origOptions *options.TerragruntOptions) (*options.TerragruntOptions, string, string){
	CMD_SPIN_UP:      spinUpDeprecationHandler,
	CMD_TEAR_DOWN:    tearDownDeprecationHandler,
	CMD_APPLY_ALL:    applyAllDeprecationHandler,
	CMD_DESTROY_ALL:  destroyAllDeprecationHandler,
	CMD_PLAN_ALL:     planAllDeprecationHandler,
	CMD_VALIDATE_ALL: validateAllDeprecationHandler,
	CMD_OUTPUT_ALL:   outputAllDeprecationHandler,
}

var TERRAFORM_COMMANDS_THAT_USE_STATE = []string{
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

var TERRAFORM_COMMANDS_THAT_DO_NOT_NEED_INIT = []string{
	"version",
	"terragrunt-info",
	"graph-dependencies",
}

// deprecatedArguments is a map of deprecated arguments to the argument that replace them.
var deprecatedArguments = map[string]string{}

// Struct is output as JSON by 'terragrunt-info':
type TerragruntInfoGroup struct {
	ConfigPath       string
	DownloadDir      string
	IamRole          string
	TerraformBinary  string
	TerraformCommand string
	WorkingDir       string
}

// Since Terragrunt is just a thin wrapper for Terraform, and we don't want to repeat every single Terraform command
// in its definition, we don't quite fit into the model of any Go CLI library. Fortunately, urfave/cli allows us to
// override the whole template used for the Usage Text.
//
// TODO: this description text has copy/pasted versions of many Terragrunt constants, such as command names and file
// names. It would be easy to make this code DRY using fmt.Sprintf(), but then it's hard to make the text align nicely.
// Write some code to take generate this help text automatically, possibly leveraging code that's part of urfave/cli.
var CUSTOM_USAGE_TEXT = `DESCRIPTION:
   {{.Name}} - {{.UsageText}}

USAGE:
   {{.Usage}}

COMMANDS:
   run-all               Run a terraform command against a 'stack' by running the specified command in each subfolder. E.g., to run 'terragrunt apply' in each subfolder, use 'terragrunt run-all apply'.
   terragrunt-info       Emits limited terragrunt state on stdout and exits
   validate-inputs       Checks if the terragrunt configured inputs align with the terraform defined variables.
   graph-dependencies    Prints the terragrunt dependency graph to stdout
   hclfmt                Recursively find hcl files and rewrite them into a canonical format.
   aws-provider-patch    Overwrite settings on nested AWS providers to work around a Terraform bug (issue #13018)
   render-json           Render the final terragrunt config, with all variables, includes, and functions resolved, as json. This is useful for enforcing policies using static analysis tools like Open Policy Agent, or for debugging your terragrunt config.
   *                     Terragrunt forwards all other commands directly to Terraform

GLOBAL OPTIONS:
   terragrunt-config                            Path to the Terragrunt config file. Default is terragrunt.hcl.
   terragrunt-tfpath                            Path to the Terraform binary. Default is terraform (on PATH).
   terragrunt-no-auto-init                      Don't automatically run 'terraform init' during other terragrunt commands. You must run 'terragrunt init' manually.
   terragrunt-no-auto-retry                     Don't automatically re-run command in case of transient errors.
   terragrunt-non-interactive                   Assume "yes" for all prompts.
   terragrunt-working-dir                       The path to the Terraform templates. Default is current directory.
   terragrunt-download-dir                      The path where to download Terraform code. Default is .terragrunt-cache in the working directory.
   terragrunt-source                            Download Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder.
   terragrunt-source-update                     Delete the contents of the temporary folder to clear out any old, cached source code before downloading new source code into it.
   terragrunt-iam-role                          Assume the specified IAM role before executing Terraform. Can also be set via the TERRAGRUNT_IAM_ROLE environment variable.
   terragrunt-iam-assume-role-duration          Session duration for IAM Assume Role session. Can also be set via the TERRAGRUNT_IAM_ASSUME_ROLE_DURATION environment variable.
   terragrunt-iam-assume-role-session-name      Name for the IAM Assummed Role session. Can also be set via TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME environment variable.
   terragrunt-ignore-dependency-errors          *-all commands continue processing components even if a dependency fails.
   terragrunt-ignore-dependency-order           *-all commands will be run disregarding the dependencies
   terragrunt-ignore-external-dependencies      *-all commands will not attempt to include external dependencies
   terragrunt-include-external-dependencies     *-all commands will include external dependencies
   terragrunt-parallelism <N>                   *-all commands parallelism set to at most N modules
   terragrunt-exclude-dir                       Unix-style glob of directories to exclude when running *-all commands
   terragrunt-include-dir                       Unix-style glob of directories to include when running *-all commands
   terragrunt-check                             Enable check mode in the hclfmt command.
   terragrunt-hclfmt-file                       The path to a single hcl file that the hclfmt command should run on.
   terragrunt-override-attr                     A key=value attribute to override in a provider block as part of the aws-provider-patch command. May be specified multiple times.
   terragrunt-debug                             Write terragrunt-debug.tfvars to working folder to help root-cause issues.
   terragrunt-log-level                         Sets the logging level for Terragrunt. Supported levels: panic, fatal, error, warn (default), info, debug, trace.
   terragrunt-strict-validate                   Sets strict mode for the validate-inputs command. By default, strict mode is off. When this flag is passed, strict mode is turned on. When strict mode is turned off, the validate-inputs command will only return an error if required inputs are missing from all input sources (env vars, var files, etc). When strict mode is turned on, an error will be returned if required inputs are missing OR if unused variables are passed to Terragrunt.
   terragrunt-json-out                          The file path that terragrunt should use when rendering the terragrunt.hcl config as json. Only used in the render-json command. Defaults to terragrunt_rendered.json.
   terragrunt-use-partial-parse-config-cache    Enables caching of includes during partial parsing operations. Will also be used for the --terragrunt-iam-role option if provided.
   terragrunt-include-module-prefix             When this flag is set output from Terraform sub-commands is prefixed with module path.

VERSION:
   {{.Version}}{{if len .Authors}}

AUTHOR(S):
   {{range .Authors}}{{.}}{{end}}
   {{end}}
`

var MODULE_REGEX = regexp.MustCompile(`module[[:blank:]]+".+"`)

// This uses the constraint syntax from https://github.com/hashicorp/go-version
// This version of Terragrunt was tested to work with Terraform 0.12.0 and above only
const DEFAULT_TERRAFORM_VERSION_CONSTRAINT = ">= v0.12.0"

const TERRAFORM_EXTENSION_GLOB = "*.tf"

// Prefix to use for terraform variables set with environment variables.
const TFVarPrefix = "TF_VAR"

// The supported flags to show help of terraform commands
var TERRAFORM_HELP_FLAGS = []string{
	"--help",
	"-help",
	"-h",
}

// map of help functions for each terragrunt command
var terragruntHelp = map[string]string{
	CMD_RENDER_JSON:                renderJsonHelp,
	CMD_AWS_PROVIDER_PATCH:         awsProviderPatchHelp,
	CMD_TERRAGRUNT_VALIDATE_INPUTS: validateInputsHelp,
}

// sourceChangeLocks is a map that keeps track of locks for source changes, to ensure we aren't overriding the generated
// code while another hook (e.g. `tflint`) is running. We use sync.Map to ensure atomic updates during concurrent access.
var sourceChangeLocks = sync.Map{}

// Create the Terragrunt CLI App
func CreateTerragruntCli(version string, writer io.Writer, errwriter io.Writer) *cli.App {
	cli.OsExiter = func(exitCode int) {
		// Do nothing. We just need to override this function, as the default value calls os.Exit, which
		// kills the app (or any automated test) dead in its tracks.
	}

	cli.AppHelpTemplate = CUSTOM_USAGE_TEXT

	app := cli.NewApp()

	app.Name = "terragrunt"
	app.Author = "Gruntwork <www.gruntwork.io>"
	app.Version = version
	app.Action = runApp
	app.Usage = "terragrunt <COMMAND> [GLOBAL OPTIONS]"
	app.Writer = writer
	app.ErrWriter = errwriter
	app.UsageText = `Terragrunt is a thin wrapper for Terraform that provides extra tools for working with multiple
   Terraform modules, remote state, and locking. For documentation, see https://github.com/gruntwork-io/terragrunt/.`

	return app
}

// The sole action for the app
func runApp(cliContext *cli.Context) (finalErr error) {
	defer errors.Recover(func(cause error) { finalErr = cause })

	// If someone calls us with no args at all, show the help text and exit
	if !cliContext.Args().Present() {
		return cli.ShowAppHelp(cliContext)
	}

	terragruntOptions, err := ParseTerragruntOptions(cliContext)
	if err != nil {
		return err
	}

	shell.PrepareConsole(terragruntOptions)

	givenCommand := cliContext.Args().First()
	newOptions, command := checkDeprecated(givenCommand, terragruntOptions)
	return runCommand(command, newOptions)
}

// checkDeprecated checks if the given command is deprecated.  If so: prints a message and returns the new command.
func checkDeprecated(command string, terragruntOptions *options.TerragruntOptions) (*options.TerragruntOptions, string) {
	deprecationHandler, deprecated := deprecatedCommands[command]
	if deprecated {
		newOptions, newCommand, newCommandFriendly := deprecationHandler(terragruntOptions)
		terragruntOptions.Logger.Warnf(
			"'%s' is deprecated. Running '%s' instead. Please update your workflows to use '%s', as '%s' may be removed in the future!\n",
			command,
			newCommandFriendly,
			newCommandFriendly,
			command,
		)
		return newOptions, newCommand
	}
	return terragruntOptions, command
}

// runCommand runs one or many terraform commands based on the type of
// terragrunt command
func runCommand(command string, terragruntOptions *options.TerragruntOptions) (finalEff error) {
	if command == CMD_RUN_ALL {
		return runAll(terragruntOptions)
	}
	if command == "destroy" {
		terragruntOptions.CheckDependentModules = true
	}
	return RunTerragrunt(terragruntOptions)
}

// Downloads terraform source if necessary, then runs terraform with the given options and CLI args.
// This will forward all the args and extra_arguments directly to Terraform.
func RunTerragrunt(terragruntOptions *options.TerragruntOptions) error {
	if shouldPrintTerragruntHelp(terragruntOptions) {
		helpMessage, _ := terragruntHelp[terragruntOptions.TerraformCommand]
		_, err := fmt.Fprintf(terragruntOptions.Writer, "%s\n", helpMessage)
		return err
	}
	if shouldPrintTerraformHelp(terragruntOptions) {
		return shell.RunTerraformCommand(terragruntOptions, terragruntOptions.TerraformCliArgs...)
	}

	if shouldRunHCLFmt(terragruntOptions) {
		return runHCLFmt(terragruntOptions)
	}

	if shouldRunGraphDependencies(terragruntOptions) {
		return runGraphDependencies(terragruntOptions)
	}

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
	terragruntOptionsClone.TerraformCommand = CMD_TERRAGRUNT_READ_CONFIG

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

	terraformVersionConstraint := DEFAULT_TERRAFORM_VERSION_CONSTRAINT
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

// Run graph dependencies prints the dependency graph to stdout
func runGraphDependencies(terragruntOptions *options.TerragruntOptions) error {
	stack, err := configstack.FindStackInSubfolders(terragruntOptions)
	if err != nil {
		return err
	}

	// Exit early if the operation wanted is to get the graph
	stack.Graph(terragruntOptions)
	return nil
}

func shouldPrintTerraformHelp(terragruntOptions *options.TerragruntOptions) bool {
	for _, tfHelpFlag := range TERRAFORM_HELP_FLAGS {
		if util.ListContainsElement(terragruntOptions.TerraformCliArgs, tfHelpFlag) {
			return true
		}
	}
	return false
}

func shouldPrintTerragruntHelp(terragruntOptions *options.TerragruntOptions) bool {
	// check if command is in help map
	_, found := terragruntHelp[terragruntOptions.TerraformCommand]
	if !found {
		return false
	}

	for _, tfHelpFlag := range TERRAFORM_HELP_FLAGS {
		if util.ListContainsElement(terragruntOptions.TerraformCliArgs, tfHelpFlag) {
			return true
		}
	}
	return false
}

func shouldRunGraphDependencies(terragruntOptions *options.TerragruntOptions) bool {
	return util.ListContainsElement(terragruntOptions.TerraformCliArgs, CMD_TERRAGRUNT_GRAPH_DEPENDENCIES)
}

func shouldPrintTerragruntInfo(terragruntOptions *options.TerragruntOptions) bool {
	return util.ListContainsElement(terragruntOptions.TerraformCliArgs, CMD_TERRAGRUNT_INFO)
}

func shouldValidateTerragruntInputs(terragruntOptions *options.TerragruntOptions) bool {
	return util.ListContainsElement(terragruntOptions.TerraformCliArgs, CMD_TERRAGRUNT_VALIDATE_INPUTS)
}

func shouldRunHCLFmt(terragruntOptions *options.TerragruntOptions) bool {
	return util.ListContainsElement(terragruntOptions.TerraformCliArgs, CMD_HCLFMT)
}

func shouldRunRenderJSON(terragruntOptions *options.TerragruntOptions) bool {
	return util.ListContainsElement(terragruntOptions.TerraformCliArgs, CMD_RENDER_JSON)
}

func shouldApplyAwsProviderPatch(terragruntOptions *options.TerragruntOptions) bool {
	return util.ListContainsElement(terragruntOptions.TerraformCliArgs, CMD_AWS_PROVIDER_PATCH)
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

			actionToExecute := curHook.Execute[0]
			actionParams := curHook.Execute[1:]
			_, possibleError := shell.RunShellCommandWithOutput(
				terragruntOptions,
				workingDir,
				false,
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

	rawActualLock, _ := sourceChangeLocks.LoadOrStore(workingDir, &sync.Mutex{})
	actualLock := rawActualLock.(*sync.Mutex)
	actualLock.Lock()
	defer actualLock.Unlock()

	actionToExecute := curHook.Execute[0]
	actionParams := curHook.Execute[1:]

	if actionToExecute == "tflint" {
		err := tflint.RunTflintWithOpts(terragruntOptions, terragruntConfig)
		if err != nil {
			terragruntOptions.Logger.Errorf("Error running hook %s with message: %s", curHook.Name, err.Error())
			return err
		}
	} else {
		_, possibleError := shell.RunShellCommandWithOutput(
			terragruntOptions,
			workingDir,
			false,
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

	if util.FirstArg(terragruntOptions.TerraformCliArgs) == CMD_INIT {
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
			lockFileError = copyLockFile(terragruntOptions.WorkingDir, originalTerragruntOptions.WorkingDir, terragruntOptions.Logger)
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
	if util.FirstArg(args) == CMD_INIT {
		return true
	}

	if util.FirstArg(args) == CMD_PROVIDERS && util.SecondArg(args) == CMD_LOCK {
		return true
	}
	return false
}

// Terraform 0.14 now generates a lock file when you run `terraform init`.
// If any such file exists, this function will copy the lock file to the destination folder
func copyLockFile(sourceFolder string, destinationFolder string, logger *logrus.Entry) error {
	sourceLockFilePath := util.JoinPath(sourceFolder, util.TerraformLockFile)
	destinationLockFilePath := util.JoinPath(destinationFolder, util.TerraformLockFile)

	if util.FileExists(sourceLockFilePath) {
		logger.Debugf("Copying lock file from %s to %s", sourceLockFilePath, destinationFolder)
		return util.CopyFile(sourceLockFilePath, destinationLockFilePath)
	}

	return nil
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
	if util.ListContainsElement(TERRAFORM_COMMANDS_THAT_DO_NOT_NEED_INIT, util.FirstArg(terragruntOptions.TerraformCliArgs)) {
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
func runTerraformInit(originalTerragruntOptions *options.TerragruntOptions, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, terraformSource *tfsource.TerraformSource) error {

	// Prevent Auto-Init if the user has disabled it
	if util.FirstArg(terragruntOptions.TerraformCliArgs) != CMD_INIT && !terragruntOptions.AutoInit {
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

func prepareInitOptions(terragruntOptions *options.TerragruntOptions, terraformSource *tfsource.TerraformSource) (*options.TerragruntOptions, error) {
	// Need to clone the terragruntOptions, so the TerraformCliArgs can be configured to run the init command
	initOptions := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	initOptions.TerraformCliArgs = []string{CMD_INIT}
	initOptions.WorkingDir = terragruntOptions.WorkingDir
	initOptions.TerraformCommand = CMD_INIT

	initOutputForCommands := []string{CMD_PLAN, CMD_APPLY}
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

	return util.Grep(MODULE_REGEX, fmt.Sprintf("%s/%s", terragruntOptions.WorkingDir, TERRAFORM_EXTENSION_GLOB))
}

// If the user entered a Terraform command that uses state (e.g. plan, apply), make sure remote state is configured
// before running the command.
func remoteStateNeedsInit(remoteState *remote.RemoteState, terragruntOptions *options.TerragruntOptions) (bool, error) {

	// We only configure remote state for the commands that use the tfstate files. We do not configure it for
	// commands such as "get" or "version".
	if remoteState != nil && util.ListContainsElement(TERRAFORM_COMMANDS_THAT_USE_STATE, util.FirstArg(terragruntOptions.TerraformCliArgs)) {
		return remoteState.NeedsInit(terragruntOptions)
	}
	return false, nil
}

// runAll runs the provided terraform command against all the modules that are found in the directory tree.
func runAll(terragruntOptions *options.TerragruntOptions) error {
	if terragruntOptions.TerraformCommand == "" {
		return MissingCommand{}
	}
	reason, isDisabled := runAllDisabledCommands[terragruntOptions.TerraformCommand]
	if isDisabled {
		return RunAllDisabledErr{
			command: terragruntOptions.TerraformCommand,
			reason:  reason,
		}
	}

	stack, err := configstack.FindStackInSubfolders(terragruntOptions)
	if err != nil {
		return err
	}

	terragruntOptions.Logger.Debugf("%s", stack.String())
	if err := stack.LogModuleDeployOrder(terragruntOptions.Logger, terragruntOptions.TerraformCommand); err != nil {
		return err
	}

	var prompt string
	switch terragruntOptions.TerraformCommand {
	case "apply":
		prompt = "Are you sure you want to run 'terragrunt apply' in each folder of the stack described above?"
	case "destroy":
		prompt = "WARNING: Are you sure you want to run `terragrunt destroy` in each folder of the stack described above? There is no undo!"
	case "state":
		prompt = "Are you sure you want to manipulate the state with `terragrunt state` in each folder of the stack described above? Note that absolute paths are shared, while relative paths will be relative to each working directory."
	}
	if prompt != "" {
		shouldRunAll, err := shell.PromptUserForYesNo(prompt, terragruntOptions)
		if err != nil {
			return err
		}
		if shouldRunAll == false {
			return nil
		}
	}

	return stack.Run(terragruntOptions)
}

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

type RunAllDisabledErr struct {
	command string
	reason  string
}

func (err RunAllDisabledErr) Error() string {
	return fmt.Sprintf("%s with run-all is disabled: %s", err.command, err.reason)
}

type MissingCommand struct{}

func (commandName MissingCommand) Error() string {
	return "Missing run-all command argument (Example: terragrunt run-all plan)"
}
