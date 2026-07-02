package azurerm_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackendName(t *testing.T) {
	t.Parallel()

	if got := azurerm.NewBackend().Name(); got != azurerm.BackendName {
		t.Fatalf("Name() = %q, want %q", got, azurerm.BackendName)
	}
}

// TestExperimentGate verifies that every lifecycle entry point refuses to run
// without the azure-backend experiment, before any network call is attempted.
func TestExperimentGate(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()
	bcfg := backend.Config(fullConfig())
	opts := optsWithExperiment(t, false)
	b := azurerm.NewBackend()

	t.Run("Bootstrap", func(t *testing.T) {
		t.Parallel()
		require.ErrorIs(t, b.Bootstrap(ctx, l, bcfg, opts), azurerm.ErrAzureBackendExperimentRequired)
	})

	t.Run("NeedsBootstrap", func(t *testing.T) {
		t.Parallel()

		_, err := b.NeedsBootstrap(ctx, l, bcfg, opts)
		require.ErrorIs(t, err, azurerm.ErrAzureBackendExperimentRequired)
	})

	t.Run("IsVersionControlEnabled", func(t *testing.T) {
		t.Parallel()

		_, err := b.IsVersionControlEnabled(ctx, l, bcfg, opts)
		require.ErrorIs(t, err, azurerm.ErrAzureBackendExperimentRequired)
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		require.ErrorIs(t, b.Delete(ctx, l, bcfg, opts), azurerm.ErrAzureBackendExperimentRequired)
	})

	t.Run("DeleteBucket", func(t *testing.T) {
		t.Parallel()
		require.ErrorIs(t, b.DeleteBucket(ctx, l, bcfg, opts), azurerm.ErrAzureBackendExperimentRequired)
	})

	t.Run("Migrate", func(t *testing.T) {
		t.Parallel()
		require.ErrorIs(t, b.Migrate(ctx, l, bcfg, bcfg, opts), azurerm.ErrAzureBackendExperimentRequired)
	})
}

// TestExperimentEnabled_InvalidConfigSurfaces verifies that once the experiment
// is enabled, the gate is passed and config validation runs (an invalid config
// returns a validation error rather than the experiment error).
func TestExperimentEnabled_InvalidConfigSurfaces(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()
	opts := optsWithExperiment(t, true)
	b := azurerm.NewBackend()

	// Missing required keys -> validation error, NOT the experiment error.
	_, err := b.NeedsBootstrap(ctx, l, backend.Config{}, opts)
	require.Error(t, err)
	assert.NotErrorIs(t, err, azurerm.ErrAzureBackendExperimentRequired)
}

// TestGetTFInitArgs_Backend exercises the Backend.GetTFInitArgs entry point.
func TestGetTFInitArgs_Backend(t *testing.T) {
	t.Parallel()

	cfg := fullConfig()
	cfg["msi_resource_id"] = "/subscriptions/x/resourceGroups/y/providers/Microsoft.ManagedIdentity/userAssignedIdentities/z"

	args := azurerm.NewBackend().GetTFInitArgs(backend.Config(cfg))
	assert.Equal(t, "tfstate1234", args["storage_account_name"])

	_, ok := args["location"]
	assert.False(t, ok, "location is a terragrunt-only key and must not be forwarded")

	// msi_resource_id is not a valid azurerm backend argument and must be stripped.
	_, ok = args["msi_resource_id"]
	assert.False(t, ok, "msi_resource_id must not be forwarded to tofu init")
}

// TestMigrate_CrossAccountRefused verifies the azurerm backend refuses a
// cross-storage-account migration (its server-side copy is account-scoped)
// instead of silently writing state into the source account.
func TestMigrate_CrossAccountRefused(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()
	opts := optsWithExperiment(t, true)
	b := azurerm.NewBackend()

	srcCfg := backend.Config(fullConfig())

	dstRaw := fullConfig()
	dstRaw["storage_account_name"] = "differentaccount"
	dstCfg := backend.Config(dstRaw)

	err := b.Migrate(ctx, l, srcCfg, dstCfg, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cross-account")
}

// optsWithExperiment returns backend.Options with the azure-backend experiment
// enabled (or not), without touching real Azure.
func optsWithExperiment(t *testing.T, enabled bool) *backend.Options {
	t.Helper()

	exps := experiment.NewExperiments()
	if enabled {
		require.NoError(t, exps.EnableExperiment(experiment.AzureBackend))
	}

	return &backend.Options{Experiments: exps, NonInteractive: true}
}
