// Package validateinputs provides the command to validate the inputs of a Terragrunt configuration file
// against the variables defined in OpenTofu/Terraform configuration files.
package validateinputs

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/validate"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "validate-inputs"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmd := &cli.Command{
		Name:  CommandName,
		Usage: "Checks if the terragrunt configured inputs align with the terraform defined variables.",
		Flags: append(run.NewFlags(opts, nil), validate.NewFlags(opts, nil).Filter(validate.StrictFlagName)...),
		Action: func(ctx *cli.Context) error {
			opts.HCLValidateInput = true

			return validate.Run(ctx, opts.OptionsFromContext(ctx))
		},
	}

	cmd = runall.WrapCommand(opts, cmd)
	cmd = graph.WrapCommand(opts, cmd)

	return cmd
}
