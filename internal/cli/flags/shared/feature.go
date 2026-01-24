package shared

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	FeatureFlagName = "feature"
)

// NewFeatureFlags defines the feature flag map that should be available to both `run` and `backend` commands.
func NewFeatureFlags(opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)

	return clihelper.Flags{
		flags.NewFlag(&clihelper.MapFlag[string, string]{
			Name:    FeatureFlagName,
			EnvVars: tgPrefix.EnvVars(FeatureFlagName),
			Usage:   "Set feature flags for the HCL code.",
			// Use default splitting behavior with comma separators via MapFlag defaults
			Action: func(_ context.Context, _ *clihelper.Context, value map[string]string) error {
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
