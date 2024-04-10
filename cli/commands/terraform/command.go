package terraform

import (
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/gruntwork-cli/collections"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/terraform"
)

const (
	CommandName     = ""
	CommandHelpName = "*"
)

var (
	nativeTerraformCommands = []string{"apply", "console", "destroy", "env", "fmt", "get", "graph", "import", "init", "login", "logout", "metadata", "output", "plan", "providers", "push", "refresh", "show", "taint", "test", "version", "validate", "untaint", "workspace", "force-unlock", "state"}
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:     CommandName,
		HelpName: CommandHelpName,
		Usage:    "Terragrunt forwards all other commands directly to Terraform",
		Action:   action(opts),
	}
}

func action(opts *options.TerragruntOptions) cli.ActionFunc {
	return func(ctx *cli.Context) error {
		if opts.TerraformCommand == terraform.CommandNameDestroy {
			opts.CheckDependentModules = true
		}

		if !opts.DisableCommandValidation && !collections.ListContainsElement(nativeTerraformCommands, opts.TerraformCommand) {
			return errors.WithStackTrace(WrongTerraformCommand(opts.TerraformCommand))
		}

		return Run(ctx.Context, opts.OptionsFromContext(ctx))
	}
}
