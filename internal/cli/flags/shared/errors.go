package shared

// AllGraphFlagsError is returned when both --all and --graph flags are used simultaneously.
type AllGraphFlagsError byte

func (err *AllGraphFlagsError) Error() string {
	return "Using the `--all` and `--graph` flags simultaneously is not supported."
}

// NoDiscoveryAuthProviderCmdRequiresExperimentError is returned when
// --no-discovery-auth-provider-cmd is set without the opt-out-auth experiment
// enabled.
type NoDiscoveryAuthProviderCmdRequiresExperimentError struct{}

func (NoDiscoveryAuthProviderCmdRequiresExperimentError) Error() string {
	return "--no-discovery-auth-provider-cmd requires the 'opt-out-auth' experiment to be enabled (e.g., --experiment=opt-out-auth)"
}
