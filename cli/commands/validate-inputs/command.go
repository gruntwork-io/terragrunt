package validateinputs

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "validate-inputs"

	FlagTerragruntStrictValidate = "terragrunt-strict-validate"
)

func NewCommand(globalOpts *options.TerragruntOptions) *cli.Command {
	opts := NewOptions(globalOpts)

	command := &cli.Command{
		Name:  CommandName,
		Usage: "Checks if the terragrunt configured inputs align with the terraform defined variables.",
		// Action: func(ctx *cli.Context) error { return Run(opts) },
	}

	command.AddFlags(
		&cli.BoolFlag{
			Name:        FlagTerragruntStrictValidate,
			Destination: &opts.ValidateStrict,
			Usage:       "Sets strict mode for the validate-inputs command. By default, strict mode is off. When this flag is passed, strict mode is turned on. When strict mode is turned off, the validate-inputs command will only return an error if required inputs are missing from all input sources (env vars, var files, etc). When strict mode is turned on, an error will be returned if required inputs are missing OR if unused variables are passed to Terragrunt.",
		},
	)

	return command
}
