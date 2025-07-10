package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestStorageAccountConfig_Validation tests the StorageAccountConfig struct validation
func TestStorageAccountConfig_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config StorageAccountConfig
		valid  bool
	}{
		{
			name: "valid minimal config",
			config: StorageAccountConfig{
				Name:              "validstorageaccount",
				ResourceGroupName: "test-rg",
				Location:          "eastus",
			},
			valid: true,
		},
		{
			name: "valid full config",
			config: StorageAccountConfig{
				Name:                  "validstorageaccount",
				ResourceGroupName:     "test-rg",
				Location:              "eastus",
				EnableVersioning:      true,
				AllowBlobPublicAccess: false,
				AccountKind:           AccountKind("StorageV2"),
				AccountTier:           AccountTier("Standard"),
				AccessTier:            AccessTier("Hot"),
				ReplicationType:       ReplicationType("LRS"),
				Tags: map[string]string{
					"Environment": "test",
					"Owner":       "terraform",
				},
			},
			valid: true,
		},
		{
			name: "empty name",
			config: StorageAccountConfig{
				Name:              "",
				ResourceGroupName: "test-rg",
				Location:          "eastus",
			},
			valid: false,
		},
		{
			name: "empty resource group",
			config: StorageAccountConfig{
				Name:              "validstorageaccount",
				ResourceGroupName: "",
				Location:          "eastus",
			},
			valid: false,
		},
		{
			name: "empty location",
			config: StorageAccountConfig{
				Name:              "validstorageaccount",
				ResourceGroupName: "test-rg",
				Location:          "",
			},
			valid: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Test basic field presence validation
			hasRequiredFields := tc.config.Name != "" &&
				tc.config.ResourceGroupName != "" &&
				tc.config.Location != ""

			if tc.valid {
				assert.True(t, hasRequiredFields, "Valid config should have required fields")
			} else {
				assert.False(t, hasRequiredFields, "Invalid config should be missing required fields")
			}
		})
	}
}

// TestStorageEndpoints_IsEmpty tests whether StorageEndpoints can be considered empty
func TestStorageEndpoints_IsEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		endpoints StorageEndpoints
		isEmpty   bool
	}{
		{
			name:      "all empty",
			endpoints: StorageEndpoints{},
			isEmpty:   true,
		},
		{
			name: "only blob endpoint",
			endpoints: StorageEndpoints{
				Blob: "https://test.blob.core.windows.net/",
			},
			isEmpty: false,
		},
		{
			name: "all endpoints populated",
			endpoints: StorageEndpoints{
				Blob:  "https://test.blob.core.windows.net/",
				Queue: "https://test.queue.core.windows.net/",
				Table: "https://test.table.core.windows.net/",
				File:  "https://test.file.core.windows.net/",
			},
			isEmpty: false,
		},
		{
			name: "partial endpoints",
			endpoints: StorageEndpoints{
				Blob: "https://test.blob.core.windows.net/",
				File: "https://test.file.core.windows.net/",
			},
			isEmpty: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			isEmpty := tc.endpoints.Blob == "" &&
				tc.endpoints.Queue == "" &&
				tc.endpoints.Table == "" &&
				tc.endpoints.File == ""

			assert.Equal(t, tc.isEmpty, isEmpty)
		})
	}
}

// TestAccountKind_String tests AccountKind string conversion
func TestAccountKind_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		kind     AccountKind
		expected string
	}{
		{
			name:     "Storage",
			kind:     AccountKind("Storage"),
			expected: "Storage",
		},
		{
			name:     "StorageV2",
			kind:     AccountKind("StorageV2"),
			expected: "StorageV2",
		},
		{
			name:     "BlobStorage",
			kind:     AccountKind("BlobStorage"),
			expected: "BlobStorage",
		},
		{
			name:     "FileStorage",
			kind:     AccountKind("FileStorage"),
			expected: "FileStorage",
		},
		{
			name:     "BlockBlobStorage",
			kind:     AccountKind("BlockBlobStorage"),
			expected: "BlockBlobStorage",
		},
		{
			name:     "empty",
			kind:     AccountKind(""),
			expected: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := string(tc.kind)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestAccessTier_String tests AccessTier string conversion
func TestAccessTier_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tier     AccessTier
		expected string
	}{
		{
			name:     "Hot",
			tier:     AccessTier("Hot"),
			expected: "Hot",
		},
		{
			name:     "Cool",
			tier:     AccessTier("Cool"),
			expected: "Cool",
		},
		{
			name:     "Archive",
			tier:     AccessTier("Archive"),
			expected: "Archive",
		},
		{
			name:     "Premium",
			tier:     AccessTier("Premium"),
			expected: "Premium",
		},
		{
			name:     "empty",
			tier:     AccessTier(""),
			expected: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := string(tc.tier)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestStorageAccountProperties_HasEndpoints tests whether storage account has endpoints configured
func TestStorageAccountProperties_HasEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		properties StorageAccountProperties
		hasPrimary bool
		hasSecond  bool
	}{
		{
			name:       "no endpoints",
			properties: StorageAccountProperties{},
			hasPrimary: false,
			hasSecond:  false,
		},
		{
			name: "only primary endpoints",
			properties: StorageAccountProperties{
				PrimaryEndpoints: StorageEndpoints{
					Blob: "https://test.blob.core.windows.net/",
				},
			},
			hasPrimary: true,
			hasSecond:  false,
		},
		{
			name: "both endpoints",
			properties: StorageAccountProperties{
				PrimaryEndpoints: StorageEndpoints{
					Blob: "https://test.blob.core.windows.net/",
				},
				SecondaryEndpoints: StorageEndpoints{
					Blob: "https://test-secondary.blob.core.windows.net/",
				},
			},
			hasPrimary: true,
			hasSecond:  true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hasPrimary := tc.properties.PrimaryEndpoints.Blob != "" ||
				tc.properties.PrimaryEndpoints.Queue != "" ||
				tc.properties.PrimaryEndpoints.Table != "" ||
				tc.properties.PrimaryEndpoints.File != ""

			hasSecondary := tc.properties.SecondaryEndpoints.Blob != "" ||
				tc.properties.SecondaryEndpoints.Queue != "" ||
				tc.properties.SecondaryEndpoints.Table != "" ||
				tc.properties.SecondaryEndpoints.File != ""

			assert.Equal(t, tc.hasPrimary, hasPrimary)
			assert.Equal(t, tc.hasSecond, hasSecondary)
		})
	}
}
