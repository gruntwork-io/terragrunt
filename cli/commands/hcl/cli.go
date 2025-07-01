// Package hcl provides commands for formatting and validating HCL configurations.
package hcl

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/validate"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const CommandName = "hcl"

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:        CommandName,
		Usage:       "Interact with HCL files.",
		Description: "Interact with Terragrunt files written in HashiCorp Configuration Language (HCL).",
		Subcommands: cli.Commands{
			format.NewCommand(l, opts),
			validate.NewCommand(l, opts),
		},
		Action: cli.ShowCommandHelp,
	}
}
