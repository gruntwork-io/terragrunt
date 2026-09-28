package shared

import "github.com/gruntwork-io/terragrunt/internal/experiment"

// AllGraphFlagsError is returned when both --all and --graph flags are used simultaneously.
type AllGraphFlagsError byte

func (err *AllGraphFlagsError) Error() string {
	return "Using the `--all` and `--graph` flags simultaneously is not supported."
}

// CASOfflineRefreshFlagsError is returned when both --cas-offline and --cas-refresh flags are used simultaneously.
type CASOfflineRefreshFlagsError byte

func (err *CASOfflineRefreshFlagsError) Error() string {
	return "Using the `--cas-offline` and `--cas-refresh` flags simultaneously is not supported: " +
		"offline mode answers from the persisted probe cache that refresh ignores."
}

// CASExperimentRequiredError is returned when a CAS probe cache flag is set without the offline-cas experiment enabled.
type CASExperimentRequiredError struct {
	FlagName string
}

func (err *CASExperimentRequiredError) Error() string {
	return "--" + err.FlagName + " requires the '" + experiment.OfflineCAS +
		"' experiment to be enabled (e.g., --experiment=" + experiment.OfflineCAS + ")"
}
