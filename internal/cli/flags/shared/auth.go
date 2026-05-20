package shared

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	AuthProviderCmdFlagName            = "auth-provider-cmd"
	NoDiscoveryAuthProviderCmdFlagName = "no-discovery-auth-provider-cmd"
)

// NewAuthProviderCmdFlag creates a flag for specifying the auth provider command.
func NewAuthProviderCmdFlag(opts *options.TerragruntOptions, prefix flags.Prefix, commandName string) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	var terragruntPrefixControl flags.RegisterStrictControlsFunc
	if commandName != "" {
		terragruntPrefixControl = flags.StrictControlsByCommand(opts.StrictControls, commandName)
	} else {
		terragruntPrefixControl = flags.StrictControlsByGlobalFlags(opts.StrictControls)
	}

	return flags.NewFlag(
		&clihelper.GenericFlag[string]{
			Name:        AuthProviderCmdFlagName,
			EnvVars:     tgPrefix.EnvVars(AuthProviderCmdFlagName),
			Destination: &opts.AuthProviderCmd,
			Usage:       "Run the provided command and arguments to authenticate Terragrunt dynamically when necessary.",
		},
		flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("auth-provider-cmd"), terragruntPrefixControl),
	)
}

// NewNoDiscoveryAuthProviderCmdFlag opts out of running --auth-provider-cmd
// during the discovery parse phase. Setting it without the opt-out-auth
// experiment returns ErrNoDiscoveryAuthProviderCmdRequiresExperiment.
func NewNoDiscoveryAuthProviderCmdFlag(opts *options.TerragruntOptions, prefix flags.Prefix) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return flags.NewFlag(&clihelper.BoolFlag{
		Name:        NoDiscoveryAuthProviderCmdFlagName,
		EnvVars:     tgPrefix.EnvVars(NoDiscoveryAuthProviderCmdFlagName),
		Destination: &opts.DiscoveryAuthProviderCmd,
		Usage:       "Skip running --auth-provider-cmd during the discovery phase. Requires the 'opt-out-auth' experiment.",
		Negative:    true,
		Action: func(_ context.Context, _ *clihelper.Context, runDiscoveryAuth bool) error {
			if runDiscoveryAuth {
				return nil
			}

			if opts.Experiments.Evaluate(experiment.OptOutAuth) {
				return nil
			}

			return ErrNoDiscoveryAuthProviderCmdRequiresExperiment
		},
	})
}
