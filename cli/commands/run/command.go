// Package run contains the logic for interacting with OpenTofu/Terraform.
package run

import (
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf"
)

const (
	CommandName = "run"
)

var (
	nativeTerraformCommands = []string{"apply", "console", "destroy", "env", "fmt", "get", "graph", "import", "init", "login", "logout", "metadata", "output", "plan", "providers", "push", "refresh", "show", "taint", "test", "version", "validate", "untaint", "workspace", "force-unlock", "state"}
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                 CommandName,
		Usage:                "Terragrunt forwards all other commands directly to Terraform",
		Flags:                NewFlags(opts),
		ErrorOnUndefinedFlag: true,
		Action:               Action(opts),
	}
}

func Action(opts *options.TerragruntOptions) cli.ActionFunc {
	return func(ctx *cli.Context) error {
		if opts.TerraformCommand == tf.CommandNameDestroy {
			opts.CheckDependentModules = !opts.NoDestroyDependenciesCheck
		}

		if !opts.DisableCommandValidation && !collections.ListContainsElement(nativeTerraformCommands, opts.TerraformCommand) {
			if strings.HasSuffix(opts.TerraformPath, "terraform") {
				return errors.New(WrongTerraformCommand(opts.TerraformCommand))
			} else {
				// We default to tofu if the terraform path does not end in Terraform
				return errors.New(WrongTofuCommand(opts.TerraformCommand))
			}
		}

		return Run(ctx.Context, opts.OptionsFromContext(ctx))
	}
}
