package cli

import (
	"github.com/urfave/cli"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/locks"
	"fmt"
	"github.com/gruntwork-io/terragrunt/shell"
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

// Create the Terragrunt CLI App
func CreateTerragruntCli(version string) *cli.App {
	cli.AppHelpTemplate = CUSTOM_USAGE_TEXT

	app := cli.NewApp()

	app.Name = "terragrunt"
	app.Author = "Gruntwork <www.gruntwork.io>"
	app.Version = version
	app.Action = runApp
	app.Usage = "terragrunt <COMMAND>"
	app.UsageText = `Terragrunt is a thin wrapper for the Terraform client that provides a distributed locking
   mechanism which allows multiple people to collaborate on the same Terraform state without overwriting each other's
   changes. Terragrunt currently uses Amazon's DynamoDB to acquire and release locks. For documentation, see
   https://github.com/gruntwork-io/terragrunt/.

   Terragrunt supports all the same commands as Terraform (e.g. plan, apply, destroy, etc). However, for the apply and
   destroy commands, it will first acquire a lock, then run the command with Terraform, and then release the lock.`

	return app
}

// The sole action for the app. It forwards all commands directly to Terraform, enforcing a few best practices along
// the way, such as configuring remote state or acquiring a lock.
func runApp(cliContext *cli.Context) error {
	terragruntConfig, err := config.ReadTerragruntConfig()
	if err != nil {
		return err
	}

	if terragruntConfig.RemoteState != nil {
		if err := terragruntConfig.RemoteState.ConfigureRemoteState(); err != nil {
			return err
		}
	}

	if terragruntConfig.DynamoDbLock != nil {
		return runTerraformCommandWithLock(cliContext, terragruntConfig.DynamoDbLock)
	} else {
		return runTerraformCommand(cliContext)
	}
}

// Run the given Terraform command with the given lock (if the command requires locking)
func runTerraformCommandWithLock(cliContext *cli.Context, lock locks.Lock) error {
	switch cliContext.Args().First() {
	case "apply", "destroy": return locks.WithLock(lock, func() error { return runTerraformCommand(cliContext) })
	case "release-lock": return runReleaseLockCommand(cliContext, lock)
	default: return runTerraformCommand(cliContext)
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