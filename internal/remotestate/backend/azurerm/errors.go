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
	return fmt.Sprintf("Exceeded max retries waiting for Azure Storage container %s", string(err))
}

type ContainerDoesNotExist struct {
	ContainerName string
	Underlying   error
}

func (err ContainerDoesNotExist) Error() string {
	return fmt.Sprintf("Container %s does not exist. Underlying error: %v", err.ContainerName, err.Underlying)
}
