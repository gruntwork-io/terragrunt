package azuretest

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/require"
)

const (
	// AzureStorageContainerMaxLength is the maximum length for a container name in Azure
	AzureStorageContainerMaxLength = 63

	// AzureStorageAccountMaxLength is the maximum length for a storage account name in Azure
	AzureStorageAccountMaxLength = 24

	// AzureResourceGroupMaxLength is the maximum length for a resource group name in Azure
	AzureResourceGroupMaxLength = 90

	// AzureTagValueMaxLength is the maximum length for a tag value in Azure
	AzureTagValueMaxLength = 256

	testIDSlugMaxLength          = 12
	containerCleanNameMaxLength  = 16
	containerPrefixTrimLength    = 6
	resourceIDTrimLength         = 8
	containerCleanupSleepSeconds = 2
	resourceDeletionSleepSeconds = 5
	defaultConfigFilePermissions = 0o644
	resourceGroupNamePrefix      = "terragrunt-test-"
	containerNamePrefix          = "tg-"
	containerNameBase            = "tg"
)

// IsolatedAzureConfig contains required configuration for running isolated Azure tests
//
//nolint:govet // fieldalignment: Test configuration matches documentation order for clarity.
type IsolatedAzureConfig struct {
	// Basic Azure Configuration
	StorageAccountName string
	ContainerName      string
	Location           string
	ResourceGroup      string
	SubscriptionID     string

	// Authentication
	AccessKey string

	// Isolation Properties
	TestName      string
	TestID        string
	IsolationMode string

	// Advanced Isolation
	TestTags map[string]string

	UseAzureAD             bool
	IsolatedStorageAccount bool
	IsolatedResourceGroup  bool
	CleanupAfterTest       bool
}

// isolatedAzureEnvConfig holds environment-based configuration for Azure tests
type isolatedAzureEnvConfig struct {
	storageAccount        string
	accessKey             string
	resourceGroup         string
	subscriptionID        string
	location              string
	isolationMode         string
	isolateStorageAccount bool
	isolateResourceGroup  bool
	cleanupAfter          bool
}

// readIsolatedAzureEnvConfig reads Azure test configuration from environment variables
func readIsolatedAzureEnvConfig() *isolatedAzureEnvConfig {
	return &isolatedAzureEnvConfig{
		storageAccount:        os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT"),
		accessKey:             os.Getenv("TERRAGRUNT_AZURE_TEST_ACCESS_KEY"),
		resourceGroup:         os.Getenv("TERRAGRUNT_AZURE_TEST_RESOURCE_GROUP"),
		subscriptionID:        os.Getenv("TERRAGRUNT_AZURE_TEST_SUBSCRIPTION_ID"),
		location:              os.Getenv("TERRAGRUNT_AZURE_TEST_LOCATION"),
		isolationMode:         os.Getenv("TERRAGRUNT_AZURE_TEST_ISOLATION"),
		isolateStorageAccount: os.Getenv("TERRAGRUNT_AZURE_TEST_ISOLATE_STORAGE") == "true",
		isolateResourceGroup:  os.Getenv("TERRAGRUNT_AZURE_TEST_ISOLATE_RESOURCE_GROUP") == "true",
		cleanupAfter:          os.Getenv("TERRAGRUNT_AZURE_TEST_CLEANUP") != "false", // Default to true
	}
}

// GetIsolatedAzureConfig returns an Azure configuration with proper test isolation
func GetIsolatedAzureConfig(t *testing.T) *IsolatedAzureConfig {
	t.Helper()

	// Get test credentials from environment
	env := readIsolatedAzureEnvConfig()

	// Check requirements
	if env.storageAccount == "" && !env.isolateStorageAccount {
		t.Skip("Skipping Azure test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT not set and isolated storage not enabled")
	}

	// Use ARM_ variables as fallbacks
	if env.subscriptionID == "" {
		env.subscriptionID = os.Getenv("ARM_SUBSCRIPTION_ID")
		if env.subscriptionID == "" {
			t.Skip("Skipping Azure test: No subscription ID provided")
		}
	}

	if env.resourceGroup == "" && !env.isolateResourceGroup {
		env.resourceGroup = "terragrunt-test"
	}

	if env.location == "" {
		env.location = "eastus"
	}

	if env.isolationMode == "" {
		env.isolationMode = "full" // Options: "full", "container", "none"
	}

	// Generate unique test ID
	testID := generateUniqueTestID(t.Name())

	// Create isolated resource names
	containerName := generateIsolatedContainerName(t.Name(), testID)

	// Create isolated storage account and resource group if needed
	finalStorageAccount := env.storageAccount
	finalResourceGroup := env.resourceGroup

	if env.isolateStorageAccount {
		finalStorageAccount = generateIsolatedStorageAccountName(t.Name(), testID)
		t.Logf("[%s] Using isolated storage account: %s", t.Name(), finalStorageAccount)
	}

	if env.isolateResourceGroup {
		finalResourceGroup = generateIsolatedResourceGroupName(t.Name(), testID)
		t.Logf("[%s] Using isolated resource group: %s", t.Name(), finalResourceGroup)
	}

	t.Logf("[%s] Created isolated Azure config with container %s (isolation: %s)",
		t.Name(), containerName, env.isolationMode)

	// Generate test tags for resource tracking
	testTags := map[string]string{
		"terragrunt-test":      "true",
		"terragrunt-test-id":   testID,
		"terragrunt-test-name": sanitizeTagValue(t.Name()),
		"terragrunt-timestamp": time.Now().Format("2006-01-02-15-04-05"),
	}

	return &IsolatedAzureConfig{
		StorageAccountName:     finalStorageAccount,
		ContainerName:          containerName,
		Location:               env.location,
		ResourceGroup:          finalResourceGroup,
		SubscriptionID:         env.subscriptionID,
		UseAzureAD:             env.accessKey == "",
		AccessKey:              env.accessKey,
		TestName:               t.Name(),
		TestID:                 testID,
		IsolationMode:          env.isolationMode,
		IsolatedStorageAccount: env.isolateStorageAccount,
		IsolatedResourceGroup:  env.isolateResourceGroup,
		CleanupAfterTest:       env.cleanupAfter,
		TestTags:               testTags,
	}
}

// generateUniqueTestID creates a unique identifier for the test run
func generateUniqueTestID(testName string) string {
	// Format: <clean-test-name>-<timestamp>-<uuid-part>
	timestamp := time.Now().Format("20060102-150405")
	uuidPart := strings.Split(uuid.New().String(), "-")[0]

	// Derive a stable, Azure-friendly slug from the test name to aid debugging
	cleanBuilder := strings.Builder{}
	lastWasHyphen := false

	for _, r := range strings.ToLower(testName) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			cleanBuilder.WriteRune(r)

			lastWasHyphen = false
		case r == '-' || r == '_' || r == ' ':
			if cleanBuilder.Len() > 0 && !lastWasHyphen {
				cleanBuilder.WriteRune('-')

				lastWasHyphen = true
			}
		default:
			if cleanBuilder.Len() > 0 && !lastWasHyphen {
				cleanBuilder.WriteRune('-')

				lastWasHyphen = true
			}
		}
	}

	cleanName := strings.Trim(cleanBuilder.String(), "-")
	if len(cleanName) > testIDSlugMaxLength {
		cleanName = cleanName[:testIDSlugMaxLength]
	}

	if cleanName != "" {
		return fmt.Sprintf("%s-%s-%s", cleanName, timestamp, uuidPart)
	}

	return fmt.Sprintf("%s-%s", timestamp, uuidPart)
}

// generateIsolatedContainerName creates a container name that ensures test isolation
func generateIsolatedContainerName(testName, testID string) string {
	// Clean the test name for Azure compliance
	cleanName := strings.ToLower(testName)
	cleanName = strings.ReplaceAll(cleanName, "/", "-")
	cleanName = strings.ReplaceAll(cleanName, "_", "-")
	cleanName = strings.ReplaceAll(cleanName, " ", "-")
	cleanName = strings.ReplaceAll(cleanName, "test", "")

	// Replace sequential dashes with single dash
	for strings.Contains(cleanName, "--") {
		cleanName = strings.ReplaceAll(cleanName, "--", "-")
	}

	// Trim dashes from beginning and end
	cleanName = strings.Trim(cleanName, "-")

	// Limit length
	if len(cleanName) > containerCleanNameMaxLength {
		cleanName = cleanName[:containerCleanNameMaxLength]
	}

	// Format: tg-<clean-name>-<test-id>
	containerName := containerNamePrefix + cleanName + "-" + testID

	// Azure container names must be 3-63 characters
	if len(containerName) > AzureStorageContainerMaxLength {
		// Use first characters of name + test ID to maintain some readability
		prefix := containerNameBase
		if len(cleanName) > containerPrefixTrimLength {
			prefix = containerNamePrefix + cleanName[:containerPrefixTrimLength]
		}

		containerName = prefix + "-" + testID

		// If still too long, use just the prefix and shortened test ID
		if len(containerName) > AzureStorageContainerMaxLength {
			containerName = prefix + "-" + testID[len(testID)-20:]
		}
	}

	return containerName
}

// GetAzureBlobClient creates a blob service client for testing
func GetAzureBlobClient(t *testing.T, config *IsolatedAzureConfig) *azurehelper.BlobServiceClient {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	ctx := context.Background()

	// Create configuration based on authentication method
	azureConfig := map[string]interface{}{
		"storage_account_name": config.StorageAccountName,
		"resource_group_name":  config.ResourceGroup,
		"subscription_id":      config.SubscriptionID,
	}

	if config.UseAzureAD {
		azureConfig["use_azuread_auth"] = true
	} else {
		azureConfig["access_key"] = config.AccessKey
	}

	// Create Azure blob service client
	blobClient, err := azurehelper.CreateBlobServiceClient(ctx, nil, opts, azureConfig)
	require.NoError(t, err, "Failed to initialize Azure blob service client")

	return blobClient
}

// EnsureContainerExists creates a container if it doesn't exist
func EnsureContainerExists(t *testing.T, config *IsolatedAzureConfig, blobClient *azurehelper.BlobServiceClient) {
	t.Helper()

	ctx := context.Background()

	// Check if container exists
	exists, err := blobClient.ContainerExists(ctx, config.ContainerName)
	require.NoError(t, err, "Failed to check if container exists")

	if !exists {
		err = blobClient.CreateContainerIfNecessary(ctx, nil, config.ContainerName)
		require.NoError(t, err, "Failed to create container")
		t.Logf("[%s] Created container: %s", t.Name(), config.ContainerName)
	} else {
		t.Logf("[%s] Container already exists: %s", t.Name(), config.ContainerName)
	}
}

// CleanupAzureResources ensures all Azure resources created for the test are deleted
func CleanupAzureResources(t *testing.T, config *IsolatedAzureConfig) {
	t.Helper()

	if config == nil || !config.CleanupAfterTest {
		return
	}

	// First, clean up container
	CleanupContainer(t, config)

	// If using isolated storage account, clean it up
	if config.IsolatedStorageAccount {
		CleanupStorageAccount(t, config)
	}

	// If using isolated resource group, clean it up (which will clean up all resources within it)
	if config.IsolatedResourceGroup {
		CleanupResourceGroup(t, config)
	}
}

// CleanupContainer ensures the Azure container is deleted after the test
func CleanupContainer(t *testing.T, config *IsolatedAzureConfig) {
	t.Helper()

	if config == nil {
		return
	}

	blobClient := GetAzureBlobClient(t, config)
	ctx := context.Background()

	// Check if container exists
	exists, err := blobClient.ContainerExists(ctx, config.ContainerName)
	if err != nil {
		t.Logf("[%s] WARNING: Failed to check if container exists for cleanup: %v", t.Name(), err)
		return
	}

	if exists {
		// Try to delete the container with retries
		const maxRetries = 3

		var deleteErr error

		for i := 0; i < maxRetries; i++ {
			deleteErr = blobClient.DeleteContainer(ctx, nil, config.ContainerName)
			if deleteErr == nil {
				t.Logf("[%s] Successfully deleted Azure container: %s", t.Name(), config.ContainerName)
				return
			}

			t.Logf("[%s] Failed to delete container (attempt %d/%d): %v",
				t.Name(), i+1, maxRetries, deleteErr)

			time.Sleep(containerCleanupSleepSeconds * time.Second)
		}

		if deleteErr != nil {
			t.Logf("[%s] WARNING: Could not delete Azure container %s after %d attempts: %v",
				t.Name(), config.ContainerName, maxRetries, deleteErr)
		}
	} else {
		t.Logf("[%s] Container %s does not exist, skipping cleanup", t.Name(), config.ContainerName)
	}
}

// CleanupStorageAccount deletes a storage account if it was created specifically for this test
func CleanupStorageAccount(t *testing.T, config *IsolatedAzureConfig) {
	t.Helper()

	if !config.IsolatedStorageAccount {
		return
	}

	ctx := context.Background()

	// Create storage account client
	storageClient, err := azurehelper.CreateStorageAccountClient(ctx, nil, map[string]interface{}{
		"storage_account_name": config.StorageAccountName,
		"resource_group_name":  config.ResourceGroup,
		"subscription_id":      config.SubscriptionID,
		"use_azuread_auth":     config.UseAzureAD,
	})
	require.NoError(t, err, "Failed to create Azure storage account client")

	// Check if storage account exists
	exists, _, err := storageClient.StorageAccountExists(ctx)
	if err != nil {
		t.Logf("[%s] WARNING: Failed to check if storage account exists for cleanup: %v", t.Name(), err)
		return
	}

	if exists {
		// Try to delete the storage account with retries
		const maxRetries = 3

		var deleteErr error

		for i := 0; i < maxRetries; i++ {
			deleteErr = storageClient.DeleteStorageAccount(ctx, nil)
			if deleteErr == nil {
				t.Logf("[%s] Successfully deleted Azure storage account: %s", t.Name(), config.StorageAccountName)
				return
			}

			t.Logf("[%s] Failed to delete storage account (attempt %d/%d): %v",
				t.Name(), i+1, maxRetries, deleteErr)

			time.Sleep(resourceDeletionSleepSeconds * time.Second) // Storage accounts can take longer to delete
		}

		if deleteErr != nil {
			t.Logf("[%s] WARNING: Could not delete Azure storage account %s after %d attempts: %v",
				t.Name(), config.StorageAccountName, maxRetries, deleteErr)
		}
	} else {
		t.Logf("[%s] Storage account %s does not exist, skipping cleanup", t.Name(), config.StorageAccountName)
	}
}

// CleanupResourceGroup deletes a resource group if it was created specifically for this test
func CleanupResourceGroup(t *testing.T, config *IsolatedAzureConfig) {
	t.Helper()

	if !config.IsolatedResourceGroup {
		return
	}

	ctx := context.Background()

	// Create resource group client
	resourceClient, err := azurehelper.CreateResourceGroupClient(ctx, nil, config.SubscriptionID)
	require.NoError(t, err, "Failed to create Azure resource group client")

	// Check if resource group exists
	exists, err := resourceClient.ResourceGroupExists(ctx, config.ResourceGroup)
	if err != nil {
		t.Logf("[%s] WARNING: Failed to check if resource group exists for cleanup: %v", t.Name(), err)
		return
	}

	if exists {
		// Try to delete the resource group with retries
		const maxRetries = 3

		var deleteErr error

		for i := 0; i < maxRetries; i++ {
			deleteErr = resourceClient.DeleteResourceGroup(ctx, nil, config.ResourceGroup)
			if deleteErr == nil {
				t.Logf("[%s] Successfully initiated deletion of Azure resource group: %s", t.Name(), config.ResourceGroup)
				return
			}

			t.Logf("[%s] Failed to delete resource group (attempt %d/%d): %v",
				t.Name(), i+1, maxRetries, deleteErr)

			time.Sleep(resourceDeletionSleepSeconds * time.Second) // Resource groups can take longer to delete
		}

		if deleteErr != nil {
			t.Logf("[%s] WARNING: Could not delete Azure resource group %s after %d attempts: %v",
				t.Name(), config.ResourceGroup, maxRetries, deleteErr)
		}
	} else {
		t.Logf("[%s] Resource group %s does not exist, skipping cleanup", t.Name(), config.ResourceGroup)
	}
}

// UpdateTerragruntConfigForAzureTest modifies the terragrunt config file to use isolated resources
func UpdateTerragruntConfigForAzureTest(t *testing.T, config *IsolatedAzureConfig, configPath string) {
	t.Helper()

	// Read the file
	content, err := os.ReadFile(configPath)
	require.NoError(t, err, "Failed to read terragrunt config")

	// Update with isolated resources
	updatedContent := string(content)

	// Update container name
	updatedContent = strings.ReplaceAll(
		updatedContent,
		`container_name = "terragrunt-test"`,
		fmt.Sprintf(`container_name = "%s"`, config.ContainerName),
	)

	// Update storage account name if isolated
	if config.IsolatedStorageAccount {
		updatedContent = strings.ReplaceAll(
			updatedContent,
			`storage_account_name = "terragrunttest"`,
			fmt.Sprintf(`storage_account_name = "%s"`, config.StorageAccountName),
		)

		// Also handle variations
		updatedContent = strings.ReplaceAll(
			updatedContent,
			`storage_account_name = "terragrunt-test"`,
			fmt.Sprintf(`storage_account_name = "%s"`, config.StorageAccountName),
		)
	}

	// Update resource group name if isolated
	if config.IsolatedResourceGroup {
		updatedContent = strings.ReplaceAll(
			updatedContent,
			`resource_group_name = "terragrunt-test"`,
			fmt.Sprintf(`resource_group_name = "%s"`, config.ResourceGroup),
		)
	}

	// Update key if provided
	if config.AccessKey != "" {
		// Remove any existing access key or Azure AD auth
		updatedContent = removeHclBlock(updatedContent, "access_key")
		updatedContent = removeHclBlock(updatedContent, "use_azuread_auth")

		// Add access key
		updatedContent = addHclAttribute(updatedContent, "access_key", config.AccessKey)
	} else {
		// Using Azure AD auth
		updatedContent = removeHclBlock(updatedContent, "access_key")
		updatedContent = addHclAttribute(updatedContent, "use_azuread_auth", "true")
	}

	// Write back to file
	err = os.WriteFile(configPath, []byte(updatedContent), defaultConfigFilePermissions)
	require.NoError(t, err, "Failed to write updated terragrunt config")

	t.Logf("[%s] Updated terragrunt config with isolated resources", t.Name())
}

// sanitizeTagValue ensures the tag value meets Azure requirements
func sanitizeTagValue(value string) string {
	if len(value) > AzureTagValueMaxLength {
		return value[:AzureTagValueMaxLength]
	}

	return value
}

// generateIsolatedStorageAccountName creates a storage account name that ensures test isolation
// Storage accounts must be 3-24 characters, lowercase letters and numbers only
func generateIsolatedStorageAccountName(testName, testID string) string {
	// Clean the test name for Azure compliance
	cleanName := strings.ToLower(testName)
	cleanName = strings.ReplaceAll(cleanName, "/", "")
	cleanName = strings.ReplaceAll(cleanName, "_", "")
	cleanName = strings.ReplaceAll(cleanName, " ", "")
	cleanName = strings.ReplaceAll(cleanName, "-", "")
	cleanName = strings.ReplaceAll(cleanName, "test", "")

	// Extract unique portion of test ID (without dashes)
	idPart := strings.ReplaceAll(testID, "-", "")
	if len(idPart) > resourceIDTrimLength {
		idPart = idPart[:resourceIDTrimLength]
	}

	// Format: tg<clean-name><id-part>
	accountName := fmt.Sprintf("tg%s%s", cleanName, idPart)

	// Storage account names must be 3-24 characters
	if len(accountName) > AzureStorageAccountMaxLength {
		accountName = accountName[:AzureStorageAccountMaxLength]
	}

	return accountName
}

// generateIsolatedResourceGroupName creates a resource group name that ensures test isolation
func generateIsolatedResourceGroupName(testName, testID string) string {
	// Clean the test name for Azure compliance
	cleanName := strings.ToLower(testName)
	cleanName = strings.ReplaceAll(cleanName, "/", "-")
	cleanName = strings.ReplaceAll(cleanName, "_", "-")
	cleanName = strings.ReplaceAll(cleanName, " ", "-")

	// Replace sequential dashes with single dash
	for strings.Contains(cleanName, "--") {
		cleanName = strings.ReplaceAll(cleanName, "--", "-")
	}

	// Trim dashes from beginning and end
	cleanName = strings.Trim(cleanName, "-")

	// Format: terragrunt-test-<clean-name>-<test-id>
	rgName := resourceGroupNamePrefix + cleanName + "-" + testID

	// Resource group names must be 1-90 characters
	if len(rgName) > AzureResourceGroupMaxLength {
		// Keep as much of the test name as possible for readability
		maxTestNameLen := AzureResourceGroupMaxLength - len(testID) - len(resourceGroupNamePrefix) - 1 // extra hyphen between name and testID
		if maxTestNameLen > 0 {
			rgName = resourceGroupNamePrefix + cleanName[:maxTestNameLen] + "-" + testID
		} else {
			rgName = resourceGroupNamePrefix + testID
		}
	}

	return rgName
}

// EnsureResourceGroupExists creates a resource group if it doesn't exist
func EnsureResourceGroupExists(t *testing.T, config *IsolatedAzureConfig) {
	t.Helper()

	// Only create resource group if we're using isolation
	if !config.IsolatedResourceGroup {
		return
	}

	ctx := context.Background()

	// Create resource group client
	resourceClient, err := azurehelper.CreateResourceGroupClient(ctx, nil, config.SubscriptionID)
	require.NoError(t, err, "Failed to create Azure resource group client")

	// Check if resource group exists
	exists, err := resourceClient.ResourceGroupExists(ctx, config.ResourceGroup)
	require.NoError(t, err, "Failed to check if resource group exists")

	if !exists {
		// Create resource group with test tags
		err = resourceClient.EnsureResourceGroup(ctx, nil, config.ResourceGroup, config.Location, config.TestTags)
		require.NoError(t, err, "Failed to create resource group")
		t.Logf("[%s] Created resource group: %s", config.TestName, config.ResourceGroup)
	} else {
		t.Logf("[%s] Resource group already exists: %s", config.TestName, config.ResourceGroup)
	}
}

// EnsureStorageAccountExists creates a storage account if it doesn't exist
func EnsureStorageAccountExists(t *testing.T, config *IsolatedAzureConfig) {
	t.Helper()

	// Only create storage account if we're using isolation
	if !config.IsolatedStorageAccount {
		return
	}

	ctx := context.Background()

	// Create storage account client
	storageClient, err := azurehelper.CreateStorageAccountClient(ctx, nil, map[string]interface{}{
		"storage_account_name": config.StorageAccountName,
		"resource_group_name":  config.ResourceGroup,
		"subscription_id":      config.SubscriptionID,
		"use_azuread_auth":     config.UseAzureAD,
		"location":             config.Location,
	})
	require.NoError(t, err, "Failed to create Azure storage account client")

	// Check if storage account exists
	exists, _, err := storageClient.StorageAccountExists(ctx)
	require.NoError(t, err, "Failed to check if storage account exists")

	if !exists {
		// Create storage account config
		storageConfig := azurehelper.StorageAccountConfig{
			StorageAccountName: config.StorageAccountName,
			ResourceGroupName:  config.ResourceGroup,
			Location:           config.Location,
			Tags:               config.TestTags,
		}

		err = storageClient.CreateStorageAccountIfNecessary(ctx, nil, storageConfig)
		require.NoError(t, err, "Failed to create storage account")
		t.Logf("[%s] Created storage account: %s", config.TestName, config.StorageAccountName)
	} else {
		t.Logf("[%s] Storage account already exists: %s", config.TestName, config.StorageAccountName)
	}
}

// HCL manipulation helpers

// removeHclBlock removes a named block or attribute from HCL content
func removeHclBlock(content, blockName string) string {
	// Simple regex-based approach for test use only
	// In production, use proper HCL parsing
	blockPattern := fmt.Sprintf(`\s*%s\s*=\s*[^{].*\n`, blockName)
	result := content

	// First try single line attribute
	re := regexp.MustCompile(blockPattern)
	result = re.ReplaceAllString(result, "")

	// Try multi-line block
	blockPattern = fmt.Sprintf(`\s*%s\s*\{[^}]*\}`, blockName)
	re = regexp.MustCompile(blockPattern)
	result = re.ReplaceAllString(result, "")

	return result
}

// addHclAttribute adds an attribute to HCL content
func addHclAttribute(content, attrName, attrValue string) string {
	// Check if we need to add quotes
	valueStr := attrValue

	if _, err := strconv.ParseBool(attrValue); err != nil {
		if _, err := strconv.ParseFloat(attrValue, 64); err != nil {
			// Not a boolean or number, wrap in quotes
			valueStr = fmt.Sprintf(`"%s"`, attrValue)
		}
	}

	// Find the last closing brace of the remote_state block
	closeBraceIdx := strings.LastIndex(content, "}")
	if closeBraceIdx == -1 {
		// No closing brace, just append
		return fmt.Sprintf("%s\n  %s = %s", content, attrName, valueStr)
	}

	// Insert before the closing brace
	return fmt.Sprintf("%s  %s = %s\n%s",
		content[:closeBraceIdx],
		attrName,
		valueStr,
		content[closeBraceIdx:])
}
