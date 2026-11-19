package shared

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// ProviderCacheFlagName is the flag name for `--provider-cache`, which turns on the Terragrunt
	// Provider Cache.
	ProviderCacheFlagName = "provider-cache"
	// ProviderCacheDirFlagName is the flag name for `--provider-cache-dir`, the provider cache directory.
	ProviderCacheDirFlagName = "provider-cache-dir"
	// ProviderCacheHostnameFlagName is the flag name for `--provider-cache-hostname`, the hostname of the
	// provider cache server.
	ProviderCacheHostnameFlagName = "provider-cache-hostname"
	// ProviderCachePortFlagName is the flag name for `--provider-cache-port`, the port of the provider
	// cache server.
	ProviderCachePortFlagName = "provider-cache-port"
	// ProviderCacheTokenFlagName is the flag name for `--provider-cache-token`, the token clients present
	// to the provider cache server.
	ProviderCacheTokenFlagName = "provider-cache-token"
	// ProviderCacheRegistryNamesFlagName is the flag name for `--provider-cache-registry-names`, the
	// registries the provider cache server caches.
	ProviderCacheRegistryNamesFlagName = "provider-cache-registry-names"
	// NoAutoProviderCacheDirFlagName is the flag name for `--no-auto-provider-cache-dir`, which turns off
	// the auto-provider-cache-dir feature.
	NoAutoProviderCacheDirFlagName = "no-auto-provider-cache-dir"
)

// NewProviderCacheFlags creates the flags that configure the Terragrunt Provider Cache server and the
// automatic provider cache directory.
func NewProviderCacheFlags(opts *options.TerragruntOptions) clihelper.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	return clihelper.Flags{
		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        ProviderCacheFlagName,
				EnvVars:     tgPrefix.EnvVars(ProviderCacheFlagName),
				Destination: &opts.ProviderCacheOptions.Enabled,
				Usage:       "Enables Terragrunt's provider caching.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("provider-cache"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        ProviderCacheDirFlagName,
				EnvVars:     tgPrefix.EnvVars(ProviderCacheDirFlagName),
				Destination: &opts.ProviderCacheOptions.Dir,
				Usage:       "The path to the Terragrunt provider cache directory. By default, 'terragrunt/providers' folder in the user cache directory.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("provider-cache-dir"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        ProviderCacheTokenFlagName,
				EnvVars:     tgPrefix.EnvVars(ProviderCacheTokenFlagName),
				Destination: &opts.ProviderCacheOptions.Token,
				Usage:       "The token for authentication to the Terragrunt Provider Cache server. By default, assigned automatically.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("provider-cache-token"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        ProviderCacheHostnameFlagName,
				EnvVars:     tgPrefix.EnvVars(ProviderCacheHostnameFlagName),
				Destination: &opts.ProviderCacheOptions.Hostname,
				Usage:       "The hostname of the Terragrunt Provider Cache server. By default, 'localhost'.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("provider-cache-hostname"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[int]{
				Name:        ProviderCachePortFlagName,
				EnvVars:     tgPrefix.EnvVars(ProviderCachePortFlagName),
				Destination: &opts.ProviderCacheOptions.Port,
				Usage:       "The port of the Terragrunt Provider Cache server. By default, assigned automatically.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("provider-cache-port"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.SliceFlag[string]{
				Name:        ProviderCacheRegistryNamesFlagName,
				EnvVars:     tgPrefix.EnvVars(ProviderCacheRegistryNamesFlagName),
				Destination: &opts.ProviderCacheOptions.RegistryNames,
				Usage:       "The list of remote registries to cached by Terragrunt Provider Cache server. By default, 'registry.terraform.io', 'registry.opentofu.org'.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("provider-cache-registry-names"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoAutoProviderCacheDirFlagName,
			EnvVars:     tgPrefix.EnvVars(NoAutoProviderCacheDirFlagName),
			Destination: &opts.NoAutoProviderCacheDir,
			Usage:       "Disable the auto-provider-cache-dir feature even when the experiment is enabled.",
		}),
	}
}
