// Package hcl provides commands for formatting and validating HCL configurations.
package hcl

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/validate"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// CommandName is the name of the `terragrunt hcl` command.
const CommandName = "hcl"

// NewCommand returns the `terragrunt hcl` command, which groups the format and
// validate subcommands and prints help when run alone.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv, s *rewrite.Store) *clihelper.Command {
	return &clihelper.Command{
		Name:        CommandName,
		Usage:       "Interact with HCL files.",
		Description: "Interact with Terragrunt files written in HashiCorp Configuration Language (HCL).",
		Subcommands: clihelper.Commands{
			format.NewCommand(l, opts, v),
			validate.NewCommand(l, opts, v, s),
		},
		Action: clihelper.ShowCommandHelp,
	}
}
