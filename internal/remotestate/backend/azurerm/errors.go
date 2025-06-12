package azurerm

import (
	"fmt"
)

type MissingRequiredAzureRemoteStateConfig string

func (configName MissingRequiredAzureRemoteStateConfig) Error() string {
	return "Missing required Azure remote state configuration " + string(configName)
}

type MaxRetriesWaitingForContainerExceeded string

func (err MaxRetriesWaitingForContainerExceeded) Error() string {
	return "Exceeded max retries waiting for Azure Storage container " + string(err)
}

type ContainerDoesNotExist struct {
	Underlying    error
	ContainerName string
}

func (err ContainerDoesNotExist) Error() string {
	return fmt.Sprintf("Container %s does not exist. Underlying error: %v", err.ContainerName, err.Underlying)
}
