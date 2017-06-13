package cli

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/urfave/cli"
	"io"
	"regexp"
)

const OPT_TERRAGRUNT_CONFIG = "terragrunt-config"
const OPT_TERRAGRUNT_TFPATH = "terragrunt-tfpath"
const OPT_NON_INTERACTIVE = "terragrunt-non-interactive"
const OPT_WORKING_DIR = "terragrunt-working-dir"
const OPT_TERRAGRUNT_SOURCE = "terragrunt-source"
const OPT_TERRAGRUNT_SOURCE_UPDATE = "terragrunt-source-update"
const OPT_TERRAGRUNT_IGNORE_DEPENDENCY_ERRORS = "terragrunt-ignore-dependency-errors"

var ALL_TERRAGRUNT_BOOLEAN_OPTS = []string{OPT_NON_INTERACTIVE, OPT_TERRAGRUNT_SOURCE_UPDATE, OPT_TERRAGRUNT_IGNORE_DEPENDENCY_ERRORS}
var ALL_TERRAGRUNT_STRING_OPTS = []string{OPT_TERRAGRUNT_CONFIG, OPT_TERRAGRUNT_TFPATH, OPT_WORKING_DIR, OPT_TERRAGRUNT_SOURCE}

const CMD_PLAN_ALL = "plan-all"
const CMD_APPLY_ALL = "apply-all"
const CMD_DESTROY_ALL = "destroy-all"
const CMD_OUTPUT_ALL = "output-all"

const CMD_INIT = "init"

// CMD_SPIN_UP is deprecated.
const CMD_SPIN_UP = "spin-up"

// CMD_TEAR_DOWN is deprecated.
const CMD_TEAR_DOWN = "tear-down"

var MULTI_MODULE_COMMANDS = []string{CMD_APPLY_ALL, CMD_DESTROY_ALL, CMD_OUTPUT_ALL, CMD_PLAN_ALL}

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

var TERRAFORM_COMMANDS_WITH_SUBCOMMAND = []string{
	"debug",
	"force-unlock",
	"state",
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
   *                    Terragrunt forwards all other commands directly to Terraform

GLOBAL OPTIONS:
   terragrunt-config                    Path to the Terragrunt config file. Default is terraform.tfvars.
   terragrunt-tfpath                    Path to the Terraform binary. Default is terraform (on PATH).
   terragrunt-non-interactive           Assume "yes" for all prompts.
   terragrunt-working-dir               The path to the Terraform templates. Default is current directory.
   terragrunt-source                    Download Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder.
   terragrunt-source-update             Delete the contents of the temporary folder to clear out any old, cached source code before downloading new source code into it.
   terragrunt-ignore-dependency-errors  *-all commands continue processing components even if a dependency fails.

VERSION:
   {{.Version}}{{if len .Authors}}

AUTHOR(S):
   {{range .Authors}}{{.}}{{end}}
   {{end}}
`

var MODULE_REGEX = regexp.MustCompile(`module[[:blank:]]+".+"`)

// This uses the constraint syntax from https://github.com/hashicorp/go-version
const DEFAULT_TERRAFORM_VERSION_CONSTRAINT = ">= v0.9.3"

const TERRAFORM_EXTENSION_GLOB = "*.tf"

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

	if err := CheckTerraformVersion(DEFAULT_TERRAFORM_VERSION_CONSTRAINT, terragruntOptions); err != nil {
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
	return runTerragrunt(terragruntOptions)
}

// Run Terragrunt with the given options and CLI args. This will forward all the args directly to Terraform, enforcing
// best practices along the way.
func runTerragrunt(terragruntOptions *options.TerragruntOptions) error {
	conf, err := config.ReadTerragruntConfig(terragruntOptions)
	if err != nil {
		return err
	}

	if conf.Terraform != nil && conf.Terraform.ExtraArgs != nil && len(conf.Terraform.ExtraArgs) > 0 {
		commandLength := 1
		if util.ListContainsElement(TERRAFORM_COMMANDS_WITH_SUBCOMMAND, terragruntOptions.TerraformCliArgs[0]) {
			commandLength = 2
		}

		// Options must be inserted after command but before the other args
		// command is either 1 word or 2 words
		var args []string
		args = append(args, terragruntOptions.TerraformCliArgs[:commandLength]...)
		args = append(args, filterTerraformExtraArgs(terragruntOptions, conf)...)
		args = append(args, terragruntOptions.TerraformCliArgs[commandLength:]...)
		terragruntOptions.TerraformCliArgs = args
	}

	if sourceUrl, hasSourceUrl := getTerraformSourceUrl(terragruntOptions, conf); hasSourceUrl {
		if err := downloadTerraformSource(sourceUrl, terragruntOptions); err != nil {
			return err
		}
	}

	if err := downloadModules(terragruntOptions); err != nil {
		return err
	}

	if conf.RemoteState != nil {
		if err := configureRemoteState(conf.RemoteState, terragruntOptions); err != nil {
			return err
		}
	}

	// If the command is 'init', stop here. That's because ConfigureRemoteState above will have already called
	// terraform init if it was necessary, and the below RunTerraformCommand would end up calling init without
	// the correct remote state arguments, which is confusing.
	if terragruntOptions.TerraformCliArgs[0] == CMD_INIT {
		terragruntOptions.Logger.Println("Running 'init' manually is not necessary: Terragrunt will call it automatically when needed before running other Terraform commands")
		return nil
	}

	return shell.RunTerraformCommand(terragruntOptions, terragruntOptions.TerraformCliArgs...)
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
	default:
		return errors.WithStackTrace(UnrecognizedCommand(command))
	}
}

// A quick sanity check that calls `terraform get` to download modules, if they aren't already downloaded.
func downloadModules(terragruntOptions *options.TerragruntOptions) error {
	switch firstArg(terragruntOptions.TerraformCliArgs) {
	case "apply", "destroy", "graph", "output", "plan", "show", "taint", "untaint", "validate":
		shouldDownload, err := shouldDownloadModules(terragruntOptions)
		if err != nil {
			return err
		}
		if shouldDownload {
			return shell.RunTerraformCommand(terragruntOptions, "get", "-update")
		}
	}

	return nil
}

// Return true if modules aren't already downloaded and the Terraform templates in this project reference modules.
// Note that to keep the logic in this code very simple, this code ONLY detects the case where you haven't downloaded
// modules at all. Detecting if your downloaded modules are out of date (as opposed to missing entirely) is more
// complicated and not something we handle at the moment.
func shouldDownloadModules(terragruntOptions *options.TerragruntOptions) (bool, error) {
	if util.FileExists(util.JoinPath(terragruntOptions.WorkingDir, ".terraform/modules")) {
		return false, nil
	}

	return util.Grep(MODULE_REGEX, fmt.Sprintf("%s/%s", terragruntOptions.WorkingDir, TERRAFORM_EXTENSION_GLOB))
}

// If the user entered a Terraform command that uses state (e.g. plan, apply), make sure remote state is configured
// before running the command.
func configureRemoteState(remoteState *remote.RemoteState, terragruntOptions *options.TerragruntOptions) error {
	// We only configure remote state for the commands that use the tfstate files. We do not configure it for
	// commands such as "get" or "version".
	if util.ListContainsElement(TERRAFORM_COMMANDS_THAT_USE_STATE, firstArg(terragruntOptions.TerraformCliArgs)) {
		return remoteState.ConfigureRemoteState(terragruntOptions)
	}

	return nil
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

// Custom error types

var DontManuallyConfigureRemoteState = fmt.Errorf("Instead of manually using the 'remote config' command, define your remote state settings in %s and Terragrunt will automatically configure it for you (and all your team members) next time you run it.", config.DefaultTerragruntConfigPath)

type UnrecognizedCommand string

func (commandName UnrecognizedCommand) Error() string {
	return fmt.Sprintf("Unrecognized command: %s", string(commandName))
}
