package shared

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// AuthProviderCmdFlagName is the flag name for `--auth-provider-cmd`, a command whose output supplies
	// credentials to Terragrunt.
	AuthProviderCmdFlagName = "auth-provider-cmd"
	// NoDiscoveryAuthProviderCmdFlagName is the flag name for `--no-discovery-auth-provider-cmd`, which
	// skips `--auth-provider-cmd` while discovery parses units.
	NoDiscoveryAuthProviderCmdFlagName = "no-discovery-auth-provider-cmd"
)

// NewAuthProviderCmdFlag creates a flag for specifying the auth provider command.
func NewAuthProviderCmdFlag(opts *options.TerragruntOptions, prefix flags.Prefix) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	return flags.NewFlag(
		&clihelper.GenericFlag[string]{
			Name:        AuthProviderCmdFlagName,
			EnvVars:     tgPrefix.EnvVars(AuthProviderCmdFlagName),
			Destination: &opts.AuthProviderCmd,
			Usage:       "Run the provided command and arguments to authenticate Terragrunt dynamically when necessary.",
		},
		flags.WithDeprecatedEnvVars(
			terragruntPrefix.EnvVars("auth-provider-cmd"),
			opts.StrictControls,
		),
	)
}

// NewNoDiscoveryAuthProviderCmdFlag opts out of running --auth-provider-cmd
// during the discovery parse phase.
func NewNoDiscoveryAuthProviderCmdFlag(
	opts *options.TerragruntOptions,
	prefix flags.Prefix,
) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return flags.NewFlag(&clihelper.BoolFlag{
		Name:        NoDiscoveryAuthProviderCmdFlagName,
		EnvVars:     tgPrefix.EnvVars(NoDiscoveryAuthProviderCmdFlagName),
		Destination: &opts.DiscoveryAuthProviderCmd,
		Usage:       "Skip running --auth-provider-cmd during the discovery phase.",
		Negative:    true,
	})
}
