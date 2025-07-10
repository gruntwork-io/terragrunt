package errors_test

import (
	"errors"
	"strings"
	"testing"

	azureerrors "github.com/gruntwork-io/terragrunt/internal/azure/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestErrorClass tests error classification constants
func TestErrorClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		class azureerrors.ErrorClass
	}{
		{"authentication", azureerrors.ErrorClassAuthentication},
		{"authorization", azureerrors.ErrorClassAuthorization},
		{"configuration", azureerrors.ErrorClassConfiguration},
		{"invalid_request", azureerrors.ErrorClassInvalidRequest},
		{"networking", azureerrors.ErrorClassNetworking},
		{"not_found", azureerrors.ErrorClassNotFound},
		{"permission", azureerrors.ErrorClassPermission},
		{"resource", azureerrors.ErrorClassResource},
		{"system", azureerrors.ErrorClassSystem},
		{"throttling", azureerrors.ErrorClassThrottling},
		{"transient", azureerrors.ErrorClassTransient},
		{"unknown", azureerrors.ErrorClassUnknown},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that class constants are defined and match expected values
			assert.Equal(t, tc.name, string(tc.class))
			assert.NotEmpty(t, tc.class)
		})
	}
}

// TestResourceType tests resource type constants
func TestResourceType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		resourceType azureerrors.ResourceType
	}{
		{"blob", azureerrors.ResourceTypeBlob},
		{"container", azureerrors.ResourceTypeContainer},
		{"resource_group", azureerrors.ResourceTypeResourceGroup},
		{"storage_account", azureerrors.ResourceTypeStorage},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that resource type constants are defined and match expected values
			assert.Equal(t, tc.name, string(tc.resourceType))
			assert.NotEmpty(t, tc.resourceType)
		})
	}
}

// TestAzureErrorCreation tests creating AzureError instances
func TestAzureErrorCreation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		azureErr *azureerrors.AzureError
	}{
		{
			name: "minimal error",
			azureErr: &azureerrors.AzureError{
				Message: "Test error message",
			},
		},
		{
			name: "complete error",
			azureErr: &azureerrors.AzureError{
				Message:        "Complete error message",
				Wrapped:        errors.New("underlying error"),
				Suggestion:     "Try checking your configuration",
				Classification: azureerrors.ErrorClassConfiguration,
				ResourceType:   azureerrors.ResourceTypeStorage,
				ResourceName:   "teststorageaccount",
				Operation:      "CreateStorageAccount",
			},
		},
		{
			name: "permission error",
			azureErr: &azureerrors.AzureError{
				Message:        "Access denied",
				Classification: azureerrors.ErrorClassPermission,
				ResourceType:   azureerrors.ResourceTypeBlob,
				ResourceName:   "test-blob.tfstate",
				Suggestion:     "Check your RBAC permissions",
			},
		},
		{
			name: "not found error",
			azureErr: &azureerrors.AzureError{
				Message:        "Resource not found",
				Classification: azureerrors.ErrorClassNotFound,
				ResourceType:   azureerrors.ResourceTypeContainer,
				ResourceName:   "tfstate-container",
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that error can be created and fields accessed
			require.NotNil(t, tc.azureErr)
			assert.NotEmpty(t, tc.azureErr.Message)

			// Test Error() method
			errStr := tc.azureErr.Error()
			assert.NotEmpty(t, errStr)
			assert.Contains(t, errStr, tc.azureErr.Message)

			// Test field access
			_ = tc.azureErr.Wrapped
			_ = tc.azureErr.Suggestion
			_ = tc.azureErr.Classification
			_ = tc.azureErr.ResourceType
			_ = tc.azureErr.ResourceName
			_ = tc.azureErr.Operation
		})
	}
}

// TestAzureErrorFormatting tests error message formatting
func TestAzureErrorFormatting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		azureErr       *azureerrors.AzureError
		expectContains []string
	}{
		{
			name: "simple message",
			azureErr: &azureerrors.AzureError{
				Message: "Simple error",
			},
			expectContains: []string{"Simple error"},
		},
		{
			name: "error with operation",
			azureErr: &azureerrors.AzureError{
				Message:   "Operation failed",
				Operation: "CreateBlob",
			},
			expectContains: []string{"Operation failed", "CreateBlob"},
		},
		{
			name: "error with resource info",
			azureErr: &azureerrors.AzureError{
				Message:      "Resource error",
				ResourceType: azureerrors.ResourceTypeBlob,
				ResourceName: "test-blob",
			},
			expectContains: []string{"Resource error", "blob", "test-blob"},
		},
		{
			name: "error with wrapped error",
			azureErr: &azureerrors.AzureError{
				Message: "Outer error",
				Wrapped: errors.New("inner error"),
			},
			expectContains: []string{"Outer error", "inner error"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			errStr := tc.azureErr.Error()

			for _, expected := range tc.expectContains {
				assert.Contains(t, strings.ToLower(errStr), strings.ToLower(expected),
					"Error string should contain '%s': %s", expected, errStr)
			}
		})
	}
}

// TestAzureErrorChaining tests error chaining and unwrapping
func TestAzureErrorChaining(t *testing.T) {
	t.Parallel()

	originalErr := errors.New("original error")
	azureErr := &azureerrors.AzureError{
		Message: "Azure error",
		Wrapped: originalErr,
	}

	// Test that error can be unwrapped
	unwrapped := errors.Unwrap(azureErr)
	if unwrapped != nil {
		assert.Equal(t, originalErr, unwrapped)
	}

	// Test error chain with errors.Is
	assert.True(t, errors.Is(azureErr, originalErr))
}

// TestAzureErrorClassification tests error classification logic
func TestAzureErrorClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		classification azureerrors.ErrorClass
		expectPattern  string
	}{
		{
			name:           "authentication error",
			classification: azureerrors.ErrorClassAuthentication,
			expectPattern:  "authentication",
		},
		{
			name:           "permission error",
			classification: azureerrors.ErrorClassPermission,
			expectPattern:  "permission",
		},
		{
			name:           "not found error",
			classification: azureerrors.ErrorClassNotFound,
			expectPattern:  "not_found",
		},
		{
			name:           "configuration error",
			classification: azureerrors.ErrorClassConfiguration,
			expectPattern:  "configuration",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := &azureerrors.AzureError{
				Message:        "Test error",
				Classification: tc.classification,
			}

			// Test that classification is preserved
			assert.Equal(t, tc.classification, err.Classification)
			assert.Equal(t, tc.expectPattern, string(err.Classification))
		})
	}
}

// TestAzureErrorSuggestions tests error suggestion handling
func TestAzureErrorSuggestions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		suggestion string
		valid      bool
	}{
		{
			name:       "empty suggestion",
			suggestion: "",
			valid:      true,
		},
		{
			name:       "helpful suggestion",
			suggestion: "Check your Azure credentials",
			valid:      true,
		},
		{
			name:       "detailed suggestion",
			suggestion: "Ensure that the storage account exists and you have the Storage Blob Data Contributor role",
			valid:      true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := &azureerrors.AzureError{
				Message:    "Test error",
				Suggestion: tc.suggestion,
			}

			// Test that suggestion is preserved
			assert.Equal(t, tc.suggestion, err.Suggestion)

			// Test that error string includes suggestion if not empty
			errStr := err.Error()
			if tc.suggestion != "" {
				assert.Contains(t, strings.ToLower(errStr), strings.ToLower(tc.suggestion))
			}
		})
	}
}

// TestAzureErrorResourceInformation tests resource information handling
func TestAzureErrorResourceInformation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		resourceType azureerrors.ResourceType
		resourceName string
	}{
		{
			name:         "blob resource",
			resourceType: azureerrors.ResourceTypeBlob,
			resourceName: "terraform.tfstate",
		},
		{
			name:         "container resource",
			resourceType: azureerrors.ResourceTypeContainer,
			resourceName: "tfstate-container",
		},
		{
			name:         "storage account resource",
			resourceType: azureerrors.ResourceTypeStorage,
			resourceName: "mystorageaccount",
		},
		{
			name:         "resource group resource",
			resourceType: azureerrors.ResourceTypeResourceGroup,
			resourceName: "my-resource-group",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := &azureerrors.AzureError{
				Message:      "Resource error",
				ResourceType: tc.resourceType,
				ResourceName: tc.resourceName,
			}

			// Test that resource information is preserved
			assert.Equal(t, tc.resourceType, err.ResourceType)
			assert.Equal(t, tc.resourceName, err.ResourceName)

			// Test that error string includes resource information
			errStr := err.Error()
			assert.Contains(t, strings.ToLower(errStr), strings.ToLower(string(tc.resourceType)))
			if tc.resourceName != "" {
				assert.Contains(t, strings.ToLower(errStr), strings.ToLower(tc.resourceName))
			}
		})
	}
}
