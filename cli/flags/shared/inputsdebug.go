package shared

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	InputsDebugFlagName = "inputs-debug"
)

// NewInputsDebugFlag creates a flag for enabling inputs debug output.
func NewInputsDebugFlag(opts *options.TerragruntOptions, prefix flags.Prefix, commandName string) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	var terragruntPrefixControl flags.RegisterStrictControlsFunc
	if commandName != "" {
		terragruntPrefixControl = flags.StrictControlsByCommand(opts.StrictControls, commandName)
	} else {
		terragruntPrefixControl = flags.StrictControlsByGlobalFlags(opts.StrictControls)
	}

	return flags.NewFlag(
		&cli.BoolFlag{
			Name:        InputsDebugFlagName,
			EnvVars:     tgPrefix.EnvVars(InputsDebugFlagName),
			Destination: &opts.Debug,
			Usage:       "Write debug.tfvars to working folder to help root-cause issues.",
		},
		flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("debug"), terragruntPrefixControl),
	)
}
