package cli

import (
	"fmt"
	"os"
	"regexp"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/locks"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/urfave/cli"
)

// Since Terragrunt is just a thin wrapper for Terraform, and we don't want to repeat every single Terraform command
// in its definition, we don't quite fit into the model of any Go CLI library. Fortunately, urfave/cli allows us to
// override the whole template used for the Usage Text.
const CUSTOM_USAGE_TEXT = `DESCRIPTION:
   {{.Name}} - {{.UsageText}}

USAGE:
   {{.Usage}}

COMMANDS:
   apply                Acquire a lock and run 'terraform apply'
   destroy              Acquire a lock and run 'terraform destroy'
   import               Acquire a lock and run 'terraform import'
   release-lock         Release a lock that is left over from some previous command
   *                    Terragrunt forwards all other commands directly to Terraform
{{if .VisibleFlags}}
GLOBAL OPTIONS:
   {{range .VisibleFlags}}{{.}}
   {{end}}{{end}}
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

	var defaultConfigFilePath = config.ConfigFilePath
	if os.Getenv("TERRAGRUNT_CONFIG") != "" {
		defaultConfigFilePath = os.Getenv("TERRAGRUNT_CONFIG")
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "terragrunt-config",
			Value: defaultConfigFilePath,
			Usage: ".terragrunt file to use",
		},
	}

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

	conf, err := config.ReadTerragruntConfig(cliContext.String("terragrunt-config"))
	if err != nil {
		return err
	}

	if err := downloadModules(cliContext); err != nil {
		return err
	}

	if conf.RemoteState != nil {
		if err := configureRemoteState(cliContext, conf.RemoteState); err != nil {
			return err
		}
	}

	if conf.Lock == nil {
		util.Logger.Printf("WARNING: you have not configured locking in your .terragrunt file. Concurrent changes to your .tfstate files may cause conflicts!")
		return runTerraformCommand(cliContext)
	}

	return runTerraformCommandWithLock(cliContext, conf.Lock)
}

// A quick sanity check that calls `terraform get` to download modules, if they aren't already downloaded.
func downloadModules(cliContext *cli.Context) error {
	switch cliContext.Args().First() {
	case "apply", "destroy", "graph", "output", "plan", "show", "taint", "untaint", "validate":
		shouldDownload, err := shouldDownloadModules()
		if err != nil {
			return err
		}
		if shouldDownload {
			return shell.RunShellCommand("terraform", "get", "-update")
		}
	}

	return nil
}

// Return true if modules aren't already downloaded and the Terraform templates in this project reference modules.
// Note that to keep the logic in this code very simple, this code ONLY detects the case where you haven't downloaded
// modules at all. Detecting if your downloaded modules are out of date (as opposed to missing entirely) is more
// complicated and not something we handle at the moment.
func shouldDownloadModules() (bool, error) {
	if util.FileExists(".terraform/modules") {
		return false, nil
	}

	return util.Grep(MODULE_REGEX, TERRAFORM_EXTENSION_GLOB)
}

// If the user entered a Terraform command that uses state (e.g. plan, apply), make sure remote state is configured
// before running the command.
func configureRemoteState(cliContext *cli.Context, remoteState *remote.RemoteState) error {
	// We only configure remote state for the commands that use the tfstate files. We do not configure it for
	// commands such as "get" or "version".
	switch cliContext.Args().First() {
	case "apply", "destroy", "import", "graph", "output", "plan", "push", "refresh", "show", "taint", "untaint", "validate":
		return remoteState.ConfigureRemoteState()
	case "remote":
		if cliContext.Args().Get(1) == "config" {
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
func runTerraformCommandWithLock(cliContext *cli.Context, lock locks.Lock) error {
	switch cliContext.Args().First() {
	case "apply", "destroy", "import":
		return locks.WithLock(lock, func() error { return runTerraformCommand(cliContext) })
	case "release-lock":
		return runReleaseLockCommand(cliContext, lock)
	default:
		return runTerraformCommand(cliContext)
	}
}

// Run the given Terraform command
func runTerraformCommand(cliContext *cli.Context) error {
	return shell.RunShellCommand("terraform", cliContext.Args()...)
}

// Release a lock, prompting the user for confirmation first
func runReleaseLockCommand(cliContext *cli.Context, lock locks.Lock) error {
	proceed, err := shell.PromptUserForYesNo(fmt.Sprintf("Are you sure you want to release %s?", lock))
	if err != nil {
		return err
	}

	if proceed {
		return lock.ReleaseLock()
	} else {
		return nil
	}
}

var DontManuallyConfigureRemoteState = fmt.Errorf("Instead of manually using the 'remote config' command, define your remote state settings in .terragrunt and Terragrunt will automatically configure it for you (and all your team members) next time you run it.")
