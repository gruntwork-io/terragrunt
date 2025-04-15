// Package hcl provides commands for formatting and validating HCL configurations.
package hcl

import (
	// "github.com/gruntwork-io/terragrunt/cli/commands/backend/migrate"

	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/options"
)

const CommandName = "hcl"

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:        CommandName,
		Usage:       "Interact with HCL files.",
		Description: "Interact with Terragrunt files written in HashiCorp Configuration Language (HCL).",
		Subcommands: cli.Commands{
			format.NewCommand(opts),
		},
		ErrorOnUndefinedFlag: true,
		Before: func(ctx *cli.Context) error {
			if !opts.Experiments.Evaluate(experiment.CLIRedesign) {
				return cli.NewExitError(errors.Errorf("requires that the %[1]s experiment is enabled. e.g. --experiment %[1]s", experiment.CLIRedesign), cli.ExitCodeGeneralError)
			}

			return nil
		},
		Action: cli.ShowCommandHelp,
	}
}
