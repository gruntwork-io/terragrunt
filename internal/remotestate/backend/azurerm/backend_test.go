// Package azurerm represents Azure storage backend for remote state
package azurerm_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	azurerm "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	azuretesting "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm/testing"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createLogger() log.Logger {
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)
	return log.New(log.WithLevel(log.DebugLevel), log.WithFormatter(formatter))
}

func TestNewBackend(t *testing.T) {
	t.Parallel()

	b := azurerm.NewBackend()
	assert.NotNil(t, b)
	assert.IsType(t, &azurerm.Backend{}, b)
}

func TestBackendBootstrapAndNeedsBootstrap(t *testing.T) {
	t.Parallel()
	// Skip test if we don't have Azure credentials
	accountName, _ := azuretesting.CheckAzureTestCredentials(t)

	l := createLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	b := azurerm.NewBackend()
	config := backend.Config{
		"storage_account_name": accountName,
		"container_name":       "terragrunt-test-container",
		"key":                  "test/terraform.tfstate",
	}

	// First check if it needs initialization
	needsInit, err := b.NeedsBootstrap(t.Context(), l, config, opts)
	require.NoError(t, err)
	assert.True(t, needsInit)

	// Bootstrap the backend
	err = b.Bootstrap(t.Context(), l, config, opts)
	require.NoError(t, err)

	// Container should exist after bootstrap
	needsInit, err = b.NeedsBootstrap(t.Context(), l, config, opts)
	require.NoError(t, err)
	assert.False(t, needsInit)
}

func TestBackendBootstrapWithTags(t *testing.T) {
	t.Parallel()
	// Skip test if we don't have Azure credentials
	accountName, _ := azurerm.CheckAzureTestCredentials(t)

	l := createLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	b := azurerm.NewBackend()
	config := backend.Config{
		"storage_account_name": accountName,
		"container_name":       "terragrunt-test-container-tags",
		"key":                  "test/terraform.tfstate",
	}

	// Bootstrap with tags
	err = b.Bootstrap(t.Context(), l, config, opts)
	require.NoError(t, err)

	// Container should exist with tags after bootstrap
	var needsInit bool
	needsInit, err = b.NeedsBootstrap(t.Context(), l, config, opts)
	require.NoError(t, err)
	assert.False(t, needsInit)
}

func TestBackendBootstrapInvalidConfig(t *testing.T) {
	t.Parallel()

	l := createLogger()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	testCases := []struct {
		config      backend.Config // map pointer (8 bytes) - place first for alignment
		name        string         // string (16 bytes) - after pointer type
		expectError bool           // bool (1 byte) - at end for padding
	}{
		{
			name: "missing-storage-account",
			config: backend.Config{
				"container_name": "test-container",
				"key":            "test/terraform.tfstate",
			},
			expectError: true,
		},
		{
			name: "missing-container",
			config: backend.Config{
				"storage_account_name": "testaccount",
				"key":                  "test/terraform.tfstate",
			},
			expectError: true,
		},
		{
			name: "missing-key",
			config: backend.Config{
				"storage_account_name": "testaccount",
				"container_name":       "test-container",
			},
			expectError: true,
		},
	}

	b := azurerm.NewBackend()
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := b.NeedsBootstrap(t.Context(), l, tc.config, opts)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
