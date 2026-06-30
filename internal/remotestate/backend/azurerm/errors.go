package azurerm

import "errors"

// MissingRequiredAzurermRemoteStateConfigError is returned when a required
// azurerm remote-state configuration key is absent.
type MissingRequiredAzurermRemoteStateConfigError string

func (configName MissingRequiredAzurermRemoteStateConfigError) Error() string {
	return "Missing required azurerm remote state configuration " + string(configName)
}

// ErrAzureBackendExperimentRequired is returned when an azurerm backend
// lifecycle operation is attempted without the `azure-backend` experiment
// enabled. Match with errors.Is.
var ErrAzureBackendExperimentRequired = errors.New(
	"the azurerm backend is experimental and requires the 'azure-backend' experiment to be enabled " +
		"(e.g. --experiment azure-backend or experiments = [\"azure-backend\"])",
)
