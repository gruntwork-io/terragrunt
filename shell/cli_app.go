package shell

import (
	"github.com/urfave/cli"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/locks"
	"fmt"
)

const CUSTOM_USAGE_TEXT = `DESCRIPTION:
   {{.Name}} - {{.UsageText}}

USAGE:
   {{.Usage}}
{{if .VisibleCommands}}
COMMANDS:{{range .VisibleCategories}}{{if .Name}}
   {{.Name}}:{{end}}{{range .VisibleCommands}}
   {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}
{{end}}{{end}}{{if .VisibleFlags}}
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
	app.Usage = "terragrunt <COMMAND>"
	app.UsageText = `Terragrunt is a thin wrapper for the Terraform client that provides simple locking mechanisms
   so that multiple people can collaborate on the same Terraform state without overwriting each other's changes. The
   supported locking mechanisms are Git and DynamoDB. You can configure the locking mechanisms using a .terragrunt file
   in the current directory.

   Terragrunt supports all the same commands as Terraform (e.g. plan, apply, destroy, etc). To see all available commands,
   run: terraform --help`


	app.Commands = []cli.Command{
		{
			Name:      	"apply",
			Usage:     	"Aquire a lock and run `terraform apply`",
			Action:    	runTerraformCommandWithLock,
		},
		{
			Name:      	"destroy",
			Usage:     	"Aquire a lock and run `terraform destroy`",
			Action:    	runTerraformCommandWithLock,
		},
		{
			Name:      	"release-lock",
			Usage:     	"Release a lock that is left over from some previous command",
			Action:        releaseLockCommand,
		},
		{
			Name:      	"*",
			Usage:     	"Terragrunt forwards all other commands directly to Terraform",
			Action: 	runTerraformCommand,
		},
	}

	app.Action = runTerraformCommand

	return app
}

func runTerraformCommandWithLock(cliContext *cli.Context) error {
	lock, err := config.GetLockForConfig()
	if err != nil {
		return err
	}

	return locks.WithLock(lock, func() error {
		return runTerraformCommand(cliContext)
	})
}

func runTerraformCommand(cliContext *cli.Context) error {
	return RunShellCommand("terraform", cliContext.Args()...)
}

func releaseLockCommand(cliContext *cli.Context) error {
	lock, err := config.GetLockForConfig()
	if err != nil {
		return err
	}

	proceed, err := PromptUserForYesNo(fmt.Sprintf("Are you sure you want to release lock %s?", lock))
	if err != nil {
		return err
	}

	if proceed {
		return lock.ReleaseLock()
	} else {
		return nil
	}
}

