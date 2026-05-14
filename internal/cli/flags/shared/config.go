package shared

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	ConfigFlagName = "config"
)

// NewConfigFlag creates a flag for specifying the Terragrunt config file path.
func NewConfigFlag(opts *options.TerragruntOptions, prefix flags.Prefix) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	return flags.NewFlag(
		&clihelper.GenericFlag[string]{
			Name:        ConfigFlagName,
			EnvVars:     tgPrefix.EnvVars(ConfigFlagName),
			Destination: &opts.TerragruntConfigPath,
			Usage:       "The path to the Terragrunt config file. Default is terragrunt.hcl.",
		},
		flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("config"), opts.StrictControls),
	)
}
