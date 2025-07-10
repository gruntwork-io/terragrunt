# Azure Interface-Based Architecture

This directory contains the interface-based architecture implementation for Azure services used by Terragrunt. This design improves testability, maintainability, and provides clean separation between Azure service contracts and their implementations.

## Architecture Overview

The interface-based architecture consists of the following components:

### 1. Interfaces (`internal/azure/interfaces/`)

Defines the contracts for all Azure service interactions:

- **`StorageAccountService`** - Storage account management operations including versioning control
- **`BlobService`** - Blob storage operations for state file management  
- **`ResourceGroupService`** - Resource group management operations
- **`AzureServiceContainer`** - Service factory interface for dependency injection

These interfaces capture only the methods needed by Terragrunt, providing a clean abstraction layer.

### 2. Production Implementations (`internal/azure/implementations/`)

Production implementations that wrap the actual Azure SDK:

- **`ProductionStorageAccountService`** - Implements StorageAccountService using Azure Storage SDK
- **`ProductionBlobService`** - Implements BlobService using Azure Storage SDK
- **`ProductionResourceGroupService`** - Implements ResourceGroupService using Azure Resource Manager SDK

These implementations provide the real Azure functionality for production use.

### 3. Service Factory (`internal/azure/factory/`)

The factory provides dependency injection and service creation:

```go
// Create factory
factory := factory.NewAzureServiceFactory()

// Factory implements AzureServiceContainer interface
var container interfaces.AzureServiceContainer = factory

// Create services with dependency injection
storageService, err := container.GetStorageAccountService(ctx, logger, config)
blobService, err := container.GetBlobService(ctx, logger, config)
resourceGroupService, err := container.GetResourceGroupService(ctx, logger, config)
```

### 4. Backend Integration (`internal/remotestate/backend/azurerm/backend.go`)

The Azure backend now uses interface-based dependency injection:

```go
// Backend constructor accepts service interfaces
backend := &Backend{
    storageAccountService: deps.StorageAccountService,
    blobService:          deps.BlobService,
    resourceGroupService: deps.ResourceGroupService,
}

// Uses interfaces throughout the implementation
exists, account, err := backend.storageAccountService.GetStorageAccount(ctx, resourceGroup, accountName)
```

## Key Features

### 1. **Interface-Based Dependency Injection**

- Clean separation between service contracts and implementations
- Easy to test with mock implementations
- Production services use real Azure SDK clients
- Factory pattern for service creation

### 2. **Version Control Integration**

- Storage account blob versioning detection and management
- `IsVersioningEnabled` method on StorageAccountService
- Automatic versioning configuration during bootstrap

### 3. **Comprehensive Error Handling**

- Structured error types in `internal/azure/errors/`
- Error classification and telemetry integration
- Consistent error handling patterns across all services

### 4. **Type Safety**

- Strong typing with custom types in `internal/azure/types/`
- Clear configuration structures for different Azure resources
- Type conversions between Azure SDK types and internal types

### 5. **Testing Infrastructure**

- Unit tests for pure functions and data structures
- Mock implementations for integration testing
- Comprehensive test coverage in `*_test.go` files

## Usage Examples

### Creating Services with Factory

```go
// Production usage with factory
factory := factory.NewAzureServiceFactory()

// Get services through factory interface
storageService, err := factory.GetStorageAccountService(ctx, logger, config)
if err != nil {
    return err
}

blobService, err := factory.GetBlobService(ctx, logger, config)
if err != nil {
    return err
}

// Use the services
exists, account, err := storageService.GetStorageAccount(ctx, resourceGroup, accountName)
if err != nil {
    return err
}

// Check versioning
versioningEnabled, err := storageService.IsVersioningEnabled(ctx)
if err != nil {
    return err
}
```

### Backend Integration Pattern

```go
// Backend uses interfaces for all Azure operations
type Backend struct {
    storageAccountService interfaces.StorageAccountService
    blobService          interfaces.BlobService
    resourceGroupService interfaces.ResourceGroupService
    // other fields...
}

// Constructor accepts interfaces
func NewBackend(
    storageAccountService interfaces.StorageAccountService,
    blobService interfaces.BlobService,
    resourceGroupService interfaces.ResourceGroupService,
) *Backend {
    return &Backend{
        storageAccountService: storageAccountService,
        blobService:          blobService,
        resourceGroupService: resourceGroupService,
    }
}
```

### Testing with Mocks

```go
// In test files, use mock implementations
func TestSomeFunction(t *testing.T) {
    // Create mock services (see testing/mock_services.go)
    mockStorage := &MockStorageAccountService{
        GetStorageAccountFunc: func(ctx context.Context, resourceGroup, accountName string) (*armstorage.Account, error) {
            // Return test data
            return &armstorage.Account{Name: &accountName}, nil
        },
        IsVersioningEnabledFunc: func(ctx context.Context) (bool, error) {
            return true, nil
        },
    }

    // Use mock in your code
    result, err := someFunction(mockStorage)
    
    // Verify behavior
    assert.NoError(t, err)
    assert.Equal(t, 1, len(mockStorage.GetStorageAccountCalls))
}
```

## Interface Definitions

### StorageAccountService

```go
type StorageAccountService interface {
    GetStorageAccount(ctx context.Context, resourceGroup, accountName string) (*armstorage.Account, error)
    CreateStorageAccountIfNecessary(ctx context.Context, l log.Logger, config azurehelper.StorageAccountConfig) error
    DeleteStorageAccount(ctx context.Context, l log.Logger) error
    IsVersioningEnabled(ctx context.Context) (bool, error)
    EnableStorageAccountVersioning(ctx context.Context, l log.Logger) error
    DisableStorageAccountVersioning(ctx context.Context, l log.Logger) error
    EnsureResourceGroup(ctx context.Context, l log.Logger, location string) error
    AssignStorageBlobDataOwnerRole(ctx context.Context, l log.Logger) error
    IsPermissionError(err error) bool
}
```

### BlobService

```go
type BlobService interface {
    GetObject(ctx context.Context, input *azurehelper.GetObjectInput) (*azurehelper.GetObjectOutput, error)
    ContainerExists(ctx context.Context, containerName string) (bool, error)
    CreateContainerIfNecessary(ctx context.Context, l log.Logger, containerName string) error
    DeleteContainer(ctx context.Context, l log.Logger, containerName string) error
    DeleteBlobIfNecessary(ctx context.Context, l log.Logger, containerName string, blobName string) error
    UploadBlob(ctx context.Context, l log.Logger, containerName, blobName string, data []byte) error
    CopyBlobToContainer(ctx context.Context, srcContainer, srcKey string, dstClient *azurehelper.BlobServiceClient, dstContainer, dstKey string) error
}
```

### ResourceGroupService

```go
type ResourceGroupService interface {
    EnsureResourceGroup(ctx context.Context, l log.Logger, resourceGroupName, location string, tags map[string]string) error
    ResourceGroupExists(ctx context.Context, resourceGroupName string) (bool, error)
    DeleteResourceGroup(ctx context.Context, l log.Logger, resourceGroupName string) error
    GetResourceGroup(ctx context.Context, resourceGroupName string) (*armresources.ResourceGroup, error)
}
```

### AzureServiceContainer

```go
type AzureServiceContainer interface {
    GetStorageAccountService(ctx context.Context, logger log.Logger, config interface{}) (StorageAccountService, error)
    GetBlobService(ctx context.Context, logger log.Logger, config interface{}) (BlobService, error)
    GetResourceGroupService(ctx context.Context, logger log.Logger, config interface{}) (ResourceGroupService, error)
}
```

## Directory Structure

```text
internal/azure/
├── interfaces/           # Service interface definitions
│   ├── storage.go       # StorageAccountService, BlobService interfaces
│   └── interfaces_test.go # Interface tests
├── implementations/     # Production implementations
│   ├── production.go    # Production service implementations
│   └── production_test.go # Production implementation tests
├── factory/            # Service factory and dependency injection
│   ├── factory.go      # AzureServiceFactory implementation
│   ├── adapters.go     # SDK adapter functions
│   └── factory_test.go # Factory tests
├── types/              # Type definitions and conversions
│   ├── storage_account.go # StorageAccount type definitions
│   ├── storage_types.go   # Storage-related types
│   ├── blob_types.go      # Blob-related types
│   └── types_test.go      # Type tests
├── errors/             # Error handling and classification
│   ├── types.go        # Error types and classification
│   └── types_test.go   # Error handling tests
├── azureutil/          # Utility functions and helpers
│   ├── types.go        # Utility types and error classification
│   ├── errors.go       # Error creation helpers
│   ├── errorhandling.go # Error handling utilities
│   └── *_test.go       # Utility tests
├── azurehelper/        # Legacy Azure helper (being phased out)
│   └── azure_storage_account.go # Legacy storage account helpers
├── remotestate/        # Remote state backend integration
│   └── backend/azurerm/testing/mock_services.go # Mock services for testing
├── README.md           # This file
└── CONFIGURATION.md    # Configuration guide
```

## Migration from Legacy Implementation

The interface-based architecture replaces the previous direct Azure SDK usage:

### Before (Legacy)

```go
// Direct SDK usage
client := azurehelper.NewStorageAccountClient(config)
exists, account, err := client.StorageAccountExists(ctx)
```

### After (Interface-Based)

```go
// Interface-based usage
factory := factory.NewAzureServiceFactory()
service, err := factory.GetStorageAccountService(ctx, logger, config)
account, err := service.GetStorageAccount(ctx, resourceGroup, accountName)
```

### Key Changes

1. **Versioning moved to StorageAccountService**: `IsVersioningEnabled` is now on StorageAccountService instead of BlobService
2. **Method signature changes**: `GetStorageAccount` replaces `StorageAccountExists` with clearer semantics
3. **Factory pattern**: Services are created through factory interface
4. **Dependency injection**: Backend accepts service interfaces instead of creating clients directly

## Testing

The interface-based design significantly improves testing:

### Unit Tests

- Pure function tests in `*_test.go` files
- Type conversion and validation tests
- Error handling and classification tests

### Integration Tests

- Mock implementations for controlled testing
- Real Azure service testing for end-to-end validation
- Test helpers in `testing/` directories

### Test Coverage

- All major internal/azure packages have comprehensive unit tests
- Interface definitions have validation tests
- Error handling has extensive test coverage

Run tests:

```bash
# Run all Azure internal tests
go test ./internal/azure/...

# Run specific package tests
go test ./internal/azure/interfaces/
go test ./internal/azure/factory/
go test ./internal/azure/types/
```
