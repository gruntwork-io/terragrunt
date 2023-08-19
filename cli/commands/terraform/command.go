package terraform

import (
	"github.com/gruntwork-io/gruntwork-cli/collections"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/pkg/errors"
)

const (
	CommandName = "terraform"
)

var (
	nativeTerraformCommands = []string{"apply", "console", "destroy", "env", "fmt", "get", "graph", "import", "init", "metadata", "output", "plan", "providers", "push", "refresh", "show", "taint", "test", "version", "validate", "untaint", "workspace", "force-unlock", "state"}
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
