// Package hcl provides commands for formatting and validating HCL configurations.
package hcl

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/validate"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const CommandName = "hcl"

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmdPrefix := flags.Name{CommandName}

	return &cli.Command{
		Name:        CommandName,
		Usage:       "Interact with HCL files.",
		Description: "Interact with Terragrunt files written in HashiCorp Configuration Language (HCL).",
		Subcommands: cli.Commands{
			format.NewCommand(opts, cmdPrefix),
			validate.NewCommand(opts, cmdPrefix),
		},
		Action: cli.ShowCommandHelp,
	}
}
