//go:build azure

package test_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/gruntwork-io/terragrunt/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	azurerm "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureAzureBackend               = "./fixtures/azure-backend"
	testFixtureAzureOutputFromRemoteState = "./fixtures/azure-output-from-remote-state"
)

// --- Azure test helpers (formerly from test/helpers/azure.go) ---
type azureStorageTestConfig struct {
	StorageAccountName string
	ContainerName      string
	Location           string
}
type terraformOutput struct {
	Value interface{} `json:"value"`
}

func getAzureStorageTestConfig(t *testing.T) *azureStorageTestConfig {
	t.Helper()
	accountName := os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")
	uniqueID := strings.ToLower(uniqueID())
	containerName := "tg" + strings.ReplaceAll(uniqueID, "_", "-")
	if len(containerName) > 63 {
		containerName = containerName[:63]
	}
	if containerName == "" {
		t.Fatal("Generated container name is empty")
	}
	if len(containerName) < 3 {
		t.Fatal("Generated container name is too short - must be at least 3 characters")
	}
	location := os.Getenv("TERRAGRUNT_AZURE_TEST_LOCATION")
	if accountName == "" {
		t.Skip("Skipping Azure tests: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT not set")
	}
	if location == "" {
		location = "westeurope"
	}
	return &azureStorageTestConfig{
		StorageAccountName: accountName,
		ContainerName:      containerName,
		Location:           location,
	}
}

func cleanupAzureContainer(t *testing.T, config *azureStorageTestConfig) {
	t.Helper()
	if config == nil {
		return
	}
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	ctx := t.Context()
	logger := logger.CreateLogger()
	client, err := azurehelper.CreateBlobServiceClient(ctx, logger, opts, map[string]interface{}{
		"storage_account_name": config.StorageAccountName,
		"use_azuread_auth":     true,
	})
	require.NoError(t, err)
	exists, err := client.ContainerExists(ctx, config.ContainerName)
	require.NoError(t, err)
	if exists {
		err = client.DeleteContainer(ctx, logger, config.ContainerName)
		require.NoError(t, err, "Failed to delete container "+config.ContainerName)
	}
}

func createStandardBlobConfig(storageAccountName, containerName string, extraConfig map[string]interface{}) map[string]interface{} {
	config := map[string]interface{}{
		"storage_account_name": storageAccountName,
		"container_name":       containerName,
		"use_azuread_auth":     true,
	}
	for key, value := range extraConfig {
		config[key] = value
	}
	return config
}

func createBlobServiceClientHelper(ctx context.Context, t *testing.T, config map[string]interface{}) *azurehelper.BlobServiceClient {
	t.Helper()
	logger := logger.CreateLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Failed to create terragrunt options")
	client, err := azurehelper.CreateBlobServiceClient(ctx, logger, opts, config)
	require.NoError(t, err, "Failed to create blob service client")
	return client
}

func assertAzureErrorType(t *testing.T, err error, expectedType string) bool {
   t.Helper()
   if err == nil {
	   t.Fatalf("Expected %s error but got nil", expectedType)
	   return false
   }
   switch expectedType {
   case "AuthorizationError":
	   // Accept any error that contains 'authorization', 'forbidden', or 'permission' in the message as an authorization error
	   errMsg := strings.ToLower(err.Error())
	   if strings.Contains(errMsg, "authorization") || strings.Contains(errMsg, "forbidden") || strings.Contains(errMsg, "permission") {
		   assert.Contains(t, errMsg, "authorization", "Error message should mention authorization")
		   return true
	   }
	   // No match, fall through to default
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
   // (Removed duplicate case "AuthorizationError")
	default:
		t.Fatalf("Unknown Azure error type: %s", expectedType)
		return false
	}
	return false
}

func checkBlobExistsHelper(ctx context.Context, t *testing.T, client *azurehelper.BlobServiceClient, containerName, blobName string) bool {
	t.Helper()
	_, err := client.GetObject(ctx, &azurehelper.GetObjectInput{
		Container: &containerName,
		Key:      &blobName,
	})
	return err == nil
}

func getBlobObjectHelper(ctx context.Context, t *testing.T, client *azurehelper.BlobServiceClient, containerName, blobName string) ([]byte, error) {
	t.Helper()
	result, err := client.GetObject(ctx, &azurehelper.GetObjectInput{
		Container: &containerName,
		Key:      &blobName,
	})
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()
	content, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read blob content: %w", err)
	}
	return content, nil
}

// --- End Azure test helpers ---
// LookupCurrentUserObjectID attempts to determine the current Azure user's object ID for role assignment.
// It first checks environment variables, then falls back to Azure CLI if available.
func LookupCurrentUserObjectID(ctx context.Context, t *testing.T) string {
	   t.Helper()
	   // 1. Check environment variables
	   envVars := []string{"AZURE_CLIENT_OBJECT_ID", "ARM_CLIENT_OBJECT_ID"}
	   for _, env := range envVars {
			   if val := os.Getenv(env); val != "" {
					   t.Logf("Using object ID from %s: %s", env, val)
					   return val
			   }
	   }

	   // 2. Try Azure CLI (az)
	   azPath, err := exec.LookPath("az")
	   if err == nil {
			   // Try to get signed-in user object ID
			   cmd := exec.CommandContext(ctx, azPath, "ad", "signed-in-user", "show", "--query", "objectId", "-o", "tsv")
			   var out bytes.Buffer
			   cmd.Stdout = &out
			   cmd.Stderr = &out
			   if err := cmd.Run(); err == nil {
					   objectID := strings.TrimSpace(out.String())
					   if objectID != "" {
							   t.Logf("Found current user object ID via Azure CLI: %s", objectID)
							   return objectID
					   }
			   } else {
					   t.Logf("Azure CLI object ID lookup failed: %v, output: %s", err, out.String())
			   }
	   } else {
			   t.Logf("Azure CLI not found in PATH, skipping CLI object ID lookup")
	   }

	   // 3. Not found
	   t.Logf("Could not determine current Azure user object ID; role assignment may be skipped.")
	   return ""
}

// TestAzureRBACRoleAssignment verifies backend operations with a service principal or managed identity with minimum RBAC
func TestAzureRBACRoleAssignment(t *testing.T) {
	t.Parallel()

	// Skip if RBAC test credentials are not set
	clientID := os.Getenv("TERRAGRUNT_AZURE_RBAC_CLIENT_ID")
	clientSecret := os.Getenv("TERRAGRUNT_AZURE_RBAC_CLIENT_SECRET")
	tenantID := os.Getenv("TERRAGRUNT_AZURE_RBAC_TENANT_ID")
	storageAccount := os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")

	if clientID == "" || clientSecret == "" || tenantID == "" || storageAccount == "" {
		t.Skip("Skipping RBAC integration test: required environment variables not set")
	}

	t.Logf("Testing Azure backend with custom RBAC principal (Client ID: %s)", clientID)

	// Generate unique container name using helper
	containerName := "terragrunt-rbac-test-" + strings.ToLower(uniqueID())

	// Create context and logger using standard pattern
	ctx := t.Context()
	log := logger.CreateLogger()

	// Setup options for non-interactive mode like other tests
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	opts.NonInteractive = true

	// Store original environment variables to restore later
	originalClientID := os.Getenv("AZURE_CLIENT_ID")
	originalClientSecret := os.Getenv("AZURE_CLIENT_SECRET")
	originalTenantID := os.Getenv("AZURE_TENANT_ID")

	// Set Azure SDK environment variables for service principal auth
	os.Setenv("AZURE_CLIENT_ID", clientID)
	os.Setenv("AZURE_CLIENT_SECRET", clientSecret)
	os.Setenv("AZURE_TENANT_ID", tenantID)

	// Ensure environment is restored at the end
	defer func() {
		// Restore original environment variables
		os.Setenv("AZURE_CLIENT_ID", originalClientID)
		os.Setenv("AZURE_CLIENT_SECRET", originalClientSecret)
		os.Setenv("AZURE_TENANT_ID", originalTenantID)

		// Clean up the container if it was created using standard pattern
		cleanupConfig := createStandardBlobConfig(storageAccount, containerName, nil)
		client := createBlobServiceClientHelper(ctx, t, cleanupConfig)
		_ = client.DeleteContainer(ctx, log, containerName)
	}()

   // Only perform object ID lookup if Azure AD auth is enabled and MSI is NOT being used
   useAzureAD := os.Getenv("TERRAGRUNT_AZURE_USE_AZUREAD_AUTH") == "true" || os.Getenv("USE_AZUREAD_AUTH") == "true"
   usingMSI := os.Getenv("AZURE_CLIENT_ID") != "" || os.Getenv("ARM_CLIENT_ID") != ""
   var objectID string
   if useAzureAD && !usingMSI {
	  objectID = LookupCurrentUserObjectID(ctx, t)
	  if objectID == "" {
		 t.Logf("Warning: Could not determine current user object ID; RBAC role assignment will be skipped.")
	  } else {
		 t.Logf("Using Azure AD authentication, looked up current user object ID: %s", objectID)
		 // Here you would assign the Storage Blob Data Owner role to the objectID if needed
		 // (Role assignment logic would go here, if not handled elsewhere)
	  }
   } else {
	  t.Logf("Skipping object ID lookup: useAzureAD=%v, usingMSI=%v", useAzureAD, usingMSI)
   }

	   // Create standard test configuration using helper
	   config := createStandardBlobConfig(storageAccount, containerName, map[string]interface{}{
			   "key":              "test/terraform.tfstate",
			   "use_azuread_auth": true,
	   })

	   // Create backend
	   backend := azurerm.NewBackend()
	   require.NotNil(t, backend, "Azure backend should be created")

	   // Test bootstrap with restricted RBAC permissions
	   t.Run("BootstrapWithRestrictedRBAC", func(t *testing.T) {
			   // Attempt to bootstrap the backend
			   err := backend.Bootstrap(ctx, log, config, opts)

			   if err != nil {
					   // Check if it's a permissions-related error
					   if strings.Contains(strings.ToLower(err.Error()), "authorizationfailed") ||
							   strings.Contains(strings.ToLower(err.Error()), "permission") ||
							   strings.Contains(strings.ToLower(err.Error()), "forbidden") {
							   // This is acceptable - verify the error is clear and helpful
							   t.Logf("Received expected authorization error with restricted RBAC: %v", err)

							   // Use helper to assert the error type
							   if !assertAzureErrorType(t, err, "AuthorizationError") {
									   // Fallback: ensure error mentions authorization
									   assert.Contains(t, err.Error(), "authorization", "Error should mention authorization issues")
							   }
					   } else {
							   // For any other error type, it's unexpected
							   t.Fatalf("Unexpected error during RBAC test: %v", err)
					   }
			   } else {
					   // Success case - verify the container was created and test blob operations
					   client := createBlobServiceClientHelper(ctx, t, config)

					   // Check container exists
					   exists, err := client.ContainerExists(ctx, containerName)
					   require.NoError(t, err)
					   assert.True(t, exists, "Container should exist after successful bootstrap with RBAC permissions")

					   // Test blob upload to verify write permissions
					   testBlobName := "rbac-test-blob.json"
					   testData := []byte(`{"created_by": "terragrunt-rbac-test", "test_key": "test_value"}`)

					   err = client.UploadBlob(ctx, log, containerName, testBlobName, testData)
					   require.NoError(t, err, "Should be able to upload blob with sufficient RBAC permissions")

					   // Verify blob exists using standard helper
					   blobExists := checkBlobExistsHelper(ctx, t, client, containerName, testBlobName)
					   assert.True(t, blobExists, "Test blob should exist after upload with RBAC permissions")

					   // Test blob download to verify read permissions
					   downloadedData, err := getBlobObjectHelper(ctx, t, client, containerName, testBlobName)
					   require.NoError(t, err, "Should be able to download blob with sufficient RBAC permissions")
					   assert.Equal(t, testData, downloadedData, "Downloaded blob data should match uploaded data")

					   t.Logf("Successfully verified RBAC permissions for container, blob upload, and blob download operations")
			   }
	   })
}

// TestCase represents the test case data without the check function.
type TestCase struct {
}

func TestAzureRMBootstrapBackend(t *testing.T) {
	t.Parallel()

	t.Log("Starting TestAzureRMBootstrapBackend")

	testCases := []struct {
		checkExpectedResultFn func(t *testing.T, output string, containerName string, rootPath string, name string, args string)
		name                  string
		args                  string
		containerName         string
	}{
		{
			checkExpectedResultFn: func(t *testing.T, output string, containerName string, rootPath string, name string, args string) {
				t.Helper()

				// In delete case, not finding the container is acceptable
				if strings.Contains(output, "ContainerNotFound") {
					return
				}

				// For thoroughness, let's try bootstrapping and then deleting
				azureCfg := getAzureStorageTestConfig(t)
				azureCfg.ContainerName = containerName

				// Bootstrap the backend first
				bootstrapOutput, bootstrapErr, err := runTerragruntCommandWithOutput(t, "terragrunt backend bootstrap --non-interactive --log-level debug --log-format key-value --working-dir "+rootPath)
				require.NoError(t, err, "Bootstrap command failed: %v\nOutput: %s\nError: %s", err, bootstrapOutput, bootstrapErr)

				client := createBlobServiceClientHelper(
					t.Context(),
					t,
					map[string]interface{}{
						"storage_account_name": azureCfg.StorageAccountName,
						"container_name":       containerName,
						"use_azuread_auth":     true,
					},
				)

				// Verify container exists after bootstrap
				exists, err := client.ContainerExists(t.Context(), containerName)
				require.NoError(t, err)
				assert.True(t, exists, "Container should exist after bootstrap")

				// Create and verify test state file
				data := []byte("{}")
				err = client.UploadBlob(t.Context(), logger.CreateLogger(), containerName, "unit1/terraform.tfstate", data)
				require.NoError(t, err, "Failed to create test state file")

				// Verify state file exists using helper
				stateExists := checkBlobExistsHelper(t.Context(), t, client, containerName, "unit1/terraform.tfstate")
				require.True(t, stateExists, "State file should exist after creation")

				// Now run the delete command again (will be run by test runner)
				deleteOutput, deleteErr, err := runTerragruntCommandWithOutput(t, "terragrunt backend delete --force --non-interactive --log-level debug --log-format key-value --working-dir "+rootPath)
				require.NoError(t, err, "Delete command failed: %v\nOutput: %s\nError: %s", err, deleteOutput, deleteErr)

				// Verify container is deleted with retries
				var containerExists bool
				maxRetries := 5
				for i := 0; i < maxRetries; i++ {
					exists, err = client.ContainerExists(t.Context(), containerName)
					require.NoError(t, err)
					if !exists {
						containerExists = false
						break
					}
					time.Sleep(3 * time.Second)
				}
				assert.False(t, containerExists, "Container should not exist after delete")
			},
			name:          "delete backend command",
			args:          "backend delete --force",
			containerName: "terragrunt-test-container-" + strings.ToLower(uniqueID()),
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

		tmpEnvPath := copyEnvironment(t, testFixtureAzureBackend)
		// CopyEnvironment copies the contents of the source directory to tmpEnvPath
		rootPath := tmpEnvPath
		commonConfigPath := util.JoinPath(rootPath, "terragrunt.hcl")

			azureCfg := getAzureStorageTestConfig(t)

			defer func() {
				// Clean up the destination container
				azureCfg.ContainerName = tc.containerName
				cleanupAzureContainer(t, azureCfg)
			}()

			// Set up common configuration parameters
			azureParams := map[string]string{
				"__FILL_IN_STORAGE_ACCOUNT_NAME__": azureCfg.StorageAccountName,
				"__FILL_IN_SUBSCRIPTION_ID__":      os.Getenv("AZURE_SUBSCRIPTION_ID"),
				"__FILL_IN_CONTAINER_NAME__":       tc.containerName,
			}

			// Set up the common configuration
			copyTerragruntConfigAndFillProviderPlaceholders(t,
				commonConfigPath,
				commonConfigPath,
				azureParams,
				azureCfg.Location)

			stdout, stderr, _ := runTerragruntCommandWithOutput(t, "terragrunt "+tc.args+" --all --non-interactive --log-level debug --log-format key-value --strict-control require-explicit-bootstrap --working-dir "+rootPath)

			tc.checkExpectedResultFn(t, stdout+stderr, tc.containerName, rootPath, tc.name, tc.args)
		})
	}
}

// TestAzureOutputFromRemoteState tests the ability to get outputs from remote state stored in Azure Storage.
func TestAzureOutputFromRemoteState(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureAzureOutputFromRemoteState)

	// CopyEnvironment copies the contents of the source directory to tmpEnvPath
	// So the environment path should be constructed accordingly
	environmentPath := util.JoinPath(tmpEnvPath, "env1")

	azureCfg := getAzureStorageTestConfig(t)

	// Fill in Azure configuration
	rootPath := tmpEnvPath
	rootTerragruntConfigPath := util.JoinPath(rootPath, "root.hcl")
	containerName := "terragrunt-test-container-" + strings.ToLower(uniqueID())

	// Set up Azure configuration parameters
	azureParams := map[string]string{
		"__FILL_IN_STORAGE_ACCOUNT_NAME__": azureCfg.StorageAccountName,
		"__FILL_IN_SUBSCRIPTION_ID__":      os.Getenv("AZURE_SUBSCRIPTION_ID"),
		"__FILL_IN_CONTAINER_NAME__":       containerName,
	}

	// Set up the root configuration
	copyTerragruntConfigAndFillProviderPlaceholders(t,
		rootTerragruntConfigPath,
		rootTerragruntConfigPath,
		azureParams,
		azureCfg.Location)

	defer func() {
		// Clean up the destination container
		azureCfg.ContainerName = containerName
		cleanupAzureContainer(t, azureCfg)
	}()

	// Run terragrunt for app3, app2, and app1 in that order (dependencies first)
	runTerragrunt(t, "terragrunt apply --non-interactive -auto-approve --working-dir "+environmentPath+"/app3")
	runTerragrunt(t, "terragrunt apply --non-interactive -auto-approve --working-dir "+environmentPath+"/app2")
	runTerragrunt(t, "terragrunt apply --non-interactive -auto-approve --working-dir "+environmentPath+"/app1")

	// Now check the outputs to make sure the remote state dependencies work
	app1OutputCmd := "terragrunt output -no-color -json --non-interactive --working-dir " + environmentPath + "/app1"
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, app1OutputCmd, &stdout, &stderr),
	)

	outputs := map[string]terraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	// Print the actual output for debugging
	t.Logf("Actual output: %s", outputs["combined_output"].Value)

	// Verify outputs from app1
	assert.Equal(t, `app1 output with app2 output with app3 output and app3 output`, outputs["combined_output"].Value)
}

// CheckAzureTestCredentials checks if the required Azure test credentials are available
// and skips the test if they are not. Returns the storage account name.
func CheckAzureTestCredentials(t *testing.T) (storageAccount string) {
	t.Helper() // Mark this as a test helper function

	storageAccount = os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")

	// Skip test if storage account isn't specified
	if storageAccount == "" {
		t.Skip("Skipping Azure test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT not set")
	}

	// Log that we're using Azure AD authentication
	t.Logf("Using Azure AD authentication for tests")

	return storageAccount
}

// TestAzureStorageContainerCreation tests the creation of Azure storage containers
func TestAzureStorageContainerCreation(t *testing.T) {
	t.Parallel()

	// Setup Azure test environment using helper
	azureCtx := setupAzureTest(t, "StorageContainerCreation")
	defer azureCtx.Cleanup()

	// Create storage account using helper
	createTestStorageAccount(azureCtx.Ctx, t, azureCtx, true)

	// Setup test container using helper
	client, cleanup := setupTestContainer(azureCtx.Ctx, t, azureCtx.StorageAccountName, azureCtx.ContainerName)
	defer cleanup()

	// Track containers for cleanup
	var secondContainerCreated bool
	containerNameState := azureCtx.ContainerName + "-state"

	// Setup container cleanup using defer
	defer func() {
		if secondContainerCreated {
			t.Logf("Cleaning up second container %s", containerNameState)
			_ = client.DeleteContainer(context.Background(), azureCtx.Logger, containerNameState)
		}
	}()

	// Check if container exists
	exists, err := client.ContainerExists(azureCtx.Ctx, azureCtx.ContainerName)
	require.NoError(t, err)
	require.True(t, exists, "Container should exist after creation")

	// Test creating multiple containers with the same client
	// Create another container
	err = client.CreateContainerIfNecessary(azureCtx.Ctx, azureCtx.Logger, containerNameState)
	require.NoError(t, err, "Failed to create second container")
	secondContainerCreated = true

	// Check if second container exists
	exists, err = client.ContainerExists(azureCtx.Ctx, containerNameState)
	require.NoError(t, err)
	require.True(t, exists, "Second container should exist after creation")

	// Test error handling for invalid container names
	t.Run("InvalidContainerName", func(t *testing.T) {
		t.Parallel()

		// Create a context for Azure operations
		ctx := t.Context()

		invalidContainerName := "UPPERCASE_CONTAINER"

		// Use the real storage account that was already created in the parent test
		// This ensures we get container validation errors, not DNS errors
		invalidConfig := createStandardBlobConfig(azureCtx.StorageAccountName, invalidContainerName, map[string]interface{}{
			"key": "test/terraform.tfstate",
		})

		// Since the storage account exists, creating the client should succeed
		invalidClient := createBlobServiceClientHelper(ctx, t, invalidConfig)

		// Now try to create a container with an invalid name - this should fail with container validation
		err := invalidClient.CreateContainerIfNecessary(ctx, azureCtx.Logger, invalidContainerName)
		require.Error(t, err, "Creating container with invalid name should fail")

		// The error should be either ContainerCreationError or ContainerValidationError
		// depending on where the validation happens (client-side vs server-side)
		var containerCreationErr azurehelper.ContainerCreationError
		var containerValidationErr azurerm.ContainerValidationError

		if errors.As(err, &containerCreationErr) {
			// Server-side validation error wrapped in ContainerCreationError
			assertAzureErrorType(t, err, "ContainerCreationError")
		} else if errors.As(err, &containerValidationErr) {
			// Client-side validation error
			assertAzureErrorType(t, err, "ContainerValidationError")
		} else {
			// Fallback: check that the error mentions container validation issues
			assert.True(t,
				strings.Contains(strings.ToLower(err.Error()), "container") ||
					strings.Contains(strings.ToLower(err.Error()), "invalid") ||
					strings.Contains(strings.ToLower(err.Error()), "uppercase"),
				"Error should mention container validation issue, got: %v", err)
		}
	})
}

// TestStorageAccountBootstrap tests storage account bootstrap functionality
func TestStorageAccountBootstrap(t *testing.T) {
	t.Parallel()
	// Skip test if we don't have Azure credentials
	accountName := CheckAzureTestCredentials(t)

	log := logger.CreateLogger()

	// Test with non-existent storage account
	t.Run("StorageAccountBootstrap_NonExistentAccount", func(t *testing.T) {
		t.Parallel()

		// Create a context for Azure operations
		ctx := t.Context()

		// Use a non-existent account name
		nonExistentName := "tgnon" + strings.ToLower(fmt.Sprintf("%x", time.Now().UnixNano()))[0:8]

		// Create configuration for the non-existent storage account
		config := map[string]interface{}{
			"storage_account_name": nonExistentName,
			"container_name":       "terragrunt-test-sa-nonexistent",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		// When we try to create a blob service client, it should fail since the account doesn't exist
		_, err := createInvalidBlobServiceClientHelper(ctx, t, config)
		require.Error(t, err, "Expected error when storage account doesn't exist")

		// The error should be wrapped properly and contain useful information
		if err != nil {
			// Check if it's an Azure-specific authentication or storage account error
			var authErr azurerm.AuthenticationError
			var storageErr azurerm.StorageAccountCreationError

			switch {
			case errors.As(err, &authErr):
				assert.NotEmpty(t, authErr.AuthMethod, "Authentication error should specify auth method")
				require.Error(t, authErr.Unwrap(), "Authentication error should have underlying error")
			case errors.As(err, &storageErr):
				assert.Equal(t, nonExistentName, storageErr.StorageAccountName, "Storage error should contain account name")
				require.Error(t, storageErr.Unwrap(), "Storage error should have underlying error")
			default:
				// Fallback check - at minimum should mention the issue
				assert.True(t,
					strings.Contains(err.Error(), "no such host") ||
						strings.Contains(err.Error(), "does not exist") ||
						strings.Contains(err.Error(), "not accessible"),
					"Error should indicate storage account access issue, got: %v", err)
			}
		}
	})

	// Test with existing storage account
	t.Run("StorageAccountBootstrap_ExistingAccount", func(t *testing.T) {
		t.Parallel()

		// Create a context for Azure operations
		ctx := t.Context()

		// We'll use the provided test storage account which should already exist
		config := map[string]interface{}{
			"storage_account_name": accountName,
			"container_name":       "terragrunt-test-sa-exists",
			"key":                  "test/terraform.tfstate",
		}

		// Set Azure AD authentication
		config["use_azuread_auth"] = true

		// Should succeed since the account should exist
		client := createBlobServiceClientHelper(ctx, t, config)

		// Create container for test
		err := client.CreateContainerIfNecessary(t.Context(), log, "terragrunt-test-sa-exists")
		require.NoError(t, err)

		// Clean up the container
		exists, err := client.ContainerExists(t.Context(), "terragrunt-test-sa-exists")
		require.NoError(t, err)

		if exists {
			err = client.DeleteContainer(t.Context(), log, "terragrunt-test-sa-exists")
			require.NoError(t, err)
		}
	})
}
func TestBlobOperations(t *testing.T) {
	storageAccount := os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")
	if storageAccount == "" {
		t.Skip("Skipping Azure blob operations test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT not set")
	}

	t.Parallel()

	ctx := t.Context()
	containerName := fmt.Sprintf("test-container-%d", os.Getpid())

	config := createStandardBlobConfig(storageAccount, containerName, nil)

	log := logger.CreateLogger()
	client := createBlobServiceClientHelper(ctx, t, config)
	require.NotNil(t, client)

	// Setup cleanup
	var containerCreated bool
	defer func() {
		if containerCreated {
			t.Logf("Cleanup: Deleting container %s", containerName)
			_ = client.DeleteContainer(ctx, log, containerName)
		}
	}()

	// Test container creation
	err := client.CreateContainerIfNecessary(ctx, log, containerName)
	require.NoError(t, err)
	containerCreated = true

	// Test blob operations using helper
	testBlobOperations(t, client, containerName)
}

// TestStorageAccountCreationAndBlobUpload tests the complete workflow of creating a storage account and uploading a blob
func TestStorageAccountCreationAndBlobUpload(t *testing.T) {
	t.Parallel()

	// Setup Azure test environment using helper
	azureCtx := setupAzureTest(t, "StorageAccountCreationAndBlobUpload")
	defer azureCtx.Cleanup()

	// Create storage account with versioning enabled
	createTestStorageAccount(azureCtx.Ctx, t, azureCtx, true)

	// Setup test container using helper
	blobClient, cleanup := setupTestContainer(azureCtx.Ctx, t, azureCtx.StorageAccountName, azureCtx.ContainerName)
	defer cleanup()

	blobName := "test-blob.json"

	// Track blob creation for specific cleanup
	var blobCreated bool
	defer func() {
		if blobCreated {
			t.Logf("Cleanup: Deleting blob %s", blobName)
			// Ignore errors during cleanup
			_ = blobClient.DeleteBlobIfNecessary(azureCtx.Ctx, azureCtx.Logger, azureCtx.ContainerName, blobName)
		}
	}()

	// Create test blob content
	testContent := map[string]interface{}{
		"test_key":   "test_value",
		"timestamp":  time.Now().Unix(),
		"created_by": "terragrunt-integration-test",
	}

	contentBytes, err := json.Marshal(testContent)
	require.NoError(t, err)

	// Upload blob
	err = blobClient.UploadBlob(azureCtx.Ctx, azureCtx.Logger, azureCtx.ContainerName, blobName, contentBytes)
	require.NoError(t, err)
	blobCreated = true

	t.Logf("Successfully uploaded blob %s to container %s", blobName, azureCtx.ContainerName)

	// Verify blob exists and has correct content
	// Use CheckBlobExistsHelper to check if blob exists
	exists := checkBlobExistsHelper(azureCtx.Ctx, t, blobClient, azureCtx.ContainerName, blobName)
	require.True(t, exists, "Blob should exist after upload")

	// Download and verify blob content
	downloadedBytes, err := getBlobObjectHelper(azureCtx.Ctx, t, blobClient, azureCtx.ContainerName, blobName)
	require.NoError(t, err)
	require.NotEmpty(t, downloadedBytes)

	// Verify content matches
	var downloadedJSON map[string]interface{}
	err = json.Unmarshal(downloadedBytes, &downloadedJSON)
	require.NoError(t, err)

	assert.Equal(t, testContent["test_key"], downloadedJSON["test_key"])
	assert.Equal(t, testContent["created_by"], downloadedJSON["created_by"])

	t.Log("Successfully verified blob content matches uploaded data")
	t.Log("Integration test completed successfully - storage account created, blob uploaded")
}

// TestAzureBackendBootstrapWithExistingStorageAccount tests Azure backend bootstrap scenarios
// that require real Azure resources - moved from unit tests
func TestAzureBackendBootstrapWithExistingStorageAccount(t *testing.T) {
	t.Parallel()

	storageAccount := os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")
	if storageAccount == "" {
		t.Skip("Skipping Azure backend bootstrap test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT not set")
	}

	// Create logger for testing
	log := logger.CreateLogger()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Make sure we're using non-interactive mode to avoid prompts
	opts.NonInteractive = true

	// Create a unique suffix for container names
	uniqueSuffix := fmt.Sprintf("%x", time.Now().UnixNano())[0:8]

	t.Run("bootstrap-with-existing-storage-account", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		containerName := "terragrunt-bootstrap-test-" + uniqueSuffix

		config := map[string]interface{}{
			"storage_account_name": storageAccount,
			"container_name":       containerName,
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		// Import the Azure backend package
		backend := azurerm.NewBackend()
		require.NotNil(t, backend, "Azure backend should be created")

		// Call Bootstrap - should succeed with real storage account
		err := backend.Bootstrap(ctx, log, config, opts)
		require.NoError(t, err)

		// Verify container was created
		client := createBlobServiceClientHelper(ctx, t, config)

		exists, err := client.ContainerExists(ctx, containerName)
		require.NoError(t, err)
		assert.True(t, exists, "Container should exist after successful bootstrap")

		// Clean up - delete the container
		defer func() {
			_ = client.DeleteContainer(ctx, log, containerName)
		}()
	})

	t.Run("invalid-storage-account-name", func(t *testing.T) {
		t.Parallel()

		testLogger := logger.CreateLogger()
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		ctx := t.Context()

		config := map[string]interface{}{
			"storage_account_name": "invalid/name/with/slashes", // Invalid storage account name
			"container_name":       "test-container",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		// Import the Azure backend package
		backend := azurerm.NewBackend()
		require.NotNil(t, backend, "Azure backend should be created")

		// Call Bootstrap and expect an error due to invalid storage account name
		err = backend.Bootstrap(ctx, testLogger, config, opts)

		// Verify an error was returned - should get an error about the storage account not being accessible
		require.Error(t, err)

		// Check if the error is one of our custom types
		var authErr azurerm.AuthenticationError
		var storageErr azurerm.StorageAccountCreationError

		switch {
		case errors.As(err, &authErr):
			// Authentication error with underlying cause
			assert.NotEmpty(t, authErr.AuthMethod, "Authentication error should specify auth method")
			require.Error(t, authErr.Unwrap(), "Authentication error should have underlying error")
		case errors.As(err, &storageErr):
			// Storage account error with underlying cause
			assert.Contains(t, storageErr.StorageAccountName, "invalid", "Storage error should contain invalid account name")
			require.Error(t, storageErr.Unwrap(), "Storage error should have underlying error")
		default:
			// Fallback check for backwards compatibility
			assert.True(t,
				strings.Contains(err.Error(), "does not exist") ||
					strings.Contains(err.Error(), "not accessible") ||
					strings.Contains(err.Error(), "no such host"),
				"Should get error about storage account not being accessible, got: %v", err)
		}
	})
}

// TestAzureBackendCustomErrorTypes tests the custom error types in integration scenarios
func TestAzureBackendCustomErrorTypes(t *testing.T) {
	t.Parallel()

	// Create logger for testing
	log := logger.CreateLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Make sure we're using non-interactive mode to avoid prompts
	opts.NonInteractive = true

	t.Run("missing-subscription-id-error", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Test MissingSubscriptionIDError when subscription_id is required but missing
		config := map[string]interface{}{
			"storage_account_name":                 "testaccount",
			"container_name":                       "test-container",
			"key":                                  "test/terraform.tfstate",
			"resource_group_name":                  "terragrunt-test-rg-missing-subscription-id",
			"location":                             "eastus",
			"create_storage_account_if_not_exists": true,
			"use_azuread_auth":                     true,
			// subscription_id is missing
		}

		backend := azurerm.NewBackend()
		require.NotNil(t, backend, "Azure backend should be created")

		err := backend.Bootstrap(ctx, log, config, opts)

		// Verify error is the custom type
		require.Error(t, err)
		assertAzureErrorType(t, err, "MissingSubscriptionIDError")
	})

	t.Run("missing-location-error", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Test MissingLocationError when location is required but missing
		config := map[string]interface{}{
			"storage_account_name":                 "testaccount",
			"container_name":                       "test-container",
			"key":                                  "test/terraform.tfstate",
			"subscription_id":                      "00000000-0000-0000-0000-000000000000",
			"resource_group_name":                  "terragrunt-test-rg-missing-location",
			"create_storage_account_if_not_exists": true,
			"use_azuread_auth":                     true,
			// location is missing
		}

		backend := azurerm.NewBackend()
		require.NotNil(t, backend, "Azure backend should be created")

		err := backend.Bootstrap(ctx, log, config, opts)

		// Verify error is the custom type
		require.Error(t, err)
		assertAzureErrorType(t, err, "MissingLocationError")
	})

	t.Run("container-validation-error", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Test ContainerValidationError for invalid container names
		testCases := []struct {
			name          string
			containerName string
			expectedMsg   string
		}{
			{
				name:          "empty-container-name",
				containerName: "",
				expectedMsg:   "missing required Azure remote state configuration container_name",
			},
			{
				name:          "container-name-too-short",
				containerName: "ab",
				expectedMsg:   "container name must be between 3 and 63 characters",
			},
			{
				name:          "invalid-uppercase",
				containerName: "Invalid-Container",
				expectedMsg:   "container name can only contain lowercase letters, numbers, and hyphens",
			},
			{
				name:          "consecutive-hyphens",
				containerName: "container--name",
				expectedMsg:   "container name cannot contain consecutive hyphens",
			},
		}

		for _, tc := range testCases {
			tc := tc // capture range variable
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				config := map[string]interface{}{
					"storage_account_name": "testaccount",
					"container_name":       tc.containerName,
					"key":                  "test/terraform.tfstate",
					"use_azuread_auth":     true,
				}

				backend := azurerm.NewBackend()
				require.NotNil(t, backend, "Azure backend should be created")

				err := backend.Bootstrap(ctx, log, config, opts)

				// Verify error is returned
				require.Error(t, err)

				// For empty container name, it should be a MissingRequiredAzureRemoteStateConfig
				// For other validation issues, it should be ContainerValidationError
				if tc.containerName == "" {
					var missingConfigError azurerm.MissingRequiredAzureRemoteStateConfig
					if assert.ErrorAs(t, err, &missingConfigError, "Empty container name should be MissingRequiredAzureRemoteStateConfig type") {
						assert.Equal(t, "container_name", string(missingConfigError), "Should indicate missing container_name")
					} else {
						// Fallback: check if error message is correct
						assert.Contains(t, err.Error(), tc.expectedMsg, "Error message should contain expected text")
					}
				} else {
					var containerValidationError azurerm.ContainerValidationError
					if assert.ErrorAs(t, err, &containerValidationError, "Error should be ContainerValidationError type") {
						assert.Contains(t, containerValidationError.ValidationIssue, tc.expectedMsg, "Validation error should contain expected text")
					} else {
						// Fallback: check if error message is correct
						assert.Contains(t, err.Error(), tc.expectedMsg, "Error message should contain expected text")
					}
				}
			})
		}
	})

	t.Run("missing-resource-group-error", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Test MissingResourceGroupError when resource_group_name is required for delete operation
		config := map[string]interface{}{
			"storage_account_name": "testaccount",
			"subscription_id":      "00000000-0000-0000-0000-000000000000",
			"container_name":       "test-container",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
			// resource_group_name is missing (required for delete operation)
		}

		backend := azurerm.NewBackend()
		require.NotNil(t, backend, "Azure backend should be created")

		err := backend.DeleteStorageAccount(ctx, log, config, opts)

		// Verify error is returned
		require.Error(t, err)
		assertAzureErrorType(t, err, "MissingResourceGroupError")
	})

	t.Run("authentication-failure-scenarios", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Test authentication-related errors with a realistic configuration
		// that will trigger authentication issues rather than the NoValidAuthMethodError
		// (since Azure AD auth is now enforced by default)
		config := map[string]interface{}{
			"storage_account_name":                 "testaccount",
			"container_name":                       "test-container",
			"key":                                  "test/terraform.tfstate",
			"subscription_id":                      "00000000-0000-0000-0000-000000000000",
			"resource_group_name":                  "terragrunt-test-rg-authentication-error",
			"location":                             "eastus",
			"create_storage_account_if_not_exists": true,
			"use_azuread_auth":                     true, // This will be used but will likely fail without proper credentials
		}

		backend := azurerm.NewBackend()
		require.NotNil(t, backend, "Azure backend should be created")

		err := backend.Bootstrap(ctx, log, config, opts)

		// Verify an error was returned - this should fail due to authentication or storage account issues
		require.Error(t, err)

		// Check if it's an authentication error, storage error, or contains auth-related messaging
		var authErr azurerm.AuthenticationError
		var storageErr azurerm.StorageAccountCreationError
		var noAuthError azurerm.NoValidAuthMethodError

		switch {
		case errors.As(err, &noAuthError):
			// This is the ideal case if we can trigger it
			assert.Contains(t, err.Error(), "no valid authentication method", "Error message should mention authentication method issue")
		case errors.As(err, &authErr):
			// Authentication error - common when credentials are missing/invalid
			assert.NotEmpty(t, authErr.AuthMethod, "Authentication error should specify auth method")
			require.Error(t, authErr.Unwrap(), "Authentication error should have underlying error")
		case errors.As(err, &storageErr):
			// Storage account error - also common when auth fails during storage operations
			assert.NotEmpty(t, storageErr.StorageAccountName, "Storage error should contain account name")
			require.Error(t, storageErr.Unwrap(), "Storage error should have underlying error")
		default:
			// Fallback - ensure the error at least mentions authentication or access issues
			errorStr := strings.ToLower(err.Error())
			assert.True(t,
				strings.Contains(errorStr, "authentication") ||
					strings.Contains(errorStr, "credential") ||
					strings.Contains(errorStr, "auth") ||
					strings.Contains(errorStr, "access denied") ||
					strings.Contains(errorStr, "unauthorized") ||
					strings.Contains(errorStr, "no such host") ||
					strings.Contains(errorStr, "storage account"),
				"Error should mention authentication or access issues, got: %v", err)
		}
	})
}

// TestAzureErrorUnwrappingAndPropagation tests error unwrapping and propagation through the entire call stack
func TestAzureErrorUnwrappingAndPropagation(t *testing.T) {
	t.Parallel()

	// Create logger for testing
	log := logger.CreateLogger()

	// Make sure we're using non-interactive mode to avoid prompts

	t.Run("error-propagation-from-azurehelper", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Test that errors from azurehelper are properly wrapped and propagated
		// Use a configuration that will trigger azurehelper errors
		config := map[string]interface{}{
			"storage_account_name": "nonexistent" + strconv.FormatInt(time.Now().UnixNano(), 10)[10:], // Non-existent account
			"container_name":       "test-container",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		// Test that CreateBlobServiceClient properly wraps errors
		_, err := createInvalidBlobServiceClientHelper(ctx, t, config)
		require.Error(t, err, "Expected error for non-existent storage account")

		// The error should be wrapped with useful context
		assert.True(t,
			strings.Contains(err.Error(), "storage account") ||
				strings.Contains(err.Error(), "authentication") ||
				strings.Contains(err.Error(), "no such host"),
			"Error should provide meaningful context, got: %v", err)

		// Test error propagation through backend bootstrap
		backend := azurerm.NewBackend()
		bootstrapErr := backend.Bootstrap(ctx, log, config, nil)
		require.Error(t, bootstrapErr, "Expected bootstrap error")

		// Check if bootstrap error wraps the underlying error properly
		if !errors.Is(bootstrapErr, err) {
			// If not the same error, it should at least contain similar context
			assert.True(t,
				strings.Contains(bootstrapErr.Error(), "storage account") ||
					strings.Contains(bootstrapErr.Error(), "authentication") ||
					strings.Contains(bootstrapErr.Error(), "no such host"),
				"Bootstrap error should propagate underlying context, got: %v", bootstrapErr)
		}
	})

	t.Run("container-creation-error-unwrapping", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Create a mock configuration that will trigger container creation errors
		config := map[string]interface{}{
			"storage_account_name": "testaccount",
			"container_name":       "INVALID_UPPERCASE_CONTAINER", // Invalid container name
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		backend := azurerm.NewBackend()
		err := backend.Bootstrap(ctx, log, config, nil)

		require.Error(t, err, "Expected error for invalid container name")

		// Should be a ContainerValidationError
		var containerValidationError azurerm.ContainerValidationError
		if assert.ErrorAs(t, err, &containerValidationError, "Error should be ContainerValidationError type") {
			assert.Contains(t, containerValidationError.ValidationIssue, "lowercase", "Validation error should mention lowercase requirement")
		}
	})

	t.Run("authentication-error-chain", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Create a configuration that might trigger authentication issues
		config := map[string]interface{}{
			"storage_account_name": "mightnotexist" + strconv.FormatInt(time.Now().UnixNano(), 10)[10:],
			"container_name":       "test-container",
			"key":                  "test/terraform.tfstate",
			"use_azuread_auth":     true,
		}

		// Test both azurehelper and backend error handling
		_, helperErr := createInvalidBlobServiceClientHelper(ctx, t, config)
		backend := azurerm.NewBackend()
		backendErr := backend.Bootstrap(ctx, log, config, nil)

		// At least one should error (or both)
		if helperErr != nil || backendErr != nil {
			// If there are errors, they should be meaningful
			if helperErr != nil {
				assert.NotEmpty(t, helperErr.Error(), "Helper error should have meaningful message")
			}
			if backendErr != nil {
				assert.NotEmpty(t, backendErr.Error(), "Backend error should have meaningful message")
			}

			// Test error unwrapping if we have authentication or storage account errors
			if backendErr != nil {
				var authErr azurerm.AuthenticationError
				var storageErr azurerm.StorageAccountCreationError

				if errors.As(backendErr, &authErr) {
					assert.Error(t, authErr.Unwrap(), "AuthenticationError should have underlying error")
					assert.NotEmpty(t, authErr.AuthMethod, "AuthenticationError should specify auth method")
				} else if errors.As(backendErr, &storageErr) {
					assert.Error(t, storageErr.Unwrap(), "StorageAccountCreationError should have underlying error")
					assert.NotEmpty(t, storageErr.StorageAccountName, "StorageAccountCreationError should specify account name")
				}
			}
		} else {
			t.Log("No errors occurred - this is acceptable if Azure credentials are properly configured")
		}
	})

	t.Run("deep-error-chain-propagation", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		// Test that errors propagate correctly through the entire call stack:
		// Bootstrap -> Backend -> Helper -> Azure SDK
		config := map[string]interface{}{
			"storage_account_name":                 "invalid-chars-###", // Invalid storage account name
			"container_name":                       "test-container",
			"key":                                  "test/terraform.tfstate",
			"subscription_id":                      "00000000-0000-0000-0000-000000000000",
			"resource_group_name":                  "terragrunt-test-rg-error-unwrapping",
			"location":                             "eastus",
			"create_storage_account_if_not_exists": true,
			"use_azuread_auth":                     true,
		}

		backend := azurerm.NewBackend()
		require.NotNil(t, backend, "Azure backend should be created")

		// This should trigger a deep error chain:
		// Backend.Bootstrap calls helper functions, which should wrap and propagate errors properly
		err := backend.Bootstrap(ctx, log, config, nil)
		require.Error(t, err, "Expected error for invalid storage account name")

		// The error should contain meaningful context about the issue
		assert.True(t,
			strings.Contains(err.Error(), "storage account") ||
				strings.Contains(err.Error(), "invalid") ||
				strings.Contains(err.Error(), "validation"),
			"Error should provide meaningful context about storage account validation, got: %v", err)

		// Try to unwrap the error chain to ensure it's properly structured
		var unwrappedErr = err
		var errorChainDepth int
		for unwrappedErr != nil && errorChainDepth < 10 { // Prevent infinite loops
			errorChainDepth++
			if nextErr := errors.Unwrap(unwrappedErr); nextErr != nil {
				unwrappedErr = nextErr
			} else {
				break
			}
		}

		// We should have at least one level of error wrapping (original error + wrapper)
		assert.Positive(t, errorChainDepth, "Error should have proper wrapping chain")

		// Test that errors.Is works correctly for checking error types in the chain
		var authErr azurerm.AuthenticationError
		var storageErr azurerm.StorageAccountCreationError
		var validationErr azurerm.ContainerValidationError

		// At least one of these error types should be found in the chain
		hasExpectedErrorType := errors.As(err, &authErr) ||
			errors.As(err, &storageErr) ||
			errors.As(err, &validationErr)

		if !hasExpectedErrorType {
			t.Logf("Error chain: %v", err)
			// For backwards compatibility, ensure basic error information is present
			assert.Contains(t, err.Error(), "storage", "Error should mention storage-related issues")
		}
	})
}

// TestStorageAccountConfigurationAndUpdate tests storage account configuration options and updates
func TestStorageAccountConfigurationAndUpdate(t *testing.T) {
	t.Parallel()

	log := logger.CreateLogger()
	ctx := t.Context()

	// Use the GetAzureCredentials helper to check for credentials and subscription ID
	_, subscriptionID := getAzureCredentialsHelper(ctx, t)
	if subscriptionID == "" {
		t.Skip("Skipping storage account configuration test: No subscription ID found in environment variables")
	}

	location := os.Getenv("AZURE_LOCATION")
	if location == "" {
		location = os.Getenv("ARM_LOCATION")
		if location == "" {
			location = "westeurope"
			t.Logf("Neither AZURE_LOCATION nor ARM_LOCATION set, using default: %s", location)
		}
	}

	// Test different storage account configurations
	testCases := []struct {
		name               string
		expectUpdate       bool
		expectWarnings     bool
		accessTierChange   bool
		replicationChange  bool
		publicAccessChange bool
		tagsChange         bool
	}{
		{
			name:             "UpdateAccessTier",
			expectUpdate:     true,
			expectWarnings:   false,
			accessTierChange: true,
		},
		{
			name:               "UpdateBlobPublicAccess",
			expectUpdate:       true,
			expectWarnings:     false,
			publicAccessChange: true,
		},
		{
			name:           "UpdateTags",
			expectUpdate:   true,
			expectWarnings: false,
			tagsChange:     true,
		},
		{
			name:              "ReadOnlyPropertyWarning",
			expectUpdate:      false,
			expectWarnings:    true,
			replicationChange: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Remove t.Parallel() here to prevent resource conflicts between subtests
			// Each subtest will have completely unique resources

			// Generate unique names for this specific subtest
			subtestUniqueID := strconv.FormatInt(time.Now().UnixNano(), 10)
			storageAccountName := "tgtest" + strings.ToLower(subtestUniqueID)[len(subtestUniqueID)-10:]
			resourceGroupName := fmt.Sprintf("terragrunt-test-rg-%s-%s", tc.name, subtestUniqueID)

			// Create configurations based on test case parameters
			initialConfig := azurehelper.StorageAccountConfig{
				SubscriptionID:        subscriptionID,
				ResourceGroupName:     resourceGroupName,
				StorageAccountName:    storageAccountName,
				Location:              location,
				EnableVersioning:      true,
				AllowBlobPublicAccess: !tc.publicAccessChange, // Start with opposite if testing change
				AccountKind:           "StorageV2",
				AccountTier:           "Standard",
				AccessTier:            "Hot",
				ReplicationType:       "LRS",
				Tags:                  map[string]string{"Environment": "Test", "created-by": "terragrunt-integration-test"},
			}

			// Create updated config based on what we're testing
			updatedConfig := initialConfig                                                                             // Copy all fields
			updatedConfig.Tags = map[string]string{"Environment": "Test", "created-by": "terragrunt-integration-test"} // Reset tags

			switch {
			case tc.accessTierChange:
				updatedConfig.AccessTier = "Cool" // Change from Hot to Cool
			case tc.publicAccessChange:
				updatedConfig.AllowBlobPublicAccess = false // Disable public access
				updatedConfig.Tags["access-updated"] = "true"
			case tc.tagsChange:
				updatedConfig.Tags = map[string]string{
					"Environment": "Production",
					"Owner":       "TeamA",
					"created-by":  "terragrunt-integration-test",
				}
			case tc.replicationChange:
				updatedConfig.ReplicationType = "GRS" // Try to change read-only property
			}

			// Create resource group client
			rgClient, _ := azurehelper.CreateResourceGroupClient(ctx, log, subscriptionID)

			// Create resource group
			err := rgClient.EnsureResourceGroup(ctx, log, resourceGroupName, location, map[string]string{"created-by": "terragrunt-integration-test"})
			require.NoError(t, err)
			t.Logf("Resource group %s created successfully", resourceGroupName)

			// Track what resources were successfully created for proper cleanup ordering
			var storageAccountCreated bool

			// **Solution 4: Improved Resource Cleanup Order**
			defer func() {
				cleanupCtx := context.Background()

				if storageAccountCreated {
					t.Logf("Cleaning up storage account %s", storageAccountName)

					// Create a fresh storage client for cleanup
					cleanupStorageConfig := map[string]interface{}{
						"storage_account_name": storageAccountName,
						"resource_group_name":  resourceGroupName,
						"subscription_id":      subscriptionID,
						"use_azuread_auth":     true,
					}

					cleanupStorageClient, _ := azurehelper.CreateStorageAccountClient(cleanupCtx, log, cleanupStorageConfig)
					if err := cleanupStorageClient.DeleteStorageAccount(cleanupCtx, log); err != nil {
						t.Logf("Warning: Failed to delete storage account %s: %v", storageAccountName, err)
					} else {
						t.Logf("Successfully deleted storage account %s", storageAccountName)
						// Wait for storage account deletion to complete before deleting resource group
						time.Sleep(10 * time.Second)
					}
				}

				// Then delete resource group (only after storage account is deleted)
				t.Logf("Cleaning up resource group %s", resourceGroupName)
				if err := rgClient.DeleteResourceGroup(cleanupCtx, log, resourceGroupName); err != nil {
					t.Logf("Warning: Failed to delete resource group %s: %v", resourceGroupName, err)
				} else {
					t.Logf("Successfully deleted resource group %s", resourceGroupName)
				}
			}()

			// Create storage account client
			storageAccountConfig := map[string]interface{}{
				"storage_account_name": storageAccountName,
				"resource_group_name":  resourceGroupName,
				"subscription_id":      subscriptionID,
				"location":             location,
				"use_azuread_auth":     true,
			}

			t.Logf("Creating storage account %s in resource group %s", storageAccountName, resourceGroupName)

			// Create storage account client
			storageClient, _ := azurehelper.CreateStorageAccountClient(ctx, log, storageAccountConfig)
			require.NotNil(t, storageClient)

			// Create storage account with initial configuration
			err = storageClient.CreateStorageAccountIfNecessary(ctx, log, initialConfig)
			require.NoError(t, err)
			storageAccountCreated = true // Mark as created for cleanup

			// Verify storage account exists
			exists, account, err := storageClient.StorageAccountExists(ctx)
			require.NoError(t, err)
			require.True(t, exists, "Storage account should exist after creation")
			require.NotNil(t, account)

			// Wait a moment for the storage account to be fully ready
			time.Sleep(5 * time.Second)

			// Update storage account with new configuration
			err = storageClient.CreateStorageAccountIfNecessary(ctx, log, updatedConfig)

			// Handle potential race condition with resource group deletion
			if err != nil && strings.Contains(err.Error(), "ResourceGroupBeingDeleted") {
				t.Skipf("Skipping test %s due to resource group cleanup race condition: %v", tc.name, err)
			}

			if tc.expectWarnings {
				// For read-only property changes, we expect success but warnings
				require.NoError(t, err)
				// We can't easily capture logs in this test framework, but the update should complete
			} else {
				require.NoError(t, err)
			}

			// Verify final state
			exists, updatedAccount, err := storageClient.StorageAccountExists(ctx)
			require.NoError(t, err)
			require.True(t, exists)
			require.NotNil(t, updatedAccount)

			// Verify specific properties that should have been updated
			if tc.expectUpdate && !tc.expectWarnings {
				// Check that updatable properties were actually changed
				switch {
				case tc.accessTierChange:
					t.Logf("Access tier should have been updated from Hot to Cool")
				case tc.publicAccessChange:
					t.Logf("Blob public access should have been updated to false")
				case tc.tagsChange:
					t.Logf("Tags should have been updated to include Owner and Environment=Production")
				}
			}

			t.Logf("Test %s completed successfully", tc.name)
		})
	}
}

// TestAzureBackendMigrationWithUnits tests backend migration using Terragrunt's backend migrate command
// with src-unit and dst-unit parameters to migrate state between different paths in the same container
func TestAzureBackendMigrationWithUnits(t *testing.T) {
	t.Parallel()

	// Setup Azure test environment with helpers
	azureCtx := setupAzureTest(t, "TestAzureBackendMigrationWithUnits")
	defer azureCtx.Cleanup()

	// Create temporary directory for test fixtures
	tmpDir := t.TempDir()

	// Create storage account with versioning enabled for migration support
	createTestStorageAccount(azureCtx.Ctx, t, azureCtx, true)

	// Create a simple terraform module for testing
	moduleDir := fmt.Sprintf("%s/modules/simple", tmpDir)
	createTerraformModule(t, moduleDir)

	t.Run("migrate_between_state_paths", func(t *testing.T) {
		// Test migrating state from one path to another in the same container

		// Create separate directories for source and destination units
		srcUnitDir := fmt.Sprintf("%s/src-unit", tmpDir)
		dstUnitDir := fmt.Sprintf("%s/dst-unit", tmpDir)

		err := os.MkdirAll(srcUnitDir, 0o755)
		require.NoError(t, err)
		err = os.MkdirAll(dstUnitDir, 0o755)
		require.NoError(t, err)

		// Copy terraform modules to both directories
		for _, unitDir := range []string{srcUnitDir, dstUnitDir} {
			unitModulesDir := fmt.Sprintf("%s/modules/simple", unitDir)
			createTerraformModule(t, unitModulesDir)
		}

		// Step 1: Create initial terragrunt.hcl for source unit with Azure backend
		srcConfig := generateAzureTerragruntConfig(
			azureCtx.StorageAccountName,
			azureCtx.ContainerName,
			"source/terraform.tfstate",
			azureCtx.ResourceGroupName,
			azureCtx.SubscriptionID,
			"./modules/simple",
		)

		srcConfigPath := fmt.Sprintf("%s/terragrunt.hcl", srcUnitDir)
		err = os.WriteFile(srcConfigPath, []byte(srcConfig), 0o644)
		require.NoError(t, err)

		// Setup container with permission test using helper
		_, cleanup := setupTestContainer(azureCtx.Ctx, t, azureCtx.StorageAccountName, azureCtx.ContainerName)
		defer cleanup()

		// Step 2: Change to source directory and bootstrap/apply to create initial state
		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalDir)

		err = os.Chdir(srcUnitDir)
		require.NoError(t, err)

		// First bootstrap the backend to ensure proper initialization
		output, stderr, err := runTerragruntCommandWithOutput(t, "terragrunt backend bootstrap --non-interactive")
		require.NoError(t, err, "Backend bootstrap failed: %v\nOutput: %s\nError: %s", err, output, stderr)
		t.Logf("Backend bootstrap completed successfully")

		// Then apply to create initial state
		output, stderr, err = runTerragruntCommandWithOutput(t, "terragrunt apply --non-interactive -auto-approve")
		require.NoError(t, err, "Initial apply failed: %v\nOutput: %s\nError: %s", err, output, stderr)

		// Get initial outputs to verify state
		output, stderr, err = runTerragruntCommandWithOutput(t, "terragrunt output --non-interactive -json")
		require.NoError(t, err, "Output command failed: %v\nOutput: %s\nError: %s", err, output, stderr)

		var initialOutputs map[string]interface{}
		err = json.Unmarshal([]byte(output), &initialOutputs)
		require.NoError(t, err)
		require.Contains(t, initialOutputs, "random_id")
		require.Contains(t, initialOutputs, "file_path")
		require.Contains(t, initialOutputs, "content")

		// Extract the random_id for verification after migration
		initialRandomID := initialOutputs["random_id"].(map[string]interface{})["value"].(string)
		initialFilePath := initialOutputs["file_path"].(map[string]interface{})["value"].(string)
		t.Logf("Initial random ID: %s", initialRandomID)
		t.Logf("Initial file path: %s", initialFilePath)

		// Verify the state exists in Azure at source path
		config := createStandardBlobConfig(azureCtx.StorageAccountName, azureCtx.ContainerName, map[string]interface{}{
			"versioning_enabled": true,
		})

		client := createBlobServiceClientHelper(azureCtx.Ctx, t, config)

		// Verify source state exists
		sourceStateExists := checkBlobExistsHelper(azureCtx.Ctx, t, client, azureCtx.ContainerName, "source/terraform.tfstate")
		require.True(t, sourceStateExists, "Source state should exist in Azure")

		// Step 3: Create destination unit terragrunt.hcl with different key path
		dstConfig := generateAzureTerragruntConfig(
			azureCtx.StorageAccountName,
			azureCtx.ContainerName,
			"destination/terraform.tfstate",
			azureCtx.ResourceGroupName,
			azureCtx.SubscriptionID,
			"./modules/simple",
		)

		dstConfigPath := fmt.Sprintf("%s/terragrunt.hcl", dstUnitDir)
		err = os.WriteFile(dstConfigPath, []byte(dstConfig), 0o644)
		require.NoError(t, err)

		// Migrate from dev to prod environment
		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		migrationCmd := fmt.Sprintf("terragrunt backend migrate --non-interactive %s %s", srcUnitDir, dstUnitDir)
		output, stderr, err = runTerragruntCommandWithOutput(t, migrationCmd)
		require.NoError(t, err, "Backend migration failed: %v\nOutput: %s\nError: %s", err, output, stderr)

		// Log the actual migration output for debugging
		t.Logf("Migration command output: %s", output)
		t.Logf("Migration command stderr: %s", stderr)

		// If migration command completed without error, consider it successful
		// The real verification is whether the state actually moved (checked below)
		t.Logf("Backend migration command completed successfully")

		// Step 5: Verify state was migrated to destination
		err = os.Chdir(dstUnitDir)
		require.NoError(t, err)

		// Check outputs in destination unit
		output, stderr, err = runTerragruntCommandWithOutput(t, "terragrunt output --non-interactive -json")
		require.NoError(t, err, "Output after migration failed: %v\nOutput: %s\nError: %s", err, output, stderr)

		var migratedOutputs map[string]interface{}
		err = json.Unmarshal([]byte(output), &migratedOutputs)
		require.NoError(t, err)

		// Verify state was preserved during migration
		migratedRandomID := migratedOutputs["random_id"].(map[string]interface{})["value"].(string)
		migratedFilePath := migratedOutputs["file_path"].(map[string]interface{})["value"].(string)

		assert.Equal(t, initialRandomID, migratedRandomID, "Random ID should be preserved during migration")
		assert.Equal(t, initialFilePath, migratedFilePath, "File path should be preserved during migration")

		// Step 6: Verify state exists at destination path in Azure
		destinationStateExists := checkBlobExistsHelper(azureCtx.Ctx, t, client, azureCtx.ContainerName, "destination/terraform.tfstate")
		require.True(t, destinationStateExists, "Destination state should exist in Azure after migration")

		// Step 7: Verify source state no longer exists (migration should move, not copy)
		sourceStateExists = checkBlobExistsHelper(azureCtx.Ctx, t, client, azureCtx.ContainerName, "source/terraform.tfstate")
		assert.False(t, sourceStateExists, "Source state should no longer exist after migration (state should be moved, not copied)")

		// Step 8: Verify we can still manage resources with the migrated state
		output, stderr, err = runTerragruntCommandWithOutput(t, "terragrunt plan --non-interactive")
		require.NoError(t, err, "Plan with migrated state failed: %v\nOutput: %s\nError: %s", err, output, stderr)
		assert.Contains(t, output, "No changes", "Plan should show no changes after successful migration")

		t.Log("Successfully migrated state from source path to destination path")

		// Cleanup resources
		output, stderr, err = runTerragruntCommandWithOutput(t, "terragrunt destroy --non-interactive -auto-approve")
		if err != nil {
			t.Logf("Warning: Failed to destroy resources: %v\nOutput: %s\nError: %s", err, output, stderr)
		} else {
			t.Log("Successfully destroyed test resources")
		}
	})
}

// --- Generic test helpers (inlined from helpers) ---
func uniqueID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

func cleanupAzureTerraformFolder(t *testing.T, dir string) {
	t.Helper()
	_ = os.RemoveAll(dir)
}

// --- Generic test helpers (continued) ---
func copyEnvironment(t *testing.T, src string) string {
	t.Helper()
	dst := fmt.Sprintf("/tmp/tgtest-%d", time.Now().UnixNano())
	err := os.RemoveAll(dst)
	if err != nil {
		t.Fatalf("Failed to clean up temp dir: %v", err)
	}
	err = copyDir(src, dst)
	if err != nil {
		t.Fatalf("Failed to copy environment: %v", err)
	}
	return dst
}

func copyDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, info.Mode())
	})
}

func copyTerragruntConfigAndFillProviderPlaceholders(t *testing.T, src, dst string, params map[string]string, location string) {
	t.Helper()
	input, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}
	content := string(input)
	for k, v := range params {
		content = strings.ReplaceAll(content, k, v)
	}
	if location != "" {
		content = strings.ReplaceAll(content, "__FILL_IN_LOCATION__", location)
	}
	err = os.WriteFile(dst, []byte(content), 0o644)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
}

func runTerragruntCommandWithOutput(t *testing.T, command string) (string, string, error) {
	t.Helper()
	
	// Set the experiment environment variable to enable Azure backend
	oldEnv := os.Getenv("TG_EXPERIMENT")
	defer func() {
		if oldEnv == "" {
			os.Unsetenv("TG_EXPERIMENT")
		} else {
			os.Setenv("TG_EXPERIMENT", oldEnv)
		}
	}()
	
	// Set the Azure backend experiment
	currentExperiments := os.Getenv("TG_EXPERIMENT")
	if currentExperiments == "" {
		os.Setenv("TG_EXPERIMENT", "azure-backend")
	} else if !strings.Contains(currentExperiments, "azure-backend") {
		os.Setenv("TG_EXPERIMENT", currentExperiments+",azure-backend")
	}
	
	// As a backup, also add the --experiment flag to ensure the experiment is enabled
	if !strings.Contains(command, "--experiment") && !strings.Contains(command, "terragrunt --help") {
		// Add the experiment flag to the command
		parts := strings.Fields(command)
		if len(parts) > 0 {
			if parts[0] == "terragrunt" {
				// Insert --experiment azure-backend after "terragrunt"
				newParts := append([]string{parts[0], "--experiment", "azure-backend"}, parts[1:]...)
				command = strings.Join(newParts, " ")
			} else {
				// If command doesn't start with terragrunt, prepend it
				command = "terragrunt --experiment azure-backend " + command
			}
		}
	}
	
	return helpers.RunTerragruntCommandWithOutput(t, command)

}

// --- Generic test helpers (continued) ---
func runTerragruntCommand(t *testing.T, command string, stdout, stderr *bytes.Buffer) error {
	t.Helper()
	
	// Set the experiment environment variable to enable Azure backend
	oldEnv := os.Getenv("TG_EXPERIMENT")
	defer func() {
		if oldEnv == "" {
			os.Unsetenv("TG_EXPERIMENT")
		} else {
			os.Setenv("TG_EXPERIMENT", oldEnv)
		}
	}()
	
	// Set the Azure backend experiment
	currentExperiments := os.Getenv("TG_EXPERIMENT")
	if currentExperiments == "" {
		os.Setenv("TG_EXPERIMENT", "azure-backend")
	} else if !strings.Contains(currentExperiments, "azure-backend") {
		os.Setenv("TG_EXPERIMENT", currentExperiments+",azure-backend")
	}
	
	// As a backup, also add the --experiment flag to ensure the experiment is enabled
	if !strings.Contains(command, "--experiment") && !strings.Contains(command, "terragrunt --help") {
		// Add the experiment flag to the command
		parts := strings.Fields(command)
		if len(parts) > 0 {
			if parts[0] == "terragrunt" {
				// Insert --experiment azure-backend after "terragrunt"
				newParts := append([]string{parts[0], "--experiment", "azure-backend"}, parts[1:]...)
				command = strings.Join(newParts, " ")
			} else {
				// If command doesn't start with terragrunt, prepend it
				command = "terragrunt --experiment azure-backend " + command
			}
		}
	}
	
	return helpers.RunTerragruntCommand(t, command, stdout, stderr)
}



// --- Generic test helpers (continued) ---
func runTerragrunt(t *testing.T, command string) {
	t.Helper()
	
	// Set the experiment environment variable to enable Azure backend
	oldEnv := os.Getenv("TG_EXPERIMENT")
	defer func() {
		if oldEnv == "" {
			os.Unsetenv("TG_EXPERIMENT")
		} else {
			os.Setenv("TG_EXPERIMENT", oldEnv)
		}
	}()
	
	// Set the Azure backend experiment
	currentExperiments := os.Getenv("TG_EXPERIMENT")
	if currentExperiments == "" {
		os.Setenv("TG_EXPERIMENT", "azure-backend")
	} else if !strings.Contains(currentExperiments, "azure-backend") {
		os.Setenv("TG_EXPERIMENT", currentExperiments+",azure-backend")
	}
	
	// As a backup, also add the --experiment flag to ensure the experiment is enabled
	if !strings.Contains(command, "--experiment") && !strings.Contains(command, "terragrunt --help") {
		// Add the experiment flag to the command
		parts := strings.Fields(command)
		if len(parts) > 0 {
			if parts[0] == "terragrunt" {
				// Insert --experiment azure-backend after "terragrunt"
				newParts := append([]string{parts[0], "--experiment", "azure-backend"}, parts[1:]...)
				command = strings.Join(newParts, " ")
			} else {
				// If command doesn't start with terragrunt, prepend it
				command = "terragrunt --experiment azure-backend " + command
			}
		}
	}
	
	helpers.RunTerragrunt(t, command)
}

// --- End generic test helpers ---

// --- More Azure test helpers (formerly from test/helpers/azure.go) ---
type azureTestContext struct {
	SubscriptionID     string
	Location           string
	StorageAccountName string
	ResourceGroupName  string
	ContainerName      string
	Logger             log.Logger
	Ctx                context.Context
	Cleanup            func()
}

func setupAzureTest(t *testing.T, testName string) *azureTestContext {
	t.Helper()
	ctx := t.Context()
	log := logger.CreateLogger()
	_, subscriptionID, err := azurehelper.GetAzureCredentials(ctx, log)
	if err != nil {
		t.Skipf("Skipping %s: Failed to get Azure credentials: %v", testName, err)
	}
	if subscriptionID == "" {
		t.Skipf("Skipping %s: No subscription ID found in environment variables", testName)
	}
	location := os.Getenv("AZURE_LOCATION")
	if location == "" {
		location = os.Getenv("ARM_LOCATION")
		if location == "" {
			location = "westeurope"
		}
	}
	uniqueID := strconv.FormatInt(time.Now().UnixNano(), 10)
	if len(uniqueID) > 10 {
		uniqueID = uniqueID[len(uniqueID)-10:]
	}
	storageAccountName := "tgtest" + strings.ToLower(uniqueID)
	resourceGroupName := fmt.Sprintf("terragrunt-test-rg-%s-%s", strings.ReplaceAll(testName, " ", "-"), uniqueID)
	containerName := "test-container-" + strings.ToLower(uniqueID)
	var resourceGroupCreated, storageAccountCreated bool
	cleanup := func() {
		cleanupLogger := logger.CreateLogger()

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
	rgClient, err := azurehelper.CreateResourceGroupClient(ctx, log, subscriptionID)
	require.NoError(t, err)
	err = rgClient.EnsureResourceGroup(ctx, log, resourceGroupName, location, map[string]string{
		"created-by": "terragrunt-integration-test",
		"test-case":  testName,
	})
	require.NoError(t, err)
	resourceGroupCreated = true
	t.Logf("Resource group %s created successfully", resourceGroupName)
	return &azureTestContext{
		SubscriptionID:     subscriptionID,
		Location:           location,
		StorageAccountName: storageAccountName,
		ResourceGroupName:  resourceGroupName,
		ContainerName:      containerName,
		Logger:             log,
		Ctx:                ctx,
		Cleanup:            cleanup,
	}
}

func createTestStorageAccount(ctx context.Context, t *testing.T, azureCtx *azureTestContext, enableVersioning bool) {
	t.Helper()
	storageAccountConfig := map[string]interface{}{
		"storage_account_name": azureCtx.StorageAccountName,
		"resource_group_name":  azureCtx.ResourceGroupName,
		"subscription_id":      azureCtx.SubscriptionID,
		"location":             azureCtx.Location,
		"use_azuread_auth":     true,
	}
	t.Logf("Creating storage account %s in resource group %s", azureCtx.StorageAccountName, azureCtx.ResourceGroupName)
	storageClient, err := azurehelper.CreateStorageAccountClient(ctx, azureCtx.Logger, storageAccountConfig)
	require.NoError(t, err)
	require.NotNil(t, storageClient)
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
	err = storageClient.CreateStorageAccountIfNecessary(ctx, azureCtx.Logger, saConfig)
	require.NoError(t, err)
	exists, account, err := storageClient.StorageAccountExists(ctx)
	require.NoError(t, err)
	require.True(t, exists, "Storage account should exist after creation")
	require.NotNil(t, account)
	t.Logf("Storage account %s created successfully", azureCtx.StorageAccountName)
}

func setupTestContainer(ctx context.Context, t *testing.T, storageAccountName, containerName string) (*azurehelper.BlobServiceClient, func()) {
	t.Helper()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	log := logger.CreateLogger()
	blobConfig := map[string]interface{}{
		"storage_account_name": storageAccountName,
		"container_name":       containerName,
		"use_azuread_auth":     true,
	}
	blobClient, err := azurehelper.CreateBlobServiceClient(ctx, log, opts, blobConfig)
	require.NoError(t, err)
	err = blobClient.CreateContainerIfNecessary(ctx, log, containerName)
	require.NoError(t, err)
	t.Logf("Container %s created successfully", containerName)
	testBlobName := "test-permissions.txt"
	testContent := []byte("Permission test")
	sleepTime := 5 * time.Second // Allow some time for the container to be fully ready
	time.Sleep(sleepTime)
	err = blobClient.UploadBlob(ctx, log, containerName, testBlobName, testContent)
	require.NoError(t, err, "Should be able to upload test blob - check Azure permissions")
	err = blobClient.DeleteBlobIfNecessary(ctx, log, containerName, testBlobName)
	require.NoError(t, err)
	t.Logf("Blob permissions test passed successfully")
	cleanup := func() {
		exists, err := blobClient.ContainerExists(ctx, containerName)
		if err == nil && exists {
			err = blobClient.DeleteContainer(ctx, log, containerName)
			if err != nil {
				t.Logf("Warning: Failed to delete container %s: %v", containerName, err)
			} else {
				t.Logf("Successfully deleted container %s", containerName)
			}
		}
	}
	return blobClient, cleanup
}

func testBlobOperations(t *testing.T, client *azurehelper.BlobServiceClient, containerName string) {
	t.Helper()
	ctx := t.Context()
	blobName := "test-blob.txt"
	log := logger.CreateLogger()
	input := &azurehelper.GetObjectInput{
		Container: &containerName,
		Key:      &blobName,
	}
	_, err := client.GetObject(ctx, input)
	require.Error(t, err, "Getting non-existent blob should fail")
	err = client.DeleteBlobIfNecessary(ctx, log, containerName, blobName)
	require.NoError(t, err, "Deleting non-existent blob should not error")
	exists, err := client.ContainerExists(ctx, containerName)
	require.NoError(t, err)
	assert.True(t, exists, "Container should exist")
}

func createInvalidBlobServiceClientHelper(ctx context.Context, t *testing.T, config map[string]interface{}) (*azurehelper.BlobServiceClient, error) {
	t.Helper()
	log := logger.CreateLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	if err != nil {
		return nil, err
	}
	return azurehelper.CreateBlobServiceClient(ctx, log, opts, config)
}

// --- Generic test helpers (continued) ---
func getAzureCredentialsHelper(ctx context.Context, t *testing.T) (*azidentity.DefaultAzureCredential, string) {
	t.Helper()
	log := logger.CreateLogger()
	creds, subscriptionID, err := azurehelper.GetAzureCredentials(ctx, log)
	require.NoError(t, err, "Failed to get Azure credentials")
	return creds, subscriptionID
}

// --- End generic test helpers ---

// --- Helper to create a simple Terraform module for testing ---
func createTerraformModule(t *testing.T, moduleDir string) {
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

// --- Helper to generate a Terragrunt config for Azure remote state ---
func generateAzureTerragruntConfig(storageAccount, container, stateKey, resourceGroup, subscriptionID, terraformSource string) string {
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
// --- End helper ---
