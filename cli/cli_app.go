package cli

import (
	"github.com/urfave/cli"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/locks"
	"fmt"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/gruntcreds/gruntcreds/util"
)

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

func CreateTerragruntCli(version string) *cli.App {
	cli.AppHelpTemplate = CUSTOM_USAGE_TEXT

	app := cli.NewApp()

	app.Name = "terragrunt"
	app.Author = "Gruntwork <www.gruntwork.io>"
	app.Version = version
	app.Action = runApp
	app.Usage = "terragrunt <COMMAND>"
	app.UsageText = `Terragrunt is a thin wrapper for [Terraform](https://www.terraform.io/) that supports locking
   via Amazon's DynamoDB and enforces best practices. For documentation, see https://github.com/gruntwork-io/terragrunt/.`

	return app
}

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
		return runCommandWithLock(cliContext, terragruntConfig.DynamoDbLock)
	} else {
		util.Logger.Printf("WARNING: you have not configured locking in your .terragrunt file. Concurrent changes to your .tfstate files may cause conflicts!")
		return runCommand(cliContext)
	}
}

func runCommandWithLock(cliContext *cli.Context, lock locks.Lock) error {
	switch cliContext.Args().First() {
	case "apply", "destroy": return locks.WithLock(lock, func() error { return runCommand(cliContext) })
	case "release-lock": return runReleaseLockCommand(cliContext, lock)
	default: return runCommand(cliContext)
	}
}

func runCommand(cliContext *cli.Context) error {
	return shell.RunShellCommand("terraform", cliContext.Args()...)
}

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