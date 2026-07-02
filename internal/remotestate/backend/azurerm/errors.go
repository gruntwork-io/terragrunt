package azurerm

import (
	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
)

// MissingRequiredAzureRMRemoteStateConfig is returned when a required
// configuration field is missing from the azurerm backend block.
type MissingRequiredAzureRMRemoteStateConfig string

func (configName MissingRequiredAzureRMRemoteStateConfig) Error() string {
	return "missing required AzureRM remote state configuration " + string(configName)
}

// ExperimentNotEnabledError is returned when an azurerm backend lifecycle
// operation is attempted while the azure-backend experiment is disabled.
type ExperimentNotEnabledError struct{}

func (ExperimentNotEnabledError) Error() string {
	return `the azurerm backend is experimental; enable it with experiments = ["` + experiment.AzureBackend + `"] in your terragrunt config`
}

// ControlPlaneUnavailableError is returned by control-plane operations
// (storage account, resource group, RBAC management) when the resolved
// auth method does not carry a token credential. SAS-token and access-key
// auth are data-plane only — they can read and write blobs but cannot
// reach ARM. Match with errors.As.
type ControlPlaneUnavailableError struct {
	Method    azurehelper.AuthMethod
	Operation string
}

func (e *ControlPlaneUnavailableError) Error() string {
	return e.Operation + " requires a token credential; auth method " + string(e.Method) + " is data-plane only"
}
