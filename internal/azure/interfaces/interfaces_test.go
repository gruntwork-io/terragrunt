package interfaces_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/azure/interfaces"
	"github.com/gruntwork-io/terragrunt/internal/azure/types"
	"github.com/stretchr/testify/assert"
)

// TestRoleAssignment tests the RoleAssignment struct
func TestRoleAssignment(t *testing.T) {
	t.Parallel()

	roleAssignment := interfaces.RoleAssignment{
		RoleName:    "Storage Blob Data Owner",
		PrincipalID: "12345678-1234-1234-1234-123456789012",
		Scope:       "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/account",
		Description: "Test role assignment",
	}

	assert.Equal(t, "Storage Blob Data Owner", roleAssignment.RoleName)
	assert.Equal(t, "12345678-1234-1234-1234-123456789012", roleAssignment.PrincipalID)
	assert.Equal(t, "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/account", roleAssignment.Scope)
	assert.Equal(t, "Test role assignment", roleAssignment.Description)
}

// TestPrincipal tests the Principal struct
func TestPrincipal(t *testing.T) {
	t.Parallel()

	principal := interfaces.Principal{
		ID:   "principal-id-123",
		Type: "ServicePrincipal",
	}

	assert.Equal(t, "principal-id-123", principal.ID)
	assert.Equal(t, "ServicePrincipal", principal.Type)
}

// TestAuthenticationConfig tests the AuthenticationConfig struct
func TestAuthenticationConfig(t *testing.T) {
	t.Parallel()

	config := interfaces.AuthenticationConfig{
		SubscriptionID:     "sub-12345",
		TenantID:           "tenant-67890",
		ClientID:           "client-abcde",
		ClientSecret:       "secret-fghij",
		UseManagedIdentity: false,
	}

	assert.Equal(t, "sub-12345", config.SubscriptionID)
	assert.Equal(t, "tenant-67890", config.TenantID)
	assert.Equal(t, "client-abcde", config.ClientID)
	assert.Equal(t, "secret-fghij", config.ClientSecret)
	assert.False(t, config.UseManagedIdentity)
}

// TestRBACConfig tests the RBACConfig struct
func TestRBACConfig(t *testing.T) {
	t.Parallel()

	config := interfaces.RBACConfig{
		MaxRetries:         5,
		RetryDelay:         3,
		PropagationTimeout: 60,
		EnableRetry:        true,
	}

	assert.Equal(t, 5, config.MaxRetries)
	assert.Equal(t, 3, config.RetryDelay)
	assert.Equal(t, 60, config.PropagationTimeout)
	assert.True(t, config.EnableRetry)
}

// TestErrorConstants tests that error constants are accessible
func TestErrorConstants(t *testing.T) {
	t.Parallel()

	// Test that error constants exist and are not nil
	assert.NotNil(t, interfaces.ErrNotImplemented)
	assert.NotNil(t, interfaces.ErrInvalidCredentials)

	// Test that error constants have meaningful messages
	assert.Contains(t, interfaces.ErrNotImplemented.Error(), "not implemented")
	assert.Contains(t, interfaces.ErrInvalidCredentials.Error(), "invalid credentials")

	// Test that they are different errors
	assert.NotEqual(t, interfaces.ErrNotImplemented, interfaces.ErrInvalidCredentials)
}

// TestConfigStructDefaults tests default values for config structs
func TestConfigStructDefaults(t *testing.T) {
	t.Parallel()

	t.Run("AuthenticationConfig defaults", func(t *testing.T) {
		config := interfaces.AuthenticationConfig{}

		assert.Equal(t, "", config.SubscriptionID)
		assert.Equal(t, "", config.TenantID)
		assert.Equal(t, "", config.ClientID)
		assert.Equal(t, "", config.ClientSecret)
		assert.False(t, config.UseManagedIdentity)
	})

	t.Run("RBACConfig defaults", func(t *testing.T) {
		config := interfaces.RBACConfig{}

		assert.Equal(t, 0, config.MaxRetries)
		assert.Equal(t, 0, config.RetryDelay)
		assert.Equal(t, 0, config.PropagationTimeout)
		assert.False(t, config.EnableRetry)
	})

	t.Run("RoleAssignment defaults", func(t *testing.T) {
		roleAssignment := interfaces.RoleAssignment{}

		assert.Equal(t, "", roleAssignment.RoleName)
		assert.Equal(t, "", roleAssignment.PrincipalID)
		assert.Equal(t, "", roleAssignment.Scope)
		assert.Equal(t, "", roleAssignment.Description)
	})

	t.Run("Principal defaults", func(t *testing.T) {
		principal := interfaces.Principal{}

		assert.Equal(t, "", principal.ID)
		assert.Equal(t, "", principal.Type)
	})
}

// TestStructFieldAssignments tests that all struct fields can be assigned and read
func TestStructFieldAssignments(t *testing.T) {
	t.Parallel()

	t.Run("AuthenticationConfig field assignment", func(t *testing.T) {
		config := &interfaces.AuthenticationConfig{}

		// Test field assignment
		config.SubscriptionID = "test-sub"
		config.TenantID = "test-tenant"
		config.ClientID = "test-client"
		config.ClientSecret = "test-secret"
		config.UseManagedIdentity = true

		// Verify assignments
		assert.Equal(t, "test-sub", config.SubscriptionID)
		assert.Equal(t, "test-tenant", config.TenantID)
		assert.Equal(t, "test-client", config.ClientID)
		assert.Equal(t, "test-secret", config.ClientSecret)
		assert.True(t, config.UseManagedIdentity)
	})

	t.Run("RBACConfig field assignment", func(t *testing.T) {
		config := &interfaces.RBACConfig{}

		// Test field assignment
		config.MaxRetries = 10
		config.RetryDelay = 5
		config.PropagationTimeout = 120
		config.EnableRetry = true

		// Verify assignments
		assert.Equal(t, 10, config.MaxRetries)
		assert.Equal(t, 5, config.RetryDelay)
		assert.Equal(t, 120, config.PropagationTimeout)
		assert.True(t, config.EnableRetry)
	})
}

// TestStructComparison tests that structs can be compared for equality
func TestStructComparison(t *testing.T) {
	t.Parallel()

	t.Run("AuthenticationConfig equality", func(t *testing.T) {
		config1 := interfaces.AuthenticationConfig{
			SubscriptionID: "sub1",
			TenantID:       "tenant1",
			ClientID:       "client1",
		}

		config2 := interfaces.AuthenticationConfig{
			SubscriptionID: "sub1",
			TenantID:       "tenant1",
			ClientID:       "client1",
		}

		config3 := interfaces.AuthenticationConfig{
			SubscriptionID: "sub2",
			TenantID:       "tenant1",
			ClientID:       "client1",
		}

		assert.Equal(t, config1, config2)
		assert.NotEqual(t, config1, config3)
	})

	t.Run("RoleAssignment equality", func(t *testing.T) {
		role1 := interfaces.RoleAssignment{
			RoleName:    "Owner",
			PrincipalID: "principal1",
			Scope:       "/subscriptions/sub1",
		}

		role2 := interfaces.RoleAssignment{
			RoleName:    "Owner",
			PrincipalID: "principal1",
			Scope:       "/subscriptions/sub1",
		}

		role3 := interfaces.RoleAssignment{
			RoleName:    "Contributor",
			PrincipalID: "principal1",
			Scope:       "/subscriptions/sub1",
		}

		assert.Equal(t, role1, role2)
		assert.NotEqual(t, role1, role3)
	})
}

// TestResourceNotFoundError tests the ResourceNotFoundError type
func TestResourceNotFoundError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		resourceType string
		resourceName string
		message      string
		expectError  string
	}{
		{
			name:         "with custom message",
			resourceType: "blob",
			resourceName: "test.tfstate",
			message:      "Custom error message",
			expectError:  "Custom error message",
		},
		{
			name:         "without custom message",
			resourceType: "container",
			resourceName: "tfstate-container",
			message:      "",
			expectError:  "Resource not found: container tfstate-container",
		},
		{
			name:         "minimal error",
			resourceType: "storage",
			resourceName: "account",
			message:      "",
			expectError:  "Resource not found: storage account",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var err *interfaces.ResourceNotFoundError
			if tc.message != "" {
				err = &interfaces.ResourceNotFoundError{
					ResourceType: tc.resourceType,
					Name:         tc.resourceName,
					Message:      tc.message,
				}
			} else {
				err = interfaces.NewResourceNotFoundError(tc.resourceType, tc.resourceName)
			}

			assert.Equal(t, tc.expectError, err.Error())
			assert.Equal(t, tc.resourceType, err.ResourceType)
			assert.Equal(t, tc.resourceName, err.Name)
		})
	}
}

// TestNewResourceNotFoundError tests the constructor function
func TestNewResourceNotFoundError(t *testing.T) {
	t.Parallel()

	err := interfaces.NewResourceNotFoundError("blob", "terraform.tfstate")

	assert.NotNil(t, err)
	assert.Equal(t, "blob", err.ResourceType)
	assert.Equal(t, "terraform.tfstate", err.Name)
	assert.Equal(t, "Resource not found: blob terraform.tfstate", err.Error())
}

// TestAccountKindConstants tests account kind constants
func TestAccountKindConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		constant types.AccountKind
		expected string
	}{
		{"StorageV2", interfaces.KindStorageV2, "StorageV2"},
		{"Storage", interfaces.KindStorage, "Storage"},
		{"BlockBlobStorage", interfaces.KindBlockBlobStorage, "BlockBlobStorage"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, string(tc.constant))
		})
	}
}

// TestAccountTierConstants tests account tier constants
func TestAccountTierConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		constant types.AccountTier
		expected string
	}{
		{"Standard", interfaces.TierStandard, "Standard"},
		{"Premium", interfaces.TierPremium, "Premium"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, string(tc.constant))
		})
	}
}

// TestAccessTierConstants tests access tier constants
func TestAccessTierConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		constant types.AccessTier
		expected string
	}{
		{"Hot", interfaces.TierHot, "Hot"},
		{"Cool", interfaces.TierCool, "Cool"},
		{"Archive", interfaces.TierArchive, "Archive"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, string(tc.constant))
		})
	}
}

// TestReplicationTypeConstants tests replication type constants
func TestReplicationTypeConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		constant types.ReplicationType
		expected string
	}{
		{"RAGRS", interfaces.RAGRS, "RAGRS"},
		{"GRS", interfaces.GRS, "GRS"},
		{"LRS", interfaces.LRS, "LRS"},
		{"ZRS", interfaces.ZRS, "ZRS"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, string(tc.constant))
		})
	}
}

// TestInterfaceBasicUsage tests basic interface usage without implementation
func TestInterfaceBasicUsage(t *testing.T) {
	t.Parallel()

	// Test that we can declare variables of interface types
	var storageService interfaces.StorageAccountService
	var blobService interfaces.BlobService
	var resourceGroupService interfaces.ResourceGroupService

	// Test that interfaces can be assigned nil
	storageService = nil
	blobService = nil
	resourceGroupService = nil

	// Test that the variables are properly typed
	assert.Nil(t, storageService)
	assert.Nil(t, blobService)
	assert.Nil(t, resourceGroupService)
}
