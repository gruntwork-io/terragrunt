// Package hcl provides commands for formatting and validating HCL configurations.
package hcl

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/validate"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const CommandName = "hcl"

func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *clihelper.Command {
	return &clihelper.Command{
		Name:        CommandName,
		Usage:       "Interact with HCL files.",
		Description: "Interact with Terragrunt files written in HashiCorp Configuration Language (HCL).",
		Subcommands: clihelper.Commands{
			format.NewCommand(l, opts, v),
			validate.NewCommand(l, opts, v),
		},
		Before: func(_ context.Context, _ *clihelper.Context) error {
			// No `hcl` subcommand runs the OpenTofu/Terraform binary, so skip RunAction's version probe for all of them.
			opts.NoAutoProviderCacheDir = true

			return nil
		},
		Action: clihelper.ShowCommandHelp,
	}
}
