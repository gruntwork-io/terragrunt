// Package azurerm provides tests for custom error types used by the Azure storage backend
package azurerm_test

import (
	"errors"
	"testing"

	azurerm "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMissingRequiredAzureRemoteStateConfig tests the MissingRequiredAzureRemoteStateConfig error type
func TestMissingRequiredAzureRemoteStateConfig(t *testing.T) {
	t.Parallel()

	t.Run("Error method returns correct format", func(t *testing.T) {
		t.Parallel()

		err := azurerm.MissingRequiredAzureRemoteStateConfig("storage_account_name")
		expectedMsg := "missing required Azure remote state configuration storage_account_name"
		assert.Equal(t, expectedMsg, err.Error())
	})

	t.Run("Different config names", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			configName  string
			expectedMsg string
		}{
			{
				configName:  "container_name",
				expectedMsg: "missing required Azure remote state configuration container_name",
			},
			{
				configName:  "key",
				expectedMsg: "missing required Azure remote state configuration key",
			},
			{
				configName:  "subscription_id",
				expectedMsg: "missing required Azure remote state configuration subscription_id",
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.configName, func(t *testing.T) {
				t.Parallel()

				err := azurerm.MissingRequiredAzureRemoteStateConfig(tc.configName)
				assert.Equal(t, tc.expectedMsg, err.Error())
			})
		}
	})

	t.Run("Can be identified using errors.As", func(t *testing.T) {
		t.Parallel()

		err := azurerm.MissingRequiredAzureRemoteStateConfig("test_config")

		var missingConfigError azurerm.MissingRequiredAzureRemoteStateConfig
		require.ErrorAs(t, err, &missingConfigError)
		assert.Equal(t, "test_config", string(missingConfigError))
	})
}

// TestMaxRetriesWaitingForContainerExceeded tests the MaxRetriesWaitingForContainerExceeded error type
func TestMaxRetriesWaitingForContainerExceeded(t *testing.T) {
	t.Parallel()

	t.Run("Error method returns correct format", func(t *testing.T) {
		t.Parallel()

		err := azurerm.MaxRetriesWaitingForContainerExceeded("test-container")
		expectedMsg := "Exceeded max retries waiting for Azure Storage container test-container"
		assert.Equal(t, expectedMsg, err.Error())
	})

	t.Run("Can be identified using errors.As", func(t *testing.T) {
		t.Parallel()

		err := azurerm.MaxRetriesWaitingForContainerExceeded("my-container")

		var maxRetriesError azurerm.MaxRetriesWaitingForContainerExceeded
		require.ErrorAs(t, err, &maxRetriesError)
		assert.Equal(t, "my-container", string(maxRetriesError))
	})
}

// TestContainerDoesNotExist tests the ContainerDoesNotExist error type
func TestContainerDoesNotExist(t *testing.T) {
	t.Parallel()

	t.Run("Error method returns correct format", func(t *testing.T) {
		t.Parallel()

		underlyingErr := errors.New("Azure API error: container not found")
		err := azurerm.ContainerDoesNotExist{
			Underlying:    underlyingErr,
			ContainerName: "test-container",
		}

		expectedMsg := "Container test-container does not exist. Underlying error: Azure API error: container not found"
		assert.Equal(t, expectedMsg, err.Error())
	})

	t.Run("Unwrap returns underlying error", func(t *testing.T) {
		t.Parallel()

		underlyingErr := errors.New("simulated Azure error")
		err := azurerm.ContainerDoesNotExist{
			Underlying:    underlyingErr,
			ContainerName: "test-container",
		}

		unwrappedErr := err.Unwrap()
		assert.Equal(t, underlyingErr, unwrappedErr)
	})

	t.Run("Supports errors.Is", func(t *testing.T) {
		t.Parallel()

		underlyingErr := errors.New("specific Azure error")
		err := azurerm.ContainerDoesNotExist{
			Underlying:    underlyingErr,
			ContainerName: "test-container",
		}

		assert.ErrorIs(t, err, underlyingErr)
	})

	t.Run("Can be identified using errors.As", func(t *testing.T) {
		t.Parallel()

		underlyingErr := errors.New("test error")
		err := azurerm.ContainerDoesNotExist{
			Underlying:    underlyingErr,
			ContainerName: "my-container",
		}

		var containerError azurerm.ContainerDoesNotExist
		require.ErrorAs(t, err, &containerError)
		assert.Equal(t, "my-container", containerError.ContainerName)
		assert.Equal(t, underlyingErr, containerError.Underlying)
	})
}

// TestMissingSubscriptionIDError tests the MissingSubscriptionIDError error type
func TestMissingSubscriptionIDError(t *testing.T) {
	t.Parallel()

	t.Run("Error method returns correct format", func(t *testing.T) {
		t.Parallel()

		err := azurerm.MissingSubscriptionIDError{}
		expectedMsg := "subscription_id is required for storage account creation"
		assert.Equal(t, expectedMsg, err.Error())
	})

	t.Run("Can be identified using errors.As", func(t *testing.T) {
		t.Parallel()

		err := azurerm.MissingSubscriptionIDError{}

		var missingSubError azurerm.MissingSubscriptionIDError
		require.ErrorAs(t, err, &missingSubError)
	})
}

// TestMissingLocationError tests the MissingLocationError error type
func TestMissingLocationError(t *testing.T) {
	t.Parallel()

	t.Run("Error method returns correct format", func(t *testing.T) {
		t.Parallel()

		err := azurerm.MissingLocationError{}
		expectedMsg := "location is required for storage account creation"
		assert.Equal(t, expectedMsg, err.Error())
	})

	t.Run("Can be identified using errors.As", func(t *testing.T) {
		t.Parallel()

		err := azurerm.MissingLocationError{}

		var missingLocError azurerm.MissingLocationError
		require.ErrorAs(t, err, &missingLocError)
	})
}

// TestNoValidAuthMethodError tests the NoValidAuthMethodError error type
func TestNoValidAuthMethodError(t *testing.T) {
	t.Parallel()

	t.Run("Error method returns correct format", func(t *testing.T) {
		t.Parallel()

		err := azurerm.NoValidAuthMethodError{}
		expectedMsg := "no valid authentication method found: Azure AD auth is recommended. Alternatively, provide one of: MSI, service principal credentials, or SAS token"
		assert.Equal(t, expectedMsg, err.Error())
	})

	t.Run("Can be identified using errors.As", func(t *testing.T) {
		t.Parallel()

		err := azurerm.NoValidAuthMethodError{}

		var noAuthError azurerm.NoValidAuthMethodError
		require.ErrorAs(t, err, &noAuthError)
	})
}

// TestStorageAccountCreationError tests the StorageAccountCreationError error type
func TestStorageAccountCreationError(t *testing.T) {
	t.Parallel()

	t.Run("Error method returns correct format", func(t *testing.T) {
		t.Parallel()

		underlyingErr := errors.New("Azure resource creation failed")
		err := azurerm.StorageAccountCreationError{
			Underlying:         underlyingErr,
			StorageAccountName: "mystorageaccount",
		}

		expectedMsg := "error with storage account mystorageaccount: Azure resource creation failed"
		assert.Equal(t, expectedMsg, err.Error())
	})

	t.Run("Unwrap returns underlying error", func(t *testing.T) {
		t.Parallel()

		underlyingErr := errors.New("simulated creation error")
		err := azurerm.StorageAccountCreationError{
			Underlying:         underlyingErr,
			StorageAccountName: "testaccount",
		}

		unwrappedErr := err.Unwrap()
		assert.Equal(t, underlyingErr, unwrappedErr)
	})

	t.Run("Supports errors.Is", func(t *testing.T) {
		t.Parallel()

		underlyingErr := errors.New("specific creation error")
		err := azurerm.StorageAccountCreationError{
			Underlying:         underlyingErr,
			StorageAccountName: "testaccount",
		}

		assert.ErrorIs(t, err, underlyingErr)
	})

	t.Run("Can be identified using errors.As", func(t *testing.T) {
		t.Parallel()

		underlyingErr := errors.New("test creation error")
		err := azurerm.StorageAccountCreationError{
			Underlying:         underlyingErr,
			StorageAccountName: "myaccount",
		}

		var storageError azurerm.StorageAccountCreationError
		require.ErrorAs(t, err, &storageError)
		assert.Equal(t, "myaccount", storageError.StorageAccountName)
		assert.Equal(t, underlyingErr, storageError.Underlying)
	})

	t.Run("Handles nil underlying error", func(t *testing.T) {
		t.Parallel()

		err := azurerm.StorageAccountCreationError{
			Underlying:         nil,
			StorageAccountName: "testaccount",
		}

		expectedMsg := "error with storage account testaccount: <nil>"
		assert.Equal(t, expectedMsg, err.Error())
		assert.NoError(t, err.Unwrap())
	})
}

// TestContainerCreationErrorType tests the ContainerCreationError error type
func TestContainerCreationErrorType(t *testing.T) {
	t.Parallel()

	t.Run("Error method returns correct format", func(t *testing.T) {
		t.Parallel()

		underlyingErr := errors.New("Azure container operation failed")
		err := azurerm.ContainerCreationError{
			Underlying:    underlyingErr,
			ContainerName: "test-container",
		}

		expectedMsg := "error with container test-container: Azure container operation failed"
		assert.Equal(t, expectedMsg, err.Error())
	})

	t.Run("Unwrap returns underlying error", func(t *testing.T) {
		t.Parallel()

		underlyingErr := errors.New("simulated container error")
		err := azurerm.ContainerCreationError{
			Underlying:    underlyingErr,
			ContainerName: "my-container",
		}

		unwrappedErr := err.Unwrap()
		assert.Equal(t, underlyingErr, unwrappedErr)
	})

	t.Run("Supports errors.Is", func(t *testing.T) {
		t.Parallel()

		underlyingErr := errors.New("specific container error")
		err := azurerm.ContainerCreationError{
			Underlying:    underlyingErr,
			ContainerName: "test-container",
		}

		assert.ErrorIs(t, err, underlyingErr)
	})

	t.Run("Can be identified using errors.As", func(t *testing.T) {
		t.Parallel()

		underlyingErr := errors.New("test container error")
		err := azurerm.ContainerCreationError{
			Underlying:    underlyingErr,
			ContainerName: "my-container",
		}

		var containerError azurerm.ContainerCreationError
		require.ErrorAs(t, err, &containerError)
		assert.Equal(t, "my-container", containerError.ContainerName)
		assert.Equal(t, underlyingErr, containerError.Underlying)
	})

	t.Run("Handles nil underlying error", func(t *testing.T) {
		t.Parallel()

		err := azurerm.ContainerCreationError{
			Underlying:    nil,
			ContainerName: "test-container",
		}

		expectedMsg := "error with container test-container: <nil>"
		assert.Equal(t, expectedMsg, err.Error())
		assert.NoError(t, err.Unwrap())
	})
}

// TestAuthenticationError tests the AuthenticationError error type
func TestAuthenticationError(t *testing.T) {
	t.Parallel()

	t.Run("Error method returns correct format", func(t *testing.T) {
		t.Parallel()

		underlyingErr := errors.New("invalid credentials provided")
		err := azurerm.AuthenticationError{
			Underlying: underlyingErr,
			AuthMethod: "Azure AD",
		}

		expectedMsg := "Azure authentication failed using Azure AD: invalid credentials provided"
		assert.Equal(t, expectedMsg, err.Error())
	})

	t.Run("Different auth methods", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			authMethod  string
			expectedMsg string
		}{
			{
				authMethod:  "MSI",
				expectedMsg: "Azure authentication failed using MSI: auth error",
			},
			{
				authMethod:  "Service Principal",
				expectedMsg: "Azure authentication failed using Service Principal: auth error",
			},
			{
				authMethod:  "SAS Token",
				expectedMsg: "Azure authentication failed using SAS Token: auth error",
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.authMethod, func(t *testing.T) {
				t.Parallel()

				underlyingErr := errors.New("auth error")
				err := azurerm.AuthenticationError{
					Underlying: underlyingErr,
					AuthMethod: tc.authMethod,
				}

				assert.Equal(t, tc.expectedMsg, err.Error())
			})
		}
	})

	t.Run("Unwrap returns underlying error", func(t *testing.T) {
		t.Parallel()

		underlyingErr := errors.New("simulated auth error")
		err := azurerm.AuthenticationError{
			Underlying: underlyingErr,
			AuthMethod: "Azure AD",
		}

		unwrappedErr := err.Unwrap()
		assert.Equal(t, underlyingErr, unwrappedErr)
	})

	t.Run("Supports errors.Is", func(t *testing.T) {
		t.Parallel()

		underlyingErr := errors.New("specific auth error")
		err := azurerm.AuthenticationError{
			Underlying: underlyingErr,
			AuthMethod: "MSI",
		}

		assert.ErrorIs(t, err, underlyingErr)
	})

	t.Run("Can be identified using errors.As", func(t *testing.T) {
		t.Parallel()

		underlyingErr := errors.New("test auth error")
		err := azurerm.AuthenticationError{
			Underlying: underlyingErr,
			AuthMethod: "Service Principal",
		}

		var authError azurerm.AuthenticationError
		require.ErrorAs(t, err, &authError)
		assert.Equal(t, "Service Principal", authError.AuthMethod)
		assert.Equal(t, underlyingErr, authError.Underlying)
	})

	t.Run("Handles nil underlying error", func(t *testing.T) {
		t.Parallel()

		err := azurerm.AuthenticationError{
			Underlying: nil,
			AuthMethod: "Azure AD",
		}

		expectedMsg := "Azure authentication failed using Azure AD: <nil>"
		assert.Equal(t, expectedMsg, err.Error())
		assert.NoError(t, err.Unwrap())
	})
}

// TestContainerValidationError tests the ContainerValidationError error type
func TestContainerValidationError(t *testing.T) {
	t.Parallel()

	t.Run("Error method returns correct format", func(t *testing.T) {
		t.Parallel()

		err := azurerm.ContainerValidationError{
			ValidationIssue: "container name must be lowercase",
		}

		expectedMsg := "container name must be lowercase"
		assert.Equal(t, expectedMsg, err.Error())
	})

	t.Run("Different validation issues", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			validationIssue string
			expectedMsg     string
		}{
			{
				validationIssue: "container name too short",
				expectedMsg:     "container name too short",
			},
			{
				validationIssue: "container name contains invalid characters",
				expectedMsg:     "container name contains invalid characters",
			},
			{
				validationIssue: "container name cannot start with a hyphen",
				expectedMsg:     "container name cannot start with a hyphen",
			},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.validationIssue, func(t *testing.T) {
				t.Parallel()

				err := azurerm.ContainerValidationError{
					ValidationIssue: tc.validationIssue,
				}

				assert.Equal(t, tc.expectedMsg, err.Error())
			})
		}
	})

	t.Run("Can be identified using errors.As", func(t *testing.T) {
		t.Parallel()

		err := azurerm.ContainerValidationError{
			ValidationIssue: "test validation issue",
		}

		var validationError azurerm.ContainerValidationError
		require.ErrorAs(t, err, &validationError)
		assert.Equal(t, "test validation issue", validationError.ValidationIssue)
	})
}

// TestMissingResourceGroupError tests the MissingResourceGroupError error type
func TestMissingResourceGroupError(t *testing.T) {
	t.Parallel()

	t.Run("Error method returns correct format", func(t *testing.T) {
		t.Parallel()

		err := azurerm.MissingResourceGroupError{}
		expectedMsg := "resource_group_name is required to delete a storage account"
		assert.Equal(t, expectedMsg, err.Error())
	})

	t.Run("Can be identified using errors.As", func(t *testing.T) {
		t.Parallel()

		err := azurerm.MissingResourceGroupError{}

		var missingRGError azurerm.MissingResourceGroupError
		require.ErrorAs(t, err, &missingRGError)
	})
}

// TestErrorChaining tests that multiple levels of error wrapping work correctly
func TestErrorChaining(t *testing.T) {
	t.Parallel()

	t.Run("Multi-level error wrapping", func(t *testing.T) {
		t.Parallel()

		// Create a chain: base error -> AuthenticationError -> StorageAccountCreationError
		baseErr := errors.New("network connection failed")
		authErr := azurerm.AuthenticationError{
			Underlying: baseErr,
			AuthMethod: "Azure AD",
		}
		storageErr := azurerm.StorageAccountCreationError{
			Underlying:         authErr,
			StorageAccountName: "testaccount",
		}

		// Test that errors.Is works through the chain
		require.ErrorIs(t, storageErr, baseErr)
		require.ErrorIs(t, storageErr, authErr)

		// Test that errors.As works through the chain
		var unwrappedAuthErr azurerm.AuthenticationError
		require.ErrorAs(t, storageErr, &unwrappedAuthErr)
		assert.Equal(t, "Azure AD", unwrappedAuthErr.AuthMethod)

		// Test direct unwrapping
		unwrapped := storageErr.Unwrap()
		assert.Equal(t, authErr, unwrapped)
	})
}
