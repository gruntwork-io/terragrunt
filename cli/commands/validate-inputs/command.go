// Package validateinputs provides the command to validate the inputs of a Terragrunt configuration file.
package validateinputs

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	// CommandName is the name of the command.
	CommandName = "validate-inputs"

	// FlagTerragruntStrictValidate is the name of the flag to enable strict mode for the validate-inputs command.
	FlagTerragruntStrictValidate = "terragrunt-strict-validate"
)

// NewFlags returns the flags for the validate-inputs command.
func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		&cli.BoolFlag{
			Name:        FlagTerragruntStrictValidate,
			Destination: &opts.ValidateStrict,
			Usage:       "Sets strict mode for the validate-inputs command. By default, strict mode is off. When this flag is passed, strict mode is turned on. When strict mode is turned off, the validate-inputs command will only return an error if required inputs are missing from all input sources (env vars, var files, etc). When strict mode is turned on, an error will be returned if required inputs are missing OR if unused variables are passed to Terragrunt.", //nolint:lll
		},
	}
}

// NewCommand returns the validate-inputs command.
func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Checks if the terragrunt configured inputs align with the terraform defined variables.",
		Flags:  NewFlags(opts).Sort(),
		Action: func(ctx *cli.Context) error { return Run(ctx, opts.OptionsFromContext(ctx)) },
	}
}
