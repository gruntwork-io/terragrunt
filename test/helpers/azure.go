// Package helpers provides test helper functions
package helpers

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/gruntwork-io/terragrunt/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// AzureStorageContainerMinLength is the minimum length for Azure storage container names (3 characters)
	AzureStorageContainerMinLength = 3
	// AzureStorageContainerMaxLength is the maximum length for Azure storage container names (63 characters)
	AzureStorageContainerMaxLength = 63
)

// AzureStorageTestConfig contains Azure storage test configuration
type AzureStorageTestConfig struct {
	StorageAccountName string
	ContainerName      string
	Location           string
}

// GetAzureStorageTestConfig gets Azure storage test config from env vars
func GetAzureStorageTestConfig(t *testing.T) *AzureStorageTestConfig {
	t.Helper()

	accountName := os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")

	// Generate Azure-compliant container name:
	// - 3-63 chars
	// - Only lowercase letters, numbers, hyphens
	// - Start/end with letter/number
	// - No consecutive hyphens
	uniqueID := strings.ToLower(UniqueID())
	containerName := "tg" + strings.ReplaceAll(uniqueID, "_", "-")

	if len(containerName) > AzureStorageContainerMaxLength {
		containerName = containerName[:AzureStorageContainerMaxLength]
	}

	if containerName == "" {
		t.Fatal("Generated container name is empty")
	}

	if len(containerName) < AzureStorageContainerMinLength {
		t.Fatal("Generated container name is too short - must be at least 3 characters")
	}

	location := os.Getenv("TERRAGRUNT_AZURE_TEST_LOCATION")

	if accountName == "" {
		t.Skip("Skipping Azure tests: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT not set")
	}

	if location == "" {
		location = "westeurope" // Default location if not specified
	}

	return &AzureStorageTestConfig{
		StorageAccountName: accountName,
		ContainerName:      containerName,
		Location:           location,
	}
}

// CleanupAzureContainer deletes an Azure storage container
func CleanupAzureContainer(t *testing.T, config *AzureStorageTestConfig) {
	t.Helper()

	if config == nil {
		return
	}

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	ctx := t.Context()
	logger := log.Default()

	// Create storage client using Azure AD authentication
	client, err := azurehelper.CreateBlobServiceClient(ctx, logger, opts, map[string]interface{}{
		"storage_account_name": config.StorageAccountName,
		"use_azuread_auth":     true,
	})
	require.NoError(t, err)

	// Delete container if exists
	exists, err := client.ContainerExists(ctx, config.ContainerName)
	require.NoError(t, err)

	if exists {
		err = client.DeleteContainer(ctx, logger, config.ContainerName)
		require.NoError(t, err, "Failed to delete container "+config.ContainerName)
	}
}

// AzureTestContext contains the Azure test environment setup
type AzureTestContext struct {
	SubscriptionID     string
	Location           string
	StorageAccountName string
	ResourceGroupName  string
	ContainerName      string
	Logger             log.Logger
	Ctx                context.Context
	Cleanup            func()
}

// SetupAzureTest sets up the Azure test environment with credentials, unique names, and cleanup
func SetupAzureTest(t *testing.T, testName string) *AzureTestContext {
	t.Helper()

	ctx := t.Context()
	logger := log.Default()

	// Get Azure credentials and subscription ID
	_, subscriptionID, err := azurehelper.GetAzureCredentials(ctx, logger)
	if err != nil {
		t.Skipf("Skipping %s: Failed to get Azure credentials: %v", testName, err)
	}

	if subscriptionID == "" {
		t.Skipf("Skipping %s: No subscription ID found in environment variables", testName)
	}

	// Get location with fallback logic
	location := os.Getenv("AZURE_LOCATION")
	if location == "" {
		location = os.Getenv("ARM_LOCATION")
		if location == "" {
			location = "westeurope"
		}
	}

	// Generate unique resource names
	uniqueID := strconv.FormatInt(time.Now().UnixNano(), 10)
	if len(uniqueID) > 10 {
		uniqueID = uniqueID[len(uniqueID)-10:]
	}
	storageAccountName := "tgtest" + strings.ToLower(uniqueID)
	resourceGroupName := fmt.Sprintf("terragrunt-test-rg-%s-%s", strings.ReplaceAll(testName, " ", "-"), uniqueID)
	containerName := "test-container-" + strings.ToLower(uniqueID)

	// Track what resources were created for cleanup
	var resourceGroupCreated, storageAccountCreated bool

	// Setup cleanup function with retries and test context timeout
	cleanup := func() {
		cleanupLogger := log.Default()
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		defer cancel()

		retry := func(op func() error, desc string) {
			const maxAttempts = 5
			const delay = 5 * time.Second
			var lastErr error
			for i := 0; i < maxAttempts; i++ {
				lastErr = op()
				if lastErr == nil {
					t.Logf("%s cleanup succeeded on attempt %d", desc, i+1)
					return
				}
				t.Logf("%s cleanup failed on attempt %d: %v", desc, i+1, lastErr)
				time.Sleep(delay)
			}
			t.Errorf("%s cleanup failed after %d attempts: %v", desc, maxAttempts, lastErr)
		}

		// Clean up storage account first if it was created
		if storageAccountCreated {
			t.Logf("Cleaning up storage account %s", storageAccountName)
			storageConfig := map[string]interface{}{
				"storage_account_name": storageAccountName,
				"resource_group_name":  resourceGroupName,
				"subscription_id":      subscriptionID,
				"use_azuread_auth":     true,
			}
			retry(func() error {
				storageClient, err := azurehelper.CreateStorageAccountClient(ctx, cleanupLogger, storageConfig)
				if err != nil {
					return err
				}
				return storageClient.DeleteStorageAccount(ctx, cleanupLogger)
			}, "Storage account")
		}

		// Clean up resource group if it was created
		if resourceGroupCreated {
			t.Logf("Cleaning up resource group %s", resourceGroupName)
			retry(func() error {
				rgClient, err := azurehelper.CreateResourceGroupClient(ctx, cleanupLogger, subscriptionID)
				if err != nil {
					return err
				}
				return rgClient.DeleteResourceGroup(ctx, cleanupLogger, resourceGroupName)
			}, "Resource group")
		}
	}

	// Create resource group
	rgClient, err := azurehelper.CreateResourceGroupClient(ctx, logger, subscriptionID)
	require.NoError(t, err)

	err = rgClient.EnsureResourceGroup(ctx, logger, resourceGroupName, location, map[string]string{
		"created-by": "terragrunt-integration-test",
		"test-case":  testName,
	})
	require.NoError(t, err)
	resourceGroupCreated = true
	t.Logf("Resource group %s created successfully", resourceGroupName)

	return &AzureTestContext{
		SubscriptionID:     subscriptionID,
		Location:           location,
		StorageAccountName: storageAccountName,
		ResourceGroupName:  resourceGroupName,
		ContainerName:      containerName,
		Logger:             logger,
		Ctx:                ctx,
		Cleanup: func() {
			cleanup()
		},
	}
}

// CreateTestStorageAccount creates a storage account with versioning enabled for testing
func CreateTestStorageAccount(ctx context.Context, t *testing.T, azureCtx *AzureTestContext, enableVersioning bool) {
	t.Helper()

	storageAccountConfig := map[string]interface{}{
		"storage_account_name": azureCtx.StorageAccountName,
		"resource_group_name":  azureCtx.ResourceGroupName,
		"subscription_id":      azureCtx.SubscriptionID,
		"location":             azureCtx.Location,
		"use_azuread_auth":     true,
	}

	t.Logf("Creating storage account %s in resource group %s", azureCtx.StorageAccountName, azureCtx.ResourceGroupName)

	// Create storage account client
	storageClient, err := azurehelper.CreateStorageAccountClient(ctx, azureCtx.Logger, storageAccountConfig)
	require.NoError(t, err)
	require.NotNil(t, storageClient)

	// Define storage account configuration
	saConfig := azurehelper.StorageAccountConfig{
		SubscriptionID:        azureCtx.SubscriptionID,
		ResourceGroupName:     azureCtx.ResourceGroupName,
		StorageAccountName:    azureCtx.StorageAccountName,
		Location:              azureCtx.Location,
		EnableVersioning:      enableVersioning,
		AllowBlobPublicAccess: false,
		AccountKind:           "StorageV2",
		AccountTier:           "Standard",
		AccessTier:            "Hot",
		ReplicationType:       "LRS",
		Tags:                  map[string]string{"created-by": "terragrunt-integration-test"},
	}

	// Create storage account
	err = storageClient.CreateStorageAccountIfNecessary(ctx, azureCtx.Logger, saConfig)
	require.NoError(t, err)

	// Verify storage account exists
	exists, account, err := storageClient.StorageAccountExists(ctx)
	require.NoError(t, err)
	require.True(t, exists, "Storage account should exist after creation")
	require.NotNil(t, account)

	t.Logf("Storage account %s created successfully", azureCtx.StorageAccountName)
}

// GenerateAzureTerragruntConfig generates a terragrunt.hcl config with Azure backend
func GenerateAzureTerragruntConfig(storageAccount, container, stateKey, resourceGroup, subscriptionID, terraformSource string) string {
	return fmt.Sprintf(`
remote_state {
  backend = "azurerm"
  generate = {
	path      = "backend.tf"
	if_exists = "overwrite"
  }
  config = {
	storage_account_name = "%s"
	container_name      = "%s"
	key                 = "%s"
	resource_group_name = "%s"
	subscription_id     = "%s"
	use_azuread_auth    = true
  }
}

terraform {
  source = "%s"
}
`, storageAccount, container, stateKey, resourceGroup, subscriptionID, terraformSource)
}

// RunTerragruntAzureCommand runs a terragrunt command with Azure backend experiment flag
func RunTerragruntAzureCommand(t *testing.T, command string) (output, stderr string, err error) {
	t.Helper()

	fullCommand := fmt.Sprintf("terragrunt --experiment azure-backend %s", command)
	return RunTerragruntCommandWithOutput(t, fullCommand)
}

// SetupTestContainer creates a container and tests blob permissions
func SetupTestContainer(ctx context.Context, t *testing.T, storageAccountName, containerName string) (*azurehelper.BlobServiceClient, func()) {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	logger := log.Default()

	blobConfig := map[string]interface{}{
		"storage_account_name": storageAccountName,
		"container_name":       containerName,
		"use_azuread_auth":     true,
	}

	blobClient, err := azurehelper.CreateBlobServiceClient(ctx, logger, opts, blobConfig)
	require.NoError(t, err)

	// Create container if it doesn't exist
	err = blobClient.CreateContainerIfNecessary(ctx, logger, containerName)
	require.NoError(t, err)
	t.Logf("Container %s created successfully", containerName)

	// Test that we can actually perform blob operations with current credentials
	testBlobName := "test-permissions.txt"
	testContent := []byte("Permission test")
	err = blobClient.UploadBlob(ctx, logger, containerName, testBlobName, testContent)
	require.NoError(t, err, "Should be able to upload test blob - check Azure permissions")

	// Clean up test blob
	err = blobClient.DeleteBlobIfNecessary(ctx, logger, containerName, testBlobName)
	require.NoError(t, err)
	t.Logf("Blob permissions test passed successfully")

	// Return cleanup function
	cleanup := func() {
		exists, err := blobClient.ContainerExists(ctx, containerName)
		if err == nil && exists {
			err = blobClient.DeleteContainer(ctx, logger, containerName)
			if err != nil {
				t.Logf("Warning: Failed to delete container %s: %v", containerName, err)
			} else {
				t.Logf("Successfully deleted container %s", containerName)
			}
		}
	}

	return blobClient, cleanup
}

// CreateStandardBlobConfig creates a standard blob service client configuration
func CreateStandardBlobConfig(storageAccountName, containerName string, extraConfig map[string]interface{}) map[string]interface{} {
	config := map[string]interface{}{
		"storage_account_name": storageAccountName,
		"container_name":       containerName,
		"use_azuread_auth":     true,
	}

	// Add any extra configuration
	for key, value := range extraConfig {
		config[key] = value
	}

	return config
}

// AssertAzureErrorType checks if an error is of a specific Azure error type and provides helpful assertions
func AssertAzureErrorType(t *testing.T, err error, expectedType string) bool {
	t.Helper()

	if err == nil {
		t.Fatalf("Expected %s error but got nil", expectedType)
		return false
	}

	switch expectedType {
	case "AuthenticationError":
		var authErr azurerm.AuthenticationError
		if assert.ErrorAs(t, err, &authErr, "Error should be AuthenticationError type") {
			assert.NotEmpty(t, authErr.AuthMethod, "Authentication error should specify auth method")
			assert.Error(t, authErr.Unwrap(), "Authentication error should have underlying error")
			return true
		}

	case "StorageAccountCreationError":
		var storageErr azurerm.StorageAccountCreationError
		if assert.ErrorAs(t, err, &storageErr, "Error should be StorageAccountCreationError type") {
			assert.NotEmpty(t, storageErr.StorageAccountName, "Storage error should contain account name")
			assert.Error(t, storageErr.Unwrap(), "Storage error should have underlying error")
			return true
		}

	case "ContainerValidationError":
		var containerErr azurerm.ContainerValidationError
		if assert.ErrorAs(t, err, &containerErr, "Error should be ContainerValidationError type") {
			assert.NotEmpty(t, containerErr.ValidationIssue, "Container validation error should have validation issue")
			return true
		}

	case "ContainerCreationError":
		var containerCreationErr azurehelper.ContainerCreationError
		if assert.ErrorAs(t, err, &containerCreationErr, "Error should be ContainerCreationError type") {
			assert.NotEmpty(t, containerCreationErr.ContainerName, "Container creation error should contain container name")
			assert.Error(t, containerCreationErr.Unwrap(), "Container creation error should have underlying error")
			return true
		}

	case "MissingSubscriptionIDError":
		var missingSubErr azurerm.MissingSubscriptionIDError
		if assert.ErrorAs(t, err, &missingSubErr, "Error should be MissingSubscriptionIDError type") {
			assert.Contains(t, err.Error(), "subscription_id is required", "Error message should mention subscription_id")
			return true
		}

	case "MissingLocationError":
		var missingLocErr azurerm.MissingLocationError
		if assert.ErrorAs(t, err, &missingLocErr, "Error should be MissingLocationError type") {
			assert.Contains(t, err.Error(), "location is required", "Error message should mention location")
			return true
		}

	case "MissingResourceGroupError":
		var missingRGErr azurerm.MissingResourceGroupError
		if assert.ErrorAs(t, err, &missingRGErr, "Error should be MissingResourceGroupError type") {
			assert.Contains(t, err.Error(), "resource_group_name is required", "Error message should mention resource_group_name")
			return true
		}

	default:
		t.Fatalf("Unknown Azure error type: %s", expectedType)
		return false
	}

	return false
}

// CreateTerraformModule creates a simple terraform module for testing
func CreateTerraformModule(t *testing.T, moduleDir string) {
	t.Helper()

	err := os.MkdirAll(moduleDir, 0o755)
	require.NoError(t, err)

	terraformContent := `
resource "random_id" "test" {
  byte_length = 8
}

resource "local_file" "test" {
  content  = "Migration test - ${random_id.test.hex}"
  filename = "/tmp/terragrunt-migration-test-${random_id.test.hex}.txt"
}

output "random_id" {
  value = random_id.test.hex
}

output "file_path" {
  value = local_file.test.filename
}

output "content" {
  value = local_file.test.content
}
`
	err = os.WriteFile(fmt.Sprintf("%s/main.tf", moduleDir), []byte(terraformContent), 0o644)
	require.NoError(t, err)
}

// TestBlobOperations performs standard blob operations tests
func TestBlobOperations(t *testing.T, client *azurehelper.BlobServiceClient, containerName string) {
	t.Helper()

	ctx := t.Context()
	blobName := "test-blob.txt"
	logger := log.Default()

	// Test blob operations
	input := &azurehelper.GetObjectInput{
		Bucket: &containerName,
		Key:    &blobName,
	}

	// Test get non-existent blob
	_, err := client.GetObject(ctx, input)
	require.Error(t, err, "Getting non-existent blob should fail")

	// Test delete non-existent blob (should not error)
	err = client.DeleteBlobIfNecessary(ctx, logger, containerName, blobName)
	require.NoError(t, err, "Deleting non-existent blob should not error")

	// Verify container exists
	exists, err := client.ContainerExists(ctx, containerName)
	require.NoError(t, err)
	assert.True(t, exists, "Container should exist")
}

// CreateBlobServiceClientHelper creates a blob service client using helper pattern
func CreateBlobServiceClientHelper(ctx context.Context, t *testing.T, config map[string]interface{}) *azurehelper.BlobServiceClient {
	t.Helper()

	logger := log.New()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Failed to create terragrunt options")

	client, err := azurehelper.CreateBlobServiceClient(ctx, logger, opts, config)
	require.NoError(t, err, "Failed to create blob service client")

	return client
}

// GetBlobObjectHelper retrieves an object from Azure blob storage
func GetBlobObjectHelper(ctx context.Context, t *testing.T, client *azurehelper.BlobServiceClient, containerName, blobName string) ([]byte, error) {
	t.Helper()

	result, err := client.GetObject(ctx, &azurehelper.GetObjectInput{
		Bucket: &containerName,
		Key:    &blobName,
	})
	if err != nil {
		return nil, err
	}

	// Read the body content
	defer result.Body.Close()
	content, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read blob content: %w", err)
	}

	return content, nil
}

// CheckBlobExistsHelper checks if a blob exists in Azure storage
func CheckBlobExistsHelper(ctx context.Context, t *testing.T, client *azurehelper.BlobServiceClient, containerName, blobName string) bool {
	t.Helper()

	_, err := client.GetObject(ctx, &azurehelper.GetObjectInput{
		Bucket: &containerName,
		Key:    &blobName,
	})

	return err == nil
}

// GetAzureCredentialsHelper retrieves Azure credentials using helper pattern
func GetAzureCredentialsHelper(ctx context.Context, t *testing.T) (*azidentity.DefaultAzureCredential, string) {
	t.Helper()

	logger := log.New()
	creds, subscriptionID, err := azurehelper.GetAzureCredentials(ctx, logger)
	require.NoError(t, err, "Failed to get Azure credentials")

	return creds, subscriptionID
}

// CreateResourceGroupClientHelper creates a resource group client using helper pattern
func CreateResourceGroupClientHelper(ctx context.Context, t *testing.T, subscriptionID string) *azurehelper.ResourceGroupClient {
	t.Helper()

	logger := log.New()
	client, err := azurehelper.CreateResourceGroupClient(ctx, logger, subscriptionID)
	require.NoError(t, err, "Failed to create resource group client")

	return client
}

// CreateStorageAccountClientHelper creates a storage account client using helper pattern
func CreateStorageAccountClientHelper(ctx context.Context, t *testing.T, config map[string]interface{}) *azurehelper.StorageAccountClient {
	t.Helper()

	logger := log.New()
	client, err := azurehelper.CreateStorageAccountClient(ctx, logger, config)
	require.NoError(t, err, "Failed to create storage account client")

	return client
}

// CreateInvalidBlobServiceClientHelper creates a blob service client with invalid config for testing error scenarios
func CreateInvalidBlobServiceClientHelper(ctx context.Context, t *testing.T, config map[string]interface{}) (*azurehelper.BlobServiceClient, error) {
	t.Helper()

	logger := log.New()
	opts, err := options.NewTerragruntOptionsForTest("")
	if err != nil {
		return nil, err
	}

	return azurehelper.CreateBlobServiceClient(ctx, logger, opts, config)
}
