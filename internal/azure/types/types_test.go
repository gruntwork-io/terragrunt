package types_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/azure/types"
	"github.com/stretchr/testify/assert"
)

// TestStorageAccountConfig tests the StorageAccountConfig struct
func TestStorageAccountConfig(t *testing.T) {
	t.Parallel()

	tests := []struct { // nolint:govet // fieldalignment is acceptable in table-driven tests
		config *types.StorageAccountConfig
		name   string
	}{
		{
			config: &types.StorageAccountConfig{
				Name:                  "testaccount",
				ResourceGroupName:     "test-rg",
				Location:              "eastus",
				EnableVersioning:      true,
				AllowBlobPublicAccess: false,
				AccountKind:           types.AccountKind("StorageV2"),
				AccountTier:           types.AccountTier("Standard"),
				AccessTier:            types.AccessTier("Hot"),
				ReplicationType:       types.ReplicationType("LRS"),
				Tags: map[string]string{
					"Environment": "test",
					"Owner":       "terragrunt",
				},
			},
			name: "complete config",
		},
		{
			config: &types.StorageAccountConfig{
				Name:              "minimalaccount",
				ResourceGroupName: "minimal-rg",
				Location:          "westus",
			},
			name: "minimal config",
		},
		{
			config: &types.StorageAccountConfig{},
			name:   "empty config",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test that the struct can be created and fields accessed
			assert.NotNil(t, tc.config)

			// Verify string fields are accessible
			_ = tc.config.Name
			_ = tc.config.ResourceGroupName
			_ = tc.config.Location

			// Verify bool fields are accessible
			_ = tc.config.EnableVersioning
			_ = tc.config.AllowBlobPublicAccess

			// Verify enum fields are accessible
			_ = tc.config.AccountKind
			_ = tc.config.AccountTier
			_ = tc.config.AccessTier
			_ = tc.config.ReplicationType

			// Verify map field is accessible
			_ = tc.config.Tags
		})
	}
}

// TestStorageAccount tests the StorageAccount struct
func TestStorageAccount(t *testing.T) {
	t.Parallel()

	tests := []struct { // nolint:govet // fieldalignment is acceptable in table-driven tests
		account *types.StorageAccount
		name    string
	}{
		{
			account: &types.StorageAccount{
				Name:              "testaccount",
				ResourceGroupName: "test-rg",
				Location:          "eastus",
				Properties: &types.StorageAccountProperties{
					AccessTier:        types.AccessTier("Hot"),
					EnableVersioning:  true,
					IsHnsEnabled:      false,
					Kind:              types.AccountKind("StorageV2"),
					ProvisioningState: "Succeeded",
					StatusOfPrimary:   "Available",
					StatusOfSecondary: "Unavailable",
					SupportsHTTPSOnly: true,
					PrimaryEndpoints: types.StorageEndpoints{
						Blob:  "https://test.blob.core.windows.net/",
						Queue: "https://test.queue.core.windows.net/",
						Table: "https://test.table.core.windows.net/",
						File:  "https://test.file.core.windows.net/",
					},
					SecondaryEndpoints: types.StorageEndpoints{
						Blob: "https://test-secondary.blob.core.windows.net/",
					},
				},
			},
			name: "complete account",
		},
		{
			account: &types.StorageAccount{
				Name:              "minimal",
				ResourceGroupName: "minimal-rg",
				Location:          "westus",
			},
			name: "minimal account",
		},
		{
			account: &types.StorageAccount{},
			name:    "empty account",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.NotNil(t, tc.account)

			// Verify basic fields are accessible
			_ = tc.account.Name
			_ = tc.account.ResourceGroupName
			_ = tc.account.Location
			_ = tc.account.Properties
		})
	}
}

// TestStorageEndpoints tests the StorageEndpoints struct
func TestStorageEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct { // nolint:govet // fieldalignment is acceptable in table-driven tests
		endpoints types.StorageEndpoints
		name      string
	}{
		{
			endpoints: types.StorageEndpoints{
				Blob:  "https://test.blob.core.windows.net/",
				Queue: "https://test.queue.core.windows.net/",
				Table: "https://test.table.core.windows.net/",
				File:  "https://test.file.core.windows.net/",
			},
			name: "complete endpoints",
		},
		{
			name: "partial endpoints",
			endpoints: types.StorageEndpoints{
				Blob: "https://test.blob.core.windows.net/",
			},
		},
		{
			name:      "empty endpoints",
			endpoints: types.StorageEndpoints{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Verify all endpoint fields are accessible
			_ = tc.endpoints.Blob
			_ = tc.endpoints.Queue
			_ = tc.endpoints.Table
			_ = tc.endpoints.File
		})
	}
}

// TestGetObjectInput tests the GetObjectInput struct
func TestGetObjectInput(t *testing.T) {
	t.Parallel()

	input := &types.GetObjectInput{
		StorageAccountName: "teststorage",
		ContainerName:      "test-container",
		BlobName:           "test-blob",
	}

	assert.Equal(t, "teststorage", input.StorageAccountName)
	assert.Equal(t, "test-container", input.ContainerName)
	assert.Equal(t, "test-blob", input.BlobName)
}

// TestGetObjectOutput tests the GetObjectOutput struct
func TestGetObjectOutput(t *testing.T) {
	t.Parallel()

	testData := []byte("test data")
	properties := map[string]string{
		"ContentType": "application/octet-stream",
		"ETag":        "\"test-etag\"",
	}

	output := &types.GetObjectOutput{
		Content:    testData,
		Properties: properties,
	}

	assert.Equal(t, testData, output.Content)
	assert.Equal(t, properties, output.Properties)
	assert.Equal(t, "application/octet-stream", output.Properties["ContentType"])
	assert.Equal(t, "\"test-etag\"", output.Properties["ETag"])
}

// TestTypeStringConversions tests that type aliases work correctly as strings
func TestTypeStringConversions(t *testing.T) {
	t.Parallel()

	tests := []struct { // nolint:govet // fieldalignment is acceptable in table-driven tests
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "AccountKind conversion",
			testFunc: func(t *testing.T) {
				t.Helper()

				kind := types.AccountKind("StorageV2")
				assert.Equal(t, "StorageV2", string(kind))
			},
		},
		{
			name: "AccountTier conversion",
			testFunc: func(t *testing.T) {
				t.Helper()

				tier := types.AccountTier("Standard")
				assert.Equal(t, "Standard", string(tier))
			},
		},
		{
			name: "AccessTier conversion",
			testFunc: func(t *testing.T) {
				t.Helper()

				accessTier := types.AccessTier("Hot")
				assert.Equal(t, "Hot", string(accessTier))
			},
		},
		{
			name: "ReplicationType conversion",
			testFunc: func(t *testing.T) {
				t.Helper()

				replication := types.ReplicationType("LRS")
				assert.Equal(t, "LRS", string(replication))
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.testFunc(t)
		})
	}
}

// TestStorageAccountPropertiesDefaults tests default values for properties
func TestStorageAccountPropertiesDefaults(t *testing.T) {
	t.Parallel()

	properties := &types.StorageAccountProperties{}

	// Test that zero values are as expected
	assert.Equal(t, types.AccessTier(""), properties.AccessTier)
	assert.False(t, properties.EnableVersioning)
	assert.False(t, properties.IsHnsEnabled)
	assert.Equal(t, types.AccountKind(""), properties.Kind)
	assert.Empty(t, properties.ProvisioningState)
	assert.Empty(t, properties.StatusOfPrimary)
	assert.Empty(t, properties.StatusOfSecondary)
	assert.False(t, properties.SupportsHTTPSOnly)

	// Test that endpoints are zero-valued structs
	assert.Equal(t, types.StorageEndpoints{}, properties.PrimaryEndpoints)
	assert.Equal(t, types.StorageEndpoints{}, properties.SecondaryEndpoints)
}

// TestStructFieldAssignment tests that all struct fields can be assigned
func TestStructFieldAssignment(t *testing.T) {
	t.Parallel()

	// Test StorageAccountConfig assignment
	config := &types.StorageAccountConfig{}
	config.Name = "test"
	config.ResourceGroupName = "rg"
	config.Location = "eastus"
	config.EnableVersioning = true
	config.AllowBlobPublicAccess = false
	config.AccountKind = types.AccountKind("StorageV2")
	config.AccountTier = types.AccountTier("Standard")
	config.AccessTier = types.AccessTier("Hot")
	config.ReplicationType = types.ReplicationType("LRS")
	config.Tags = map[string]string{"test": "value"}

	assert.Equal(t, "test", config.Name)
	assert.Equal(t, "rg", config.ResourceGroupName)
	assert.Equal(t, "eastus", config.Location)
	assert.True(t, config.EnableVersioning)
	assert.False(t, config.AllowBlobPublicAccess)
	assert.Equal(t, types.AccountKind("StorageV2"), config.AccountKind)
	assert.Equal(t, types.AccountTier("Standard"), config.AccountTier)
	assert.Equal(t, types.AccessTier("Hot"), config.AccessTier)
	assert.Equal(t, types.ReplicationType("LRS"), config.ReplicationType)
	assert.Equal(t, map[string]string{"test": "value"}, config.Tags)

	// Test StorageAccount assignment
	account := &types.StorageAccount{}
	account.Name = "account"
	account.ResourceGroupName = "rg"
	account.Location = "westus"
	account.Properties = &types.StorageAccountProperties{}

	assert.Equal(t, "account", account.Name)
	assert.Equal(t, "rg", account.ResourceGroupName)
	assert.Equal(t, "westus", account.Location)
	assert.NotNil(t, account.Properties)

	// Test StorageEndpoints assignment
	endpoints := &types.StorageEndpoints{}
	endpoints.Blob = "blob-url"
	endpoints.Queue = "queue-url"
	endpoints.Table = "table-url"
	endpoints.File = "file-url"

	assert.Equal(t, "blob-url", endpoints.Blob)
	assert.Equal(t, "queue-url", endpoints.Queue)
	assert.Equal(t, "table-url", endpoints.Table)
	assert.Equal(t, "file-url", endpoints.File)
}
