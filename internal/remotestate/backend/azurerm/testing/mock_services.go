// Package testing provides mock implementations for Azure services used in testing
package testing

import (
	"context"
	"errors"

	"github.com/gruntwork-io/terragrunt/internal/azure/factory"
	"github.com/gruntwork-io/terragrunt/internal/azure/interfaces"
	"github.com/gruntwork-io/terragrunt/internal/azure/types"
	azurerm "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// TestingServicesConfig provides configuration for mock services in testing
type TestingServicesConfig struct {
	ExistsError     error
	UploadError     error
	DownloadError   error
	ExistsReturns   bool
	ContainerExists bool
}

// NewTestingServices creates new mock service instances for testing
func NewTestingServices(config *TestingServicesConfig) (interfaces.StorageAccountService, interfaces.BlobService) {
	return NewMockServices(config)
}

// NewMockServices creates mock services with the provided configuration
func NewMockServices(config *TestingServicesConfig) (interfaces.StorageAccountService, interfaces.BlobService) {
	if config == nil {
		config = &TestingServicesConfig{}
	}

	storageService := &MockStorageAccountService{
		CreateStorageAccountFunc: func(ctx context.Context, cfg *types.StorageAccountConfig) error {
			return config.ExistsError // Use the same error pattern for creation
		},
	}

	blobService := &MockBlobService{
		ContainerExistsFunc: func(ctx context.Context, containerName string) (bool, error) {
			return config.ContainerExists, nil
		},
		UploadBlobFunc: func(ctx context.Context, l log.Logger, containerName, blobName string, data []byte) error {
			return config.UploadError
		},
		GetObjectFunc: func(ctx context.Context, input *types.GetObjectInput) (*types.GetObjectOutput, error) {
			if config.DownloadError != nil {
				return nil, config.DownloadError
			}

			return &types.GetObjectOutput{
				Content: []byte("mock-state-data"),
			}, nil
		},
	}

	return storageService, blobService
}

// NewTestBackendConfig creates a backend config for testing with mock services
func NewTestBackendConfig() *azurerm.BackendConfig {
	// Create a mock factory that provides the mock services
	f := factory.NewAzureServiceFactory()
	// Wrap it to implement ServiceFactory
	mockFactory := &mockServiceFactory{factory: f}

	return &azurerm.BackendConfig{
		ServiceFactory: mockFactory,
	}
}

// mockServiceFactory wraps a factory.EnhancedServiceFactory to implement interfaces.ServiceFactory
type mockServiceFactory struct {
	factory interfaces.AzureServiceContainer
}

func (m *mockServiceFactory) CreateContainer(ctx context.Context) interfaces.AzureServiceContainer {
	return m.factory
}

func (m *mockServiceFactory) Options() *interfaces.FactoryOptions {
	return &interfaces.FactoryOptions{
		EnableMocking: true,
	}
}

// MockStorageAccountService implements interfaces.StorageAccountService for testing
type MockStorageAccountService struct {
	// Function hooks for customizing behavior (pointers first for alignment)
	CreateStorageAccountFunc        func(ctx context.Context, cfg *types.StorageAccountConfig) error
	DeleteStorageAccountFunc        func(ctx context.Context, l log.Logger) error
	ExistsFunc                      func(ctx context.Context) (bool, error)
	GetStorageAccountFunc           func(ctx context.Context) (*types.StorageAccount, error)
	GetStorageAccountKeysFunc       func(ctx context.Context) ([]string, error)
	GetStorageAccountSASFunc        func(ctx context.Context) (string, error)
	GetStorageAccountPropertiesFunc func(ctx context.Context) (*types.StorageAccountProperties, error)
	IsVersioningEnabledFunc         func(ctx context.Context) (bool, error)

	// Configuration for the mock - which account this service "operates on"
	ResourceGroupName  string
	StorageAccountName string
}

// GetResourceGroupName returns the resource group name this mock service operates on
func (m *MockStorageAccountService) GetResourceGroupName() string {
	return m.ResourceGroupName
}

// GetStorageAccountName returns the storage account name this mock service operates on
func (m *MockStorageAccountService) GetStorageAccountName() string {
	return m.StorageAccountName
}

func (m *MockStorageAccountService) CreateStorageAccount(ctx context.Context, cfg *types.StorageAccountConfig) error {
	if m.CreateStorageAccountFunc != nil {
		return m.CreateStorageAccountFunc(ctx, cfg)
	}

	return nil
}

func (m *MockStorageAccountService) DeleteStorageAccount(ctx context.Context, l log.Logger) error {
	if m.DeleteStorageAccountFunc != nil {
		return m.DeleteStorageAccountFunc(ctx, l)
	}

	return nil
}

func (m *MockStorageAccountService) Exists(ctx context.Context) (bool, error) {
	if m.ExistsFunc != nil {
		return m.ExistsFunc(ctx)
	}

	return true, nil
}

func (m *MockStorageAccountService) GetStorageAccount(ctx context.Context) (*types.StorageAccount, error) {
	if m.GetStorageAccountFunc != nil {
		return m.GetStorageAccountFunc(ctx)
	}

	return nil, nil
}

func (m *MockStorageAccountService) GetStorageAccountKeys(ctx context.Context) ([]string, error) {
	if m.GetStorageAccountKeysFunc != nil {
		return m.GetStorageAccountKeysFunc(ctx)
	}

	return []string{}, nil
}

func (m *MockStorageAccountService) GetStorageAccountSAS(ctx context.Context) (string, error) {
	if m.GetStorageAccountSASFunc != nil {
		return m.GetStorageAccountSASFunc(ctx)
	}

	return "", nil
}

func (m *MockStorageAccountService) GetStorageAccountProperties(ctx context.Context) (*types.StorageAccountProperties, error) {
	if m.GetStorageAccountPropertiesFunc != nil {
		return m.GetStorageAccountPropertiesFunc(ctx)
	}

	return nil, nil
}

func (m *MockStorageAccountService) IsVersioningEnabled(ctx context.Context) (bool, error) {
	if m.IsVersioningEnabledFunc != nil {
		return m.IsVersioningEnabledFunc(ctx)
	}
	// For testing, default to versioning enabled unless explicitly configured
	return true, nil
}

// MockBlobService implements interfaces.BlobService for testing
type MockBlobService struct {
	GetObjectFunc                  func(ctx context.Context, input *types.GetObjectInput) (*types.GetObjectOutput, error)
	ContainerExistsFunc            func(ctx context.Context, containerName string) (bool, error)
	CreateContainerIfNecessaryFunc func(ctx context.Context, l log.Logger, containerName string) error
	DeleteContainerFunc            func(ctx context.Context, l log.Logger, containerName string) error
	DeleteBlobIfNecessaryFunc      func(ctx context.Context, l log.Logger, containerName string, blobName string) error
	UploadBlobFunc                 func(ctx context.Context, l log.Logger, containerName, blobName string, data []byte) error
	CopyBlobToContainerFunc        func(ctx context.Context, srcContainer, srcKey string, dstClient interfaces.BlobService, dstContainer, dstKey string) error
}

func (m *MockBlobService) GetObject(ctx context.Context, input *types.GetObjectInput) (*types.GetObjectOutput, error) {
	if m.GetObjectFunc != nil {
		return m.GetObjectFunc(ctx, input)
	}

	return nil, errors.New("not implemented")
}

func (m *MockBlobService) ContainerExists(ctx context.Context, containerName string) (bool, error) {
	if m.ContainerExistsFunc != nil {
		return m.ContainerExistsFunc(ctx, containerName)
	}

	return false, nil
}

func (m *MockBlobService) CreateContainerIfNecessary(ctx context.Context, l log.Logger, containerName string) error {
	if m.CreateContainerIfNecessaryFunc != nil {
		return m.CreateContainerIfNecessaryFunc(ctx, l, containerName)
	}

	return nil
}

func (m *MockBlobService) DeleteContainer(ctx context.Context, l log.Logger, containerName string) error {
	if m.DeleteContainerFunc != nil {
		return m.DeleteContainerFunc(ctx, l, containerName)
	}

	return nil
}

func (m *MockBlobService) DeleteBlobIfNecessary(ctx context.Context, l log.Logger, containerName string, blobName string) error {
	if m.DeleteBlobIfNecessaryFunc != nil {
		return m.DeleteBlobIfNecessaryFunc(ctx, l, containerName, blobName)
	}

	return nil
}

func (m *MockBlobService) UploadBlob(ctx context.Context, l log.Logger, containerName, blobName string, data []byte) error {
	if m.UploadBlobFunc != nil {
		return m.UploadBlobFunc(ctx, l, containerName, blobName, data)
	}

	return nil
}

func (m *MockBlobService) CopyBlobToContainer(ctx context.Context, srcContainer, srcKey string, dstClient interfaces.BlobService, dstContainer, dstKey string) error {
	if m.CopyBlobToContainerFunc != nil {
		return m.CopyBlobToContainerFunc(ctx, srcContainer, srcKey, dstClient, dstContainer, dstKey)
	}

	return nil
}
