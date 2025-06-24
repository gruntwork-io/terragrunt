package azurerm

import (
	"fmt"
)

// MissingRequiredAzureRemoteStateConfig represents a missing required configuration parameter for Azure remote state.
type MissingRequiredAzureRemoteStateConfig string

// Error returns a string indicating that the Azure remote state configuration is missing a required parameter.
// The returned error message will include the name of the missing configuration parameter.
func (configName MissingRequiredAzureRemoteStateConfig) Error() string {
	return "missing required Azure remote state configuration " + string(configName)
}

// MaxRetriesWaitingForContainerExceeded represents an error when the maximum number of retries is exceeded
// while waiting for an Azure Storage container to become available.
type MaxRetriesWaitingForContainerExceeded string

// Error returns a string indicating that the maximum number of retries was exceeded
// while waiting for an Azure Storage container to become available.
func (err MaxRetriesWaitingForContainerExceeded) Error() string {
	return "Exceeded max retries waiting for Azure Storage container " + string(err)
}

// ContainerDoesNotExist represents an error when an Azure Storage container does not exist.
type ContainerDoesNotExist struct {
	Underlying    error
	ContainerName string
}

// Error returns a string indicating that an Azure Storage container does not exist,
// along with the underlying error details.
func (err ContainerDoesNotExist) Error() string {
	return fmt.Sprintf("Container %s does not exist. Underlying error: %v", err.ContainerName, err.Underlying)
}
