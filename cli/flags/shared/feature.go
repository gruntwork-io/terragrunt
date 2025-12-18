package shared

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	FeatureFlagName = "feature"
)

// NewFeatureFlags defines the feature flag map that should be available to both `run` and `backend` commands.
func NewFeatureFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)

	return cli.Flags{
		flags.NewFlag(&cli.MapFlag[string, string]{
			Name:    FeatureFlagName,
			EnvVars: tgPrefix.EnvVars(FeatureFlagName),
			Usage:   "Set feature flags for the HCL code.",
			// Use default splitting behavior with comma separators via MapFlag defaults
			Action: func(_ *cli.Context, value map[string]string) error {
				for key, val := range value {
					opts.FeatureFlags.Store(key, val)
				}
				return nil
			},
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("feature"), terragruntPrefixControl),
		),
	}
}
