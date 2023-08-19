package terraform

import (
	"strings"

	"github.com/gruntwork-io/gruntwork-cli/collections"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/pkg/errors"
)

const (
	CommandName = "terraform"
)

var (
	nativeTerraformCommands = []string{"apply", "console", "destroy", "env", "env list", "env select", "env new", "env delete", "fmt", "get", "graph", "import", "init", "metadata", "metadata functions", "output", "plan", "providers", "providers lock", "providers mirror", "providers schema", "push", "refresh", "show", "taint", "test", "validate", "untaint", "workspace", "workspace list", "workspace select", "workspace show", "workspace new", "workspace delete", "force-unlock", "state", "state list", "state rm", "state mv", "state pull", "state push", "state show", "state replace-provider"}
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:     CommandName,
		HelpName: "*",
		Usage:    "Terragrunt forwards all other commands directly to Terraform",
		Action:   action(opts),
	}
}

func action(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
	return func(ctx *cli.Context) error {
		if opts.TerraformCommand == CommandNameDestroy {
			opts.CheckDependentModules = true
		}

		if err := validateTerraformCommand(opts.TerraformCliArgs); err != nil {
			return err
		}

		return Run(opts.OptionsFromContext(ctx))
	}
}

func validateTerraformCommand(args []string) error {
	var commandArgs []string

	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			break
		}
		commandArgs = append(commandArgs, arg)
	}
	command := strings.Join(commandArgs, " ")

	if !collections.ListContainsElement(nativeTerraformCommands, command) {
		return errors.Errorf("Terraform has no command named %q. To see all of Terraform's top-level commands, run: terraform -help", command)
	}

	return nil
}
