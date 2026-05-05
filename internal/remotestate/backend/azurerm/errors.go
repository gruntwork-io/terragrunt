package azurerm

import "fmt"

// MissingRequiredAzureRMRemoteStateConfig is returned when a required
// configuration field is missing from the azurerm backend block.
type MissingRequiredAzureRMRemoteStateConfig string

func (configName MissingRequiredAzureRMRemoteStateConfig) Error() string {
	return "Missing required AzureRM remote state configuration " + string(configName)
}

// ExperimentNotEnabledError is returned when an azurerm backend lifecycle
// operation is attempted while the azure-backend experiment is disabled.
type ExperimentNotEnabledError struct{}

func (ExperimentNotEnabledError) Error() string {
	return fmt.Sprintf(
		"the azurerm backend is experimental; enable it with experiments = [%q] in your terragrunt config",
		"azure-backend",
	)
}
