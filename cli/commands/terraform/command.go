package terraform

import (
	"github.com/gruntwork-io/go-commons/collections"
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

		if !collections.ListContainsElement(nativeTerraformCommands, opts.TerraformCommand) {
			return errors.Errorf("Terraform has no command named %q. To see all of Terraform's top-level commands, run: terraform -help", opts.TerraformCommand)
		}

		return Run(opts.OptionsFromContext(ctx))
	}
}
