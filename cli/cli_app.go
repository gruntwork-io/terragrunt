package cli

import (
	"fmt"
	"regexp"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/locks"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/urfave/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/spin"
)

const OPT_TERRAGRUNT_CONFIG = "terragrunt-config"
const OPT_TERRAGRUNT_TFPATH = "terragrunt-tfpath"
const OPT_NON_INTERACTIVE = "terragrunt-non-interactive"
const OPT_WORKING_DIR = "terragrunt-working-dir"
const OPT_TERRAGRUNT_SOURCE = "terragrunt-source"
const OPT_TERRAGRUNT_SOURCE_UPDATE = "terragrunt-source-update"
var ALL_TERRAGRUNT_BOOLEAN_OPTS = []string{OPT_NON_INTERACTIVE, OPT_TERRAGRUNT_SOURCE_UPDATE}
var ALL_TERRAGRUNT_STRING_OPTS = []string{OPT_TERRAGRUNT_CONFIG, OPT_TERRAGRUNT_TFPATH, OPT_WORKING_DIR, OPT_TERRAGRUNT_SOURCE}

const CMD_ACQUIRE_LOCK = "acquire-lock"
const CMD_RELEASE_LOCK = "release-lock"
const CMD_SPIN_UP = "spin-up"
const CMD_TEAR_DOWN = "tear-down"
var MULTI_MODULE_COMMANDS = []string{CMD_SPIN_UP, CMD_TEAR_DOWN}

// Since Terragrunt is just a thin wrapper for Terraform, and we don't want to repeat every single Terraform command
// in its definition, we don't quite fit into the model of any Go CLI library. Fortunately, urfave/cli allows us to
// override the whole template used for the Usage Text.
var CUSTOM_USAGE_TEXT = `DESCRIPTION:
   {{.Name}} - {{.UsageText}}

USAGE:
   {{.Usage}}

COMMANDS:
   apply                Acquire a lock and run 'terraform apply'
   destroy              Acquire a lock and run 'terraform destroy'
   import               Acquire a lock and run 'terraform import'
   refresh              Acquire a lock and run 'terraform refresh'
   remote push          Acquire a lock and run 'terraform remote push'
   acquire-lock         Acquire a long-term lock for these templates
   release-lock         Release a long-term lock or a lock that failed to clean up
   spin-up              Spin up a 'stack' by running 'terragrunt apply' in each subfolder
   tear-down            Tear down a 'stack' by running 'terragrunt destroy' in each subfolder
   *                    Terragrunt forwards all other commands directly to Terraform

GLOBAL OPTIONS:
   terragrunt-config             Path to the Terragrunt config file. Default is .terragrunt.
   terragrunt-tfpath             Path to the Terraform binary. Default is terraform (on PATH).
   terragrunt-non-interactive    Assume "yes" for all prompts.
   terragrunt-working-dir        The path to the Terraform templates. Default is current directory.
   terragrunt-source             Download Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder
   terragrunt-source-update      Delete the contents of the temporary folder to clear out any old, cached source code before downloading new source code into it

VERSION:
   {{.Version}}{{if len .Authors}}

AUTHOR(S):
   {{range .Authors}}{{.}}{{end}}
   {{end}}
`

var MODULE_REGEX = regexp.MustCompile(`module ".+"`)

const TERRAFORM_EXTENSION_GLOB = "*.tf"

// Create the Terragrunt CLI App
func CreateTerragruntCli(version string) *cli.App {
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
	app.UsageText = `Terragrunt is a thin wrapper for [Terraform](https://www.terraform.io/) that supports locking
   via Amazon's DynamoDB and enforces best practices. Terragrunt forwards almost all commands, arguments, and options
   directly to Terraform, using whatever version of Terraform you already have installed. However, before running
   Terraform, Terragrunt will ensure your remote state is configured according to the settings in the .terragrunt file.
   Moreover, for the apply and destroy commands, Terragrunt will first try to acquire a lock using DynamoDB. For
   documentation, see https://github.com/gruntwork-io/terragrunt/.`

	return app
}

// The sole action for the app. It forwards all commands directly to Terraform, enforcing a few best practices along
// the way, such as configuring remote state or acquiring a lock.
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

	if isMultiModuleCommand(cliContext.Args().First()) {
		return runMultiModuleCommand(cliContext.Args().First(), terragruntOptions)
	} else {
		return runTerragrunt(terragruntOptions)
	}
}

// Run Terragrunt with the given options and CLI args. This will forward all the args directly to Terraform, enforcing
// best practices along the way, such as configuring remote state or acquiring a lock.
func runTerragrunt(terragruntOptions *options.TerragruntOptions) error {
	conf, err := config.ReadTerragruntConfig(terragruntOptions)
	if err != nil {
		return err
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

	if conf.Lock == nil {
		terragruntOptions.Logger.Printf("WARNING: you have not configured locking in your .terragrunt file. Concurrent changes to your .tfstate files may cause conflicts!")
		return runTerraformCommand(terragruntOptions)
	}

	return runTerraformCommandWithLock(conf.Lock, terragruntOptions)
}

// Returns true if the command the user wants to execute is supposed to affect multiple Terraform modules, such as the
// spin-up or tear-down command.
func isMultiModuleCommand(command string) bool {
	return util.ListContainsElement(MULTI_MODULE_COMMANDS, command)
}

// Execute a command that affects multiple Terraform modules, such as the spin-up or tear-down command.
func runMultiModuleCommand(command string, terragruntOptions *options.TerragruntOptions) error {
	switch command {
	case CMD_SPIN_UP:
		return spinUp(terragruntOptions)
	case CMD_TEAR_DOWN:
		return tearDown(terragruntOptions)
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
			return shell.RunShellCommand(terragruntOptions, terragruntOptions.TerraformPath, "get", "-update")
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
	switch firstArg(terragruntOptions.TerraformCliArgs) {
	case "apply", "destroy", "import", "graph", "output", "plan", "push", "refresh", "show", "taint", "untaint", "validate":
		return remoteState.ConfigureRemoteState(terragruntOptions)
	case "remote":
		if secondArg(terragruntOptions.TerraformCliArgs) == "config" {
			// Encourage the user to configure remote state by defining it in .terragrunt and letting
			// Terragrunt handle it for them
			return errors.WithStackTrace(DontManuallyConfigureRemoteState)
		} else {
			// The other "terraform remote" commands explicitly push or pull state, so we shouldn't mess
			// with the configuration
			return nil
		}
	}

	return nil
}

// Run the given Terraform command with the given lock (if the command requires locking)
func runTerraformCommandWithLock(lock locks.Lock, terragruntOptions *options.TerragruntOptions) error {
	switch firstArg(terragruntOptions.TerraformCliArgs) {
	case "apply", "destroy", "import", "refresh":
		return locks.WithLock(lock, terragruntOptions, func() error { return runTerraformCommand(terragruntOptions) })
	case "remote":
		if secondArg(terragruntOptions.TerraformCliArgs) == "push" {
			return locks.WithLock(lock, terragruntOptions, func() error { return runTerraformCommand(terragruntOptions) })
		} else {
			return runTerraformCommand(terragruntOptions)
		}
	case CMD_ACQUIRE_LOCK:
		return acquireLock(lock, terragruntOptions)
	case CMD_RELEASE_LOCK:
		return releaseLockCommand(lock, terragruntOptions)
	default:
		return runTerraformCommand(terragruntOptions)
	}
}

// Spin up an entire "stack" by running 'terragrunt apply' in each subfolder, processing them in the right order based
// on terraform_remote_state dependencies.
func spinUp(terragruntOptions *options.TerragruntOptions) error {
	stack, err := spin.FindStackInSubfolders(terragruntOptions)
	if err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("%s", stack.String())
	shouldSpinUp, err := shell.PromptUserForYesNo("Are you sure you want to run 'terragrunt apply' in each folder of the stack described above?", terragruntOptions)
	if err != nil {
		return err
	}

	if shouldSpinUp {
		return stack.Apply(terragruntOptions)
	}

	return nil
}

// Tear down an entire "stack" by running 'terragrunt destroy' in each subfolder, processing them in the right order
// based on terraform_remote_state dependencies.
func tearDown(terragruntOptions *options.TerragruntOptions) error {
	stack, err := spin.FindStackInSubfolders(terragruntOptions)
	if err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("%s", stack.String())
	shouldTearDown, err := shell.PromptUserForYesNo("WARNING: Are you sure you want to run `terragrunt destroy` in each folder of the stack described above? There is no undo!", terragruntOptions)
	if err != nil {
		return err
	}

	if shouldTearDown {
		return stack.Destroy(terragruntOptions)
	}

	return nil
}

// Acquire a lock. This can be useful for locking down a deploy for a long time, such as during a major deployment.
func acquireLock(lock locks.Lock, terragruntOptions *options.TerragruntOptions) error {
	shouldAcquireLock, err := shell.PromptUserForYesNo("Are you sure you want to acquire a long-term lock?", terragruntOptions)
	if err != nil {
		return err
	}

	if shouldAcquireLock {
		terragruntOptions.Logger.Printf("Acquiring long-term lock. To release the lock, use the release-lock command.")
		return lock.AcquireLock(terragruntOptions)
	}

	return nil
}

// Release a lock, prompting the user for confirmation first
func releaseLockCommand(lock locks.Lock, terragruntOptions *options.TerragruntOptions) error {
	prompt := fmt.Sprintf("Are you sure you want to release %s?", lock)
	proceed, err := shell.PromptUserForYesNo(prompt, terragruntOptions)
	if err != nil {
		return err
	}

	if proceed {
		return lock.ReleaseLock(terragruntOptions)
	} else {
		return nil
	}
}

// Run the given Terraform command
func runTerraformCommand(terragruntOptions *options.TerragruntOptions) error {
	return shell.RunShellCommand(terragruntOptions, terragruntOptions.TerraformPath, terragruntOptions.TerraformCliArgs...)
}


// Custom error types

var DontManuallyConfigureRemoteState = fmt.Errorf("Instead of manually using the 'remote config' command, define your remote state settings in .terragrunt and Terragrunt will automatically configure it for you (and all your team members) next time you run it.")

type UnrecognizedCommand string
func (commandName UnrecognizedCommand) Error() string {
	return fmt.Sprintf("Unrecognized command: %s", string(commandName))
}