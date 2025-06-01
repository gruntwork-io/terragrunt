package azurerm

import (
	"context"
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBackend(t *testing.T) {
	t.Parallel()

	backend := NewBackend()
	assert.NotNil(t, backend)
	assert.Equal(t, BackendName, backend.Name())
}

func TestBackendInitialization(t *testing.T) {
	if os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT") == "" ||
		os.Getenv("TERRAGRUNT_AZURE_TEST_ACCESS_KEY") == "" {
		t.Skip("Skipping Azure backend initialization test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT or TERRAGRUNT_AZURE_TEST_ACCESS_KEY not set")
	}

	t.Parallel()

	ctx := context.Background()
	containerName := "test-container"

	backend := NewBackend()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	config := backend.Config{
		"storage_account_name": os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT"),
		"storage_account_key":  os.Getenv("TERRAGRUNT_AZURE_TEST_ACCESS_KEY"),
		"container_name":      containerName,
		"key":                "test/terraform.tfstate",
	}

	// Test NeedsBootstrap
	needsInit, err := backend.NeedsBootstrap(ctx, log.Logger{}, config, opts)
	require.NoError(t, err)
	assert.True(t, needsInit)

	// Test Bootstrap
	err = backend.Bootstrap(ctx, log.Logger{}, config, opts)
	require.NoError(t, err)

	// Test NeedsBootstrap after initialization
	needsInit, err = backend.NeedsBootstrap(ctx, log.Logger{}, config, opts)
	require.NoError(t, err)
	assert.False(t, needsInit)

	// Clean up
	err = backend.Delete(ctx, log.Logger{}, config, opts)
	require.NoError(t, err)
}

func TestBackendErrorHandling(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend := NewBackend()
	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	invalidConfig := backend.Config{
		"storage_account_name": "invalid-account-name",
		"container_name":      "invalid$container",
		"key":                "test/terraform.tfstate",
	}

	// Test NeedsBootstrap with invalid config
	_, err = backend.NeedsBootstrap(ctx, log.Logger{}, invalidConfig, opts)
	assert.Error(t, err)

	// Test Bootstrap with invalid config
	err = backend.Bootstrap(ctx, log.Logger{}, invalidConfig, opts)
	assert.Error(t, err)

	// Test Delete with invalid config
	err = backend.Delete(ctx, log.Logger{}, invalidConfig, opts)
	assert.Error(t, err)
}

func TestBackendConfigFiltering(t *testing.T) {
	t.Parallel()

	backend := NewBackend()
	config := backend.Config{
		"storage_account_name":   "testaccount",
		"container_name":        "test-container",
		"key":                  "test/terraform.tfstate",
		"skip_blob_versioning": true,
		"container_tags": map[string]string{
			"Environment": "Test",
		},
	}

	initArgs := backend.GetTFInitArgs(config)
	assert.NotContains(t, initArgs, "skip_blob_versioning")
	assert.NotContains(t, initArgs, "container_tags")
	assert.Contains(t, initArgs, "storage_account_name")
	assert.Contains(t, initArgs, "container_name")
	assert.Contains(t, initArgs, "key")
}
