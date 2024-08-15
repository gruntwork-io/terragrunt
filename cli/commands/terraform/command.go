// Package terraform provides a way to run OpenTofu/Terraform commands from Terragrunt.
package terraform

import (
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/gruntwork-cli/collections"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/terraform"
)

const (
	// CommandName is the name of this command. It's empty because it's the default command.
	CommandName = ""

	// CommandHelpName is the name of this command to be displayed in help.
	CommandHelpName = "*"
)

var (
	nativeTerraformCommands = []string{ //nolint:gochecknoglobals
		"apply",
		"console",
		"destroy",
		"env",
		"fmt",
		"get",
		"graph",
		"import",
		"init",
		"login",
		"logout",
		"metadata",
		"output",
		"plan",
		"providers",
		"push",
		"refresh",
		"show",
		"taint",
		"test",
		"version",
		"validate",
		"untaint",
		"workspace",
		"force-unlock",
		"state",
	}
)

// NewCommand builds a new command instance.
func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:     CommandName,
		HelpName: CommandHelpName,
		Usage:    "Terragrunt forwards all other commands directly to Terraform",
		Action:   Action(opts),
	}
}

// Action runs the command.
func Action(opts *options.TerragruntOptions) cli.ActionFunc {
	return func(ctx *cli.Context) error {
		if opts.TerraformCommand == terraform.CommandNameDestroy {
			opts.CheckDependentModules = true
		}

		isValidCommand := collections.ListContainsElement(nativeTerraformCommands, opts.TerraformCommand)

		if !opts.DisableCommandValidation && !isValidCommand {
			if strings.HasSuffix(opts.TerraformPath, "terraform") {
				return errors.WithStackTrace(WrongTerraformCommandError(opts.TerraformCommand))
			}

			// We default to tofu if the terraform path does not end in Terraform
			return errors.WithStackTrace(WrongTofuCommandError(opts.TerraformCommand))
		}

		return Run(ctx.Context, opts.OptionsFromContext(ctx))
	}
}
