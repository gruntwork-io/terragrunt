// Package validateinputs provides the command to validate the inputs of a Terragrunt configuration file
// against the variables defined in OpenTofu/Terraform configuration files.
package validateinputs

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "validate-inputs"

	StrictValidateFlagName = "strict-validate"

	DeprecatedStrictValidateFlagName = "strict-validate"
)

func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName)

	return cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        StrictValidateFlagName,
			EnvVars:     tgPrefix.EnvVars(StrictValidateFlagName),
			Destination: &opts.ValidateStrict,
			Usage:       "Sets strict mode for the validate-inputs command. By default, strict mode is off. When this flag is passed, strict mode is turned on. When strict mode is turned off, the validate-inputs command will only return an error if required inputs are missing from all input sources (env vars, var files, etc). When strict mode is turned on, an error will be returned if required inputs are missing OR if unused variables are passed to Terragrunt.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedStrictValidateFlagName), terragruntPrefixControl)),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmd := &cli.Command{
		Name:   CommandName,
		Usage:  "Checks if the terragrunt configured inputs align with the terraform defined variables.",
		Flags:  append(run.NewFlags(opts, nil), NewFlags(opts, nil)...),
		Action: func(ctx *cli.Context) error { return Run(ctx, opts.OptionsFromContext(ctx)) },
	}

	cmd = runall.WrapCommand(opts, cmd)
	cmd = graph.WrapCommand(opts, cmd)

	return cmd
}
