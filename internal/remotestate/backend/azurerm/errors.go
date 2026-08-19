package azurerm

import (
	"errors"
	"fmt"
)

// MissingRequiredAzurermRemoteStateConfigError is returned when a required
// azurerm remote-state configuration key is absent.
type MissingRequiredAzurermRemoteStateConfigError string

func (configName MissingRequiredAzurermRemoteStateConfigError) Error() string {
	return "Missing required azurerm remote state configuration " + string(configName)
}

// CrossAccountMigrationError is returned when a state migration names a
// different storage account for the destination. The backend resolves
// credentials only for the source account, so it cannot write to another one.
// Match with errors.As.
type CrossAccountMigrationError struct {
	SrcStorageAccount string
	DstStorageAccount string
}

func (e *CrossAccountMigrationError) Error() string {
	return fmt.Sprintf(
		"cross-account state migration from storage account %q to %q is not supported by the azurerm backend "+
			"(it resolves credentials only for the source account); "+
			"migrate via separate init/pull/push or keep both units on the same storage account",
		e.SrcStorageAccount, e.DstStorageAccount,
	)
}

// CrossCloudMigrationError is returned when a state migration names the same
// storage account in two different Azure clouds. Storage account names are
// unique only within a cloud, and the blob client is built from the source
// config, so the copy would land in the source account and the source key
// would then be deleted. Match with errors.As.
type CrossCloudMigrationError struct {
	StorageAccount string
	// SrcEnvironment and DstEnvironment are the RESOLVED cloud identities, so
	// the message stays meaningful when the cloud came from ARM_ENVIRONMENT
	// rather than an `environment` config key.
	SrcEnvironment string
	DstEnvironment string
}

func (e *CrossCloudMigrationError) Error() string {
	return fmt.Sprintf(
		"cross-cloud state migration for storage account %q from environment %q to %q is not supported by the azurerm backend "+
			"(a storage account name identifies a different account in each Azure cloud); "+
			"migrate via separate init/pull/push, or keep both units in the same environment",
		e.StorageAccount, e.SrcEnvironment, e.DstEnvironment,
	)
}

// ErrBackendOptionsRequired is the panic value used when a lifecycle entry
// point is called without backend options, which the remote-state layer
// always supplies.
var ErrBackendOptionsRequired = errors.New("backend options are required")

// ErrStateClientSetup marks a client-construction failure, never an absent state blob. Match with errors.Is.
var ErrStateClientSetup = errors.New(
	"could not build a client for the azurerm state blob; verify resource_group_name, " +
		"storage_account_name, and subscription_id in the remote_state block name resources " +
		"that exist, and that the identity may read the storage account keys",
)

// ErrAzureBackendExperimentRequired is returned when an azurerm backend
// lifecycle operation is attempted without the `azure-backend` experiment
// enabled. Match with errors.Is.
var ErrAzureBackendExperimentRequired = errors.New(
	"the azurerm backend is experimental and requires the 'azure-backend' experiment to be enabled " +
		"(e.g. --experiment azure-backend or experiments = [\"azure-backend\"])",
)
