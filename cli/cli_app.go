package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/mattn/go-zglob"
	"github.com/urfave/cli"
)

const OPT_TERRAGRUNT_CONFIG = "terragrunt-config"
const OPT_TERRAGRUNT_TFPATH = "terragrunt-tfpath"
const OPT_TERRAGRUNT_NO_AUTO_INIT = "terragrunt-no-auto-init"
const OPT_TERRAGRUNT_NO_AUTO_RETRY = "terragrunt-no-auto-retry"
const OPT_NON_INTERACTIVE = "terragrunt-non-interactive"
const OPT_WORKING_DIR = "terragrunt-working-dir"
const OPT_DOWNLOAD_DIR = "terragrunt-download-dir"
const OPT_TERRAGRUNT_SOURCE = "terragrunt-source"
const OPT_TERRAGRUNT_SOURCE_UPDATE = "terragrunt-source-update"
const OPT_TERRAGRUNT_IAM_ROLE = "terragrunt-iam-role"
const OPT_TERRAGRUNT_IGNORE_DEPENDENCY_ERRORS = "terragrunt-ignore-dependency-errors"
const OPT_TERRAGRUNT_IGNORE_DEPENDENCY_ORDER = "terragrunt-ignore-dependency-order"
const OPT_TERRAGRUNT_IGNORE_EXTERNAL_DEPENDENCIES = "terragrunt-ignore-external-dependencies"
const OPT_TERRAGRUNT_INCLUDE_EXTERNAL_DEPENDENCIES = "terragrunt-include-external-dependencies"
const OPT_TERRAGRUNT_EXCLUDE_DIR = "terragrunt-exclude-dir"
const OPT_TERRAGRUNT_INCLUDE_DIR = "terragrunt-include-dir"
const OPT_TERRAGRUNT_STRICT_INCLUDE = "terragrunt-strict-include"
const OPT_TERRAGRUNT_CHECK = "terragrunt-check"
const OPT_TERRAGRUNT_HCLFMT_FILE = "terragrunt-hclfmt-file"

var ALL_TERRAGRUNT_BOOLEAN_OPTS = []string{
	OPT_NON_INTERACTIVE,
	OPT_TERRAGRUNT_SOURCE_UPDATE,
	OPT_TERRAGRUNT_IGNORE_DEPENDENCY_ERRORS,
	OPT_TERRAGRUNT_IGNORE_DEPENDENCY_ORDER,
	OPT_TERRAGRUNT_IGNORE_EXTERNAL_DEPENDENCIES,
	OPT_TERRAGRUNT_INCLUDE_EXTERNAL_DEPENDENCIES,
	OPT_TERRAGRUNT_NO_AUTO_INIT,
	OPT_TERRAGRUNT_NO_AUTO_RETRY,
	OPT_TERRAGRUNT_CHECK,
	OPT_TERRAGRUNT_STRICT_INCLUDE,
}
var ALL_TERRAGRUNT_STRING_OPTS = []string{
	OPT_TERRAGRUNT_CONFIG,
	OPT_TERRAGRUNT_TFPATH,
	OPT_WORKING_DIR,
	OPT_DOWNLOAD_DIR,
	OPT_TERRAGRUNT_SOURCE,
	OPT_TERRAGRUNT_IAM_ROLE,
	OPT_TERRAGRUNT_EXCLUDE_DIR,
	OPT_TERRAGRUNT_INCLUDE_DIR,
	OPT_TERRAGRUNT_HCLFMT_FILE,
}

const CMD_PLAN_ALL = "plan-all"
const CMD_APPLY_ALL = "apply-all"
const CMD_DESTROY_ALL = "destroy-all"
const CMD_OUTPUT_ALL = "output-all"
const CMD_VALIDATE_ALL = "validate-all"

const CMD_INIT = "init"
const CMD_INIT_FROM_MODULE = "init-from-module"
const CMD_TERRAGRUNT_INFO = "terragrunt-info"
const CMD_TERRAGRUNT_READ_CONFIG = "terragrunt-read-config"
const CMD_HCLFMT = "hclfmt"

// CMD_SPIN_UP is deprecated.
const CMD_SPIN_UP = "spin-up"

// CMD_TEAR_DOWN is deprecated.
const CMD_TEAR_DOWN = "tear-down"

var MULTI_MODULE_COMMANDS = []string{CMD_APPLY_ALL, CMD_DESTROY_ALL, CMD_OUTPUT_ALL, CMD_PLAN_ALL, CMD_VALIDATE_ALL}

// DEPRECATED_COMMANDS is a map of deprecated commands to the commands that replace them.
var DEPRECATED_COMMANDS = map[string]string{
	CMD_SPIN_UP:   CMD_APPLY_ALL,
	CMD_TEAR_DOWN: CMD_DESTROY_ALL,
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
}

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
   plan-all             Display the plans of a 'stack' by running 'terragrunt plan' in each subfolder
   apply-all            Apply a 'stack' by running 'terragrunt apply' in each subfolder
   output-all           Display the outputs of a 'stack' by running 'terragrunt output' in each subfolder
   destroy-all          Destroy a 'stack' by running 'terragrunt destroy' in each subfolder
   validate-all         Validate 'stack' by running 'terragrunt validate' in each subfolder
   terragrunt-info      Emits limited terragrunt state on stdout and exits
   hclfmt               Recursively find terragrunt.hcl files and rewrite them into a canonical format.
   *                    Terragrunt forwards all other commands directly to Terraform

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
   terragrunt-iam-role             	    	    Assume the specified IAM role before executing Terraform. Can also be set via the TERRAGRUNT_IAM_ROLE environment variable.
   terragrunt-ignore-dependency-errors          *-all commands continue processing components even if a dependency fails.
   terragrunt-ignore-dependency-order           *-all commands will be run disregarding the dependencies
   terragrunt-ignore-external-dependencies      *-all commands will not attempt to include external dependencies
   terragrunt-include-external-dependencies     *-all commands will include external dependencies
   terragrunt-exclude-dir                       Unix-style glob of directories to exclude when running *-all commands
   terragrunt-include-dir                       Unix-style glob of directories to include when running *-all commands
   terragrunt-check                             Enable check mode in the hclfmt command.
   terragrunt-hclfmt-file						The path to a single terragrunt.hcl file that the hclfmt command should run on.

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

// The supported flags to show help of terraform commands
var TERRAFORM_HELP_FLAGS = []string{
	"--help",
	"-help",
	"-h",
}

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
	app.Usage = "terragrunt <COMMAND>"
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
		cli.ShowAppHelp(cliContext)
		return nil
	}

	terragruntOptions, err := ParseTerragruntOptions(cliContext)
	if err != nil {
		return err
	}

	givenCommand := cliContext.Args().First()
	command := checkDeprecated(givenCommand, terragruntOptions)
	return runCommand(command, terragruntOptions)
}

// checkDeprecated checks if the given command is deprecated.  If so: prints a message and returns the new command.
func checkDeprecated(command string, terragruntOptions *options.TerragruntOptions) string {
	newCommand, deprecated := DEPRECATED_COMMANDS[command]
	if deprecated {
		terragruntOptions.Logger.Printf("%v is deprecated; running %v instead.\n", command, newCommand)
		return newCommand
	}
	return command
}

// runCommand runs one or many terraform commands based on the type of
// terragrunt command
func runCommand(command string, terragruntOptions *options.TerragruntOptions) (finalEff error) {
	if isMultiModuleCommand(command) {
		return runMultiModuleCommand(command, terragruntOptions)
	}
	return RunTerragrunt(terragruntOptions)
}

// Downloads terraform source if necessary, then runs terraform with the given options and CLI args.
// This will forward all the args and extra_arguments directly to Terraform.
func RunTerragrunt(terragruntOptions *options.TerragruntOptions) error {
	if shouldPrintTerraformHelp(terragruntOptions) {
		return shell.RunTerraformCommand(terragruntOptions, terragruntOptions.TerraformCliArgs...)
	}

	if shouldRunHCLFmt(terragruntOptions) {
		return runHCLFmt(terragruntOptions)
	}

	terragruntConfig, err := config.ReadTerragruntConfig(terragruntOptions)

	if err != nil {
		return err
	}

	terragruntOptionsClone := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	terragruntOptionsClone.TerraformCommand = CMD_TERRAGRUNT_READ_CONFIG

	if err := processHooks(terragruntConfig.Terraform.GetAfterHooks(), terragruntOptionsClone); err != nil {
		return err
	}

	// Change the terraform binary path before checking the version
	// if the path is not changed from default and set in the config.
	if terragruntOptions.TerraformPath == options.TERRAFORM_DEFAULT_PATH && terragruntConfig.TerraformBinary != "" {
		terragruntOptions.TerraformPath = terragruntConfig.TerraformBinary
	}

	if err := PopulateTerraformVersion(terragruntOptions); err != nil {
		return err
	}

	versionConstraint := DEFAULT_TERRAFORM_VERSION_CONSTRAINT
	if terragruntConfig.TerraformVersionConstraint != "" {
		versionConstraint = terragruntConfig.TerraformVersionConstraint
	}
	if err := CheckTerraformVersion(versionConstraint, terragruntOptions); err != nil {
		return err
	}

	if terragruntConfig.Skip {
		terragruntOptions.Logger.Printf("Skipping terragrunt module %s due to skip = true.",
			terragruntOptions.TerragruntConfigPath)
		return nil
	}

	if terragruntOptions.IamRole == "" {
		terragruntOptions.IamRole = terragruntConfig.IamRole
	}

	if err := assumeRoleIfNecessary(terragruntOptions); err != nil {
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

	if sourceUrl := getTerraformSourceUrl(terragruntOptions, terragruntConfig); sourceUrl != "" {
		if err := downloadTerraformSource(sourceUrl, terragruntOptions, terragruntConfig); err != nil {
			return err
		}
	}

	if shouldPrintTerragruntInfo(terragruntOptions) {
		group := TerragruntInfoGroup{
			ConfigPath:       terragruntOptions.TerragruntConfigPath,
			DownloadDir:      terragruntOptions.DownloadDir,
			IamRole:          terragruntOptions.IamRole,
			TerraformBinary:  terragruntOptions.TerraformPath,
			TerraformCommand: terragruntOptions.TerraformCommand,
			WorkingDir:       terragruntOptions.WorkingDir,
		}
		b, err := json.MarshalIndent(group, "", "  ")
		if err != nil {
			terragruntOptions.Logger.Printf("JSON error marshalling terragrunt-info")
			return err
		}
		fmt.Fprintf(terragruntOptions.Writer, "%s\n", b)
		return nil
	}

	if err := checkFolderContainsTerraformCode(terragruntOptions); err != nil {
		return err
	}

	// Handle code generation configs, both generate blocks and generate attribute of remote_state.
	// Note that relative paths are relative to the terragrunt working dir (where terraform is called).
	for _, config := range terragruntConfig.GenerateConfigs {
		if err := codegen.WriteToFile(terragruntOptions.Logger, terragruntOptions.WorkingDir, config); err != nil {
			return err
		}
	}
	if terragruntConfig.RemoteState != nil && terragruntConfig.RemoteState.Generate != nil {
		if err := terragruntConfig.RemoteState.GenerateTerraformCode(terragruntOptions); err != nil {
			return err
		}
	}

	if terragruntConfig.RemoteState != nil {
		if err := checkTerraformCodeDefinesBackend(terragruntOptions, terragruntConfig.RemoteState.Backend); err != nil {
			return err
		}
	}

	return runTerragruntWithConfig(terragruntOptions, terragruntConfig, false)
}

func shouldPrintTerraformHelp(terragruntOptions *options.TerragruntOptions) bool {
	for _, tfHelpFlag := range TERRAFORM_HELP_FLAGS {
		if util.ListContainsElement(terragruntOptions.TerraformCliArgs, tfHelpFlag) {
			return true
		}
	}
	return false
}

func shouldPrintTerragruntInfo(terragruntOptions *options.TerragruntOptions) bool {
	return util.ListContainsElement(terragruntOptions.TerraformCliArgs, CMD_TERRAGRUNT_INFO)
}

func shouldRunHCLFmt(terragruntOptions *options.TerragruntOptions) bool {
	return util.ListContainsElement(terragruntOptions.TerraformCliArgs, CMD_HCLFMT)
}

func processHooks(hooks []config.Hook, terragruntOptions *options.TerragruntOptions, previousExecError ...error) error {
	if len(hooks) == 0 {
		return nil
	}

	errorsOccurred := []error{}

	terragruntOptions.Logger.Printf("Detected %d Hooks", len(hooks))

	for _, curHook := range hooks {
		allPreviousErrors := append(previousExecError, errorsOccurred...)
		if shouldRunHook(curHook, terragruntOptions, allPreviousErrors...) {
			terragruntOptions.Logger.Printf("Executing hook: %s", curHook.Name)
			actionToExecute := curHook.Execute[0]
			actionParams := curHook.Execute[1:]
			possibleError := shell.RunShellCommand(terragruntOptions, actionToExecute, actionParams...)

			if possibleError != nil {
				terragruntOptions.Logger.Printf("Error running hook %s with message: %s", curHook.Name, possibleError.Error())
				errorsOccurred = append(errorsOccurred, possibleError)
			}

		}
	}

	return errors.NewMultiError(errorsOccurred...)
}

func shouldRunHook(hook config.Hook, terragruntOptions *options.TerragruntOptions, previousExecErrors ...error) bool {
	//if there's no previous error, execute command
	//OR if a previous error DID happen AND we want to run anyways
	//then execute.
	//Skip execution if there was an error AND we care about errors

	//resolves: https://github.com/gruntwork-io/terragrunt/issues/459
	//by helping to filter out nil errors that were acting as false positives
	//for the len(previousExecErrors) == 0 check that used to be here
	multiError := errors.NewMultiError(previousExecErrors...)

	return util.ListContainsElement(hook.Commands, terragruntOptions.TerraformCommand) && (multiError == nil || (hook.RunOnError != nil && *hook.RunOnError))
}

// Assume an IAM role, if one is specified, by making API calls to Amazon STS and setting the environment variables
// we get back inside of terragruntOptions.Env
func assumeRoleIfNecessary(terragruntOptions *options.TerragruntOptions) error {
	if terragruntOptions.IamRole == "" {
		return nil
	}

	terragruntOptions.Logger.Printf("Assuming IAM role %s", terragruntOptions.IamRole)
	creds, err := aws_helper.AssumeIamRole(terragruntOptions.IamRole)
	if err != nil {
		return err
	}

	terragruntOptions.Env["AWS_ACCESS_KEY_ID"] = aws.StringValue(creds.AccessKeyId)
	terragruntOptions.Env["AWS_SECRET_ACCESS_KEY"] = aws.StringValue(creds.SecretAccessKey)
	terragruntOptions.Env["AWS_SESSION_TOKEN"] = aws.StringValue(creds.SessionToken)
	terragruntOptions.Env["AWS_SECURITY_TOKEN"] = aws.StringValue(creds.SessionToken)

	return nil
}

// Runs terraform with the given options and CLI args.
// This will forward all the args and extra_arguments directly to Terraform.
func runTerragruntWithConfig(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, allowSourceDownload bool) error {

	// Add extra_arguments to the command
	if terragruntConfig.Terraform != nil && terragruntConfig.Terraform.ExtraArgs != nil && len(terragruntConfig.Terraform.ExtraArgs) > 0 {
		terragruntOptions.InsertTerraformCliArgs(filterTerraformExtraArgs(terragruntOptions, terragruntConfig)...)
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
		if err := prepareNonInitCommand(terragruntOptions, terragruntConfig); err != nil {
			return err
		}
	}

	if err := checkProtectedModule(terragruntOptions, terragruntConfig); err != nil {
		return err
	}

	return runActionWithHooks("terraform", terragruntOptions, terragruntConfig, func() error {
		return runTerraformWithRetry(terragruntOptions)
	})
}

// Run the given action function surrounded by hooks. That is, run the before hooks first, then, if there were no
// errors, run the action, and finally, run the after hooks. Return any errors hit from the hooks or action.
func runActionWithHooks(description string, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, action func() error) error {
	beforeHookErrors := processHooks(terragruntConfig.Terraform.GetBeforeHooks(), terragruntOptions)

	var actionErrors error
	if beforeHookErrors == nil {
		actionErrors = action()
	} else {
		terragruntOptions.Logger.Printf("Errors encountered running before_hooks. Not running '%s'.", description)
	}

	postHookErrors := processHooks(terragruntConfig.Terraform.GetAfterHooks(), terragruntOptions, beforeHookErrors, actionErrors)

	return errors.NewMultiError(beforeHookErrors, actionErrors, postHookErrors)
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
	for i := 0; i < terragruntOptions.MaxRetryAttempts; i++ {
		if out, tferr := shell.RunTerraformCommandWithOutput(terragruntOptions, terragruntOptions.TerraformCliArgs...); tferr != nil {
			if out != nil && isRetryable(out.Stderr, tferr, terragruntOptions) {
				terragruntOptions.Logger.Printf("Encountered an error eligible for retrying. Sleeping %v before retrying.\n", terragruntOptions.Sleep)
				time.Sleep(terragruntOptions.Sleep)
			} else {
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
	files, err := zglob.Glob(fmt.Sprintf("%s/**/*.tf", terragruntOptions.WorkingDir))
	if err != nil {
		return errors.WithStackTrace(err)
	}

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

// Prepare for running any command other than 'terraform init' by
// running 'terraform init' if necessary
func prepareNonInitCommand(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	needsInit, err := needsInit(terragruntOptions, terragruntConfig)
	if err != nil {
		return err
	}

	if needsInit {
		if err := runTerraformInit(terragruntOptions, terragruntConfig, nil); err != nil {
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
	providersPath := util.JoinPath(terragruntOptions.DataDir(), "plugins")
	return !util.FileExists(providersPath)
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
// This method will return an error and NOT run terraform init if the user has disabled Auto-Init
func runTerraformInit(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, terraformSource *TerraformSource) error {

	// Prevent Auto-Init if the user has disabled it
	if util.FirstArg(terragruntOptions.TerraformCliArgs) != CMD_INIT && !terragruntOptions.AutoInit {
		return errors.WithStackTrace(InitNeededButDisabled("Cannot continue because init is needed, but Auto-Init is disabled.  You must run 'terragrunt init' manually."))
	}

	initOptions, err := prepareInitOptions(terragruntOptions, terraformSource)

	if err != nil {
		return err
	}

	return runTerragruntWithConfig(initOptions, terragruntConfig, terraformSource != nil)
}

func prepareInitOptions(terragruntOptions *options.TerragruntOptions, terraformSource *TerraformSource) (*options.TerragruntOptions, error) {
	// Need to clone the terragruntOptions, so the TerraformCliArgs can be configured to run the init command
	initOptions := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	initOptions.TerraformCliArgs = []string{CMD_INIT}
	initOptions.WorkingDir = terragruntOptions.WorkingDir
	initOptions.TerraformCommand = CMD_INIT

	// Don't pollute stdout with the stdout from Auto Init
	initOptions.Writer = initOptions.ErrWriter

	return initOptions, nil
}

// Returns true if the command the user wants to execute is supposed to affect multiple Terraform modules, such as the
// apply-all or destroy-all command.
func isMultiModuleCommand(command string) bool {
	return util.ListContainsElement(MULTI_MODULE_COMMANDS, command)
}

// Execute a command that affects multiple Terraform modules, such as the apply-all or destroy-all command.
func runMultiModuleCommand(command string, terragruntOptions *options.TerragruntOptions) error {
	switch command {
	case CMD_PLAN_ALL:
		return planAll(terragruntOptions)
	case CMD_APPLY_ALL:
		return applyAll(terragruntOptions)
	case CMD_DESTROY_ALL:
		return destroyAll(terragruntOptions)
	case CMD_OUTPUT_ALL:
		return outputAll(terragruntOptions)
	case CMD_VALIDATE_ALL:
		return validateAll(terragruntOptions)
	default:
		return errors.WithStackTrace(UnrecognizedCommand(command))
	}
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

// planAll prints the plans from all configuration in a stack, in the order
// specified in the terraform_remote_state dependencies
func planAll(terragruntOptions *options.TerragruntOptions) error {
	stack, err := configstack.FindStackInSubfolders(terragruntOptions)
	if err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("%s", stack.String())
	return stack.Plan(terragruntOptions)
}

// Spin up an entire "stack" by running 'terragrunt apply' in each subfolder, processing them in the right order based
// on terraform_remote_state dependencies.
func applyAll(terragruntOptions *options.TerragruntOptions) error {
	stack, err := configstack.FindStackInSubfolders(terragruntOptions)
	if err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("%s", stack.String())
	shouldApplyAll, err := shell.PromptUserForYesNo("Are you sure you want to run 'terragrunt apply' in each folder of the stack described above?", terragruntOptions)
	if err != nil {
		return err
	}

	if shouldApplyAll {
		return stack.Apply(terragruntOptions)
	}

	return nil
}

// Tear down an entire "stack" by running 'terragrunt destroy' in each subfolder, processing them in the right order
// based on terraform_remote_state dependencies.
func destroyAll(terragruntOptions *options.TerragruntOptions) error {
	stack, err := configstack.FindStackInSubfolders(terragruntOptions)
	if err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("%s", stack.String())
	shouldDestroyAll, err := shell.PromptUserForYesNo("WARNING: Are you sure you want to run `terragrunt destroy` in each folder of the stack described above? There is no undo!", terragruntOptions)
	if err != nil {
		return err
	}

	if shouldDestroyAll {
		return stack.Destroy(terragruntOptions)
	}

	return nil
}

// outputAll prints the outputs from all configuration in a stack, in the order
// specified in the terraform_remote_state dependencies
func outputAll(terragruntOptions *options.TerragruntOptions) error {
	stack, err := configstack.FindStackInSubfolders(terragruntOptions)
	if err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("%s", stack.String())
	return stack.Output(terragruntOptions)
}

// validateAll validates runs terraform validate on all the modules
func validateAll(terragruntOptions *options.TerragruntOptions) error {
	stack, err := configstack.FindStackInSubfolders(terragruntOptions)
	if err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("%s", stack.String())
	return stack.Validate(terragruntOptions)
}

// checkProtectedModule checks if module is protected via the "prevent_destroy" flag
func checkProtectedModule(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	if util.FirstArg(terragruntOptions.TerraformCliArgs) != "destroy" {
		return nil
	}
	if terragruntConfig.PreventDestroy {
		return errors.WithStackTrace(ModuleIsProtected{Opts: terragruntOptions})
	}
	return nil
}

// isRetryable checks whether there was an error and we should attempt again
func isRetryable(tfoutput string, tferr error, terragruntOptions *options.TerragruntOptions) bool {
	if !terragruntOptions.AutoRetry || tferr == nil {
		return false
	}
	return util.MatchesAny(terragruntOptions.RetryableErrors, tfoutput)
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

type InitNeededButDisabled string

func (err InitNeededButDisabled) Error() string {
	return string(err)
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
	return fmt.Sprintf("Exhausted retries (%v) for command %v %v", err.Opts.MaxRetryAttempts, err.Opts.TerraformPath, strings.Join(err.Opts.TerraformCliArgs, " "))
}
