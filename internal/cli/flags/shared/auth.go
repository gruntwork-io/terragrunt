package shared

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
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

// NewNoDiscoveryAuthProviderCmdFlag opts out of running the auth provider
// command during the discovery parse phase. Requires the opt-out-auth
// experiment; callers must validate that with [ValidateNoDiscoveryAuthProviderCmd]
// from the command's Before hook so the check is order-independent.
func NewNoDiscoveryAuthProviderCmdFlag(opts *options.TerragruntOptions, prefix flags.Prefix) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return flags.NewFlag(&clihelper.BoolFlag{
		Name:        NoDiscoveryAuthProviderCmdFlagName,
		EnvVars:     tgPrefix.EnvVars(NoDiscoveryAuthProviderCmdFlagName),
		Destination: &opts.DiscoveryAuthProviderCmd,
		Usage:       "Skip running --auth-provider-cmd during the discovery phase. Requires the 'opt-out-auth' experiment. Speeds up commands like 'run --all --queue-include-units-reading' at the cost of parse errors in units whose discovery-relevant blocks (exclude/include/dependency) require credentials.",
		Negative:    true,
	})
}

// ValidateNoDiscoveryAuthProviderCmd returns an error when the user set
// --no-discovery-auth-provider-cmd without enabling the opt-out-auth
// experiment. Call from a command's Before hook, by which point both flags
// (and the experiment registration) have been applied regardless of
// command-line ordering.
func ValidateNoDiscoveryAuthProviderCmd(opts *options.TerragruntOptions) error {
	if opts.DiscoveryAuthProviderCmd {
		return nil
	}

	if opts.Experiments.Evaluate(experiment.OptOutAuth) {
		return nil
	}

	return errors.New(NoDiscoveryAuthProviderCmdRequiresExperimentError{})
}
