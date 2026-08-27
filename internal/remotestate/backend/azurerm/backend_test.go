package azurerm_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackendName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, azurerm.BackendName, azurerm.NewBackend().Name())
}

// TestExperimentGate pins how a disabled experiment behaves. The passive
// lifecycle checks must be no-ops: before this backend existed an azurerm
// config inherited CommonBackend's no-op, so a globally applied
// --backend-bootstrap continued into native init. Gating must not turn that
// previously working path into a failure. Explicitly invoked destructive
// commands still report that the experiment is required.
func TestExperimentGate(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := t.Context()
	bcfg := backend.Config(fullConfig())
	opts := optsWithExperiment(t, false)
	b := azurerm.NewBackend()

	t.Run("NeedsBootstrap is a no-op", func(t *testing.T) {
		t.Parallel()

		needs, err := b.NeedsBootstrap(ctx, l, venvtest.New(), bcfg, opts)
		require.NoError(t, err)
		assert.False(t, needs)
	})

	t.Run("Bootstrap is a no-op", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, b.Bootstrap(ctx, l, venvtest.New(), bcfg, opts))
	})

	t.Run("IsVersionControlEnabled is a no-op", func(t *testing.T) {
		t.Parallel()

		enabled, err := b.IsVersionControlEnabled(ctx, l, venvtest.New(), bcfg, opts)
		require.NoError(t, err)
		assert.False(t, enabled)
	})

	t.Run("Delete reports the experiment", func(t *testing.T) {
		t.Parallel()
		require.ErrorIs(t, b.Delete(ctx, l, venvtest.New(), bcfg, opts), azurerm.ErrAzureBackendExperimentRequired)
	})

	t.Run("DeleteBucket reports the experiment", func(t *testing.T) {
		t.Parallel()
		require.ErrorIs(
			t,
			b.DeleteBucket(ctx, l, venvtest.New(), bcfg, opts),
			azurerm.ErrAzureBackendExperimentRequired,
		)
	})

	t.Run("Migrate reports the experiment", func(t *testing.T) {
		t.Parallel()
		require.ErrorIs(t, b.Migrate(ctx, l, venvtest.New(), venvtest.New(), bcfg, bcfg, opts), azurerm.ErrAzureBackendExperimentRequired)
	})
}

// TestExperimentEnabled_InvalidConfigSurfaces verifies that once the experiment
// is enabled, the gate is passed and config validation runs (an invalid config
// returns a validation error rather than the experiment error).
func TestExperimentEnabled_InvalidConfigSurfaces(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := t.Context()
	opts := optsWithExperiment(t, true)
	b := azurerm.NewBackend()

	// Missing required keys -> validation error, NOT the experiment error.
	_, err := b.NeedsBootstrap(ctx, l, venvtest.New(), backend.Config{}, opts)
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
// cross-storage-account migration (its blob client is bound to a single
// account) instead of silently writing state into the source account.
func TestMigrate_CrossAccountRefused(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := t.Context()
	opts := optsWithExperiment(t, true)
	b := azurerm.NewBackend()

	srcCfg := backend.Config(fullConfig())

	dstRaw := fullConfig()
	dstRaw["storage_account_name"] = "differentaccount"
	dstCfg := backend.Config(dstRaw)

	err := b.Migrate(ctx, l, venvtest.New(), venvtest.New(), srcCfg, dstCfg, opts)
	require.Error(t, err)

	var crossAccount *azurerm.CrossAccountMigrationError
	require.ErrorAs(t, err, &crossAccount)
	assert.Equal(t, "tfstate1234", crossAccount.SrcStorageAccount)
	assert.Equal(t, "differentaccount", crossAccount.DstStorageAccount)
}

// TestMigrate_CrossCloudRefused verifies the azurerm backend refuses a
// migration whose destination names the same storage account in a different
// Azure cloud. Storage account names are unique only within a cloud, and the
// blob client is built from the source config, so allowing this would write
// the state into the source account and then delete the source key.
func TestMigrate_CrossCloudRefused(t *testing.T) {
	t.Parallel()

	b := azurerm.NewBackend()
	opts := optsWithExperiment(t, true)

	srcRaw := fullConfig()
	srcRaw["environment"] = "public"

	// Same account name, different sovereign cloud.
	dstRaw := fullConfig()
	dstRaw["environment"] = "usgovernment"

	err := b.Migrate(
		t.Context(), logger.CreateLogger(), venvtest.New(), venvtest.New(),
		backend.Config(srcRaw), backend.Config(dstRaw), opts)

	require.Error(t, err)

	var crossCloud *azurerm.CrossCloudMigrationError
	require.ErrorAs(t, err, &crossCloud)
	assert.Equal(t, "public", crossCloud.SrcEnvironment)
	assert.Equal(t, "usgovernment", crossCloud.DstEnvironment)
}

// TestMigrate_SameCloudAliasAllowed verifies the cloud comparison is by
// canonical cloud, not raw string: "AzurePublicCloud" is an alias of "public"
// and must not be refused as cross-cloud. The destination names a different
// account so the call returns at the cross-account gate, which sits after the
// cloud gate and before any client construction, keeping the test hermetic.
func TestMigrate_SameCloudAliasAllowed(t *testing.T) {
	t.Parallel()

	b := azurerm.NewBackend()
	opts := optsWithExperiment(t, true)

	srcRaw := fullConfig()
	srcRaw["environment"] = "public"

	dstRaw := fullConfig()
	dstRaw["environment"] = "AzurePublicCloud" // alias for the same cloud
	dstRaw["storage_account_name"] = "otheraccount"

	err := b.Migrate(
		t.Context(), logger.CreateLogger(), venvtest.New(), venvtest.New(),
		backend.Config(srcRaw), backend.Config(dstRaw), opts)

	var crossCloud *azurerm.CrossCloudMigrationError
	assert.NotErrorAs(t, err, &crossCloud, "an alias of the same cloud must not be treated as cross-cloud")

	// Reaching the cross-account gate proves the cloud gate let the alias through.
	var crossAccount *azurerm.CrossAccountMigrationError
	require.ErrorAs(t, err, &crossAccount)
}

// TestNeedsBootstrap_SkipsArmPlaneWhenNoArmWork verifies a user-managed
// account with all creation and policy work skipped needs no ARM access.
func TestNeedsBootstrap_SkipsArmPlaneWhenNoArmWork(t *testing.T) {
	t.Parallel()

	needs, err := azurerm.NewBackend().NeedsBootstrap(
		t.Context(),
		logger.CreateLogger(), venvtest.New(), backend.Config(rgLessSkipAllConfig()), optsWithExperiment(t, true))
	require.NoError(t, err)
	assert.False(t, needs)
}

// TestBootstrap_SkipsArmPlaneWhenNoArmWork verifies Bootstrap succeeds without
// a resource group when the account is user-managed and nothing is converged.
func TestBootstrap_SkipsArmPlaneWhenNoArmWork(t *testing.T) {
	t.Parallel()

	err := azurerm.NewBackend().Bootstrap(
		t.Context(),
		logger.CreateLogger(), venvtest.New(), backend.Config(rgLessSkipAllConfig()), optsWithExperiment(t, true))
	require.NoError(t, err)
}

// TestIsVersionControlEnabled_NoResourceGroupDegrades verifies the versioning
// check degrades to false instead of erroring when no resource group is known.
func TestIsVersionControlEnabled_NoResourceGroupDegrades(t *testing.T) {
	t.Parallel()

	cfg := fullConfig()
	delete(cfg, "resource_group_name")

	enabled, err := azurerm.NewBackend().IsVersionControlEnabled(
		t.Context(), logger.CreateLogger(), venvtest.New(), backend.Config(cfg), optsWithExperiment(t, true))
	require.NoError(t, err)
	assert.False(t, enabled)
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

// rgLessSkipAllConfig returns a config with no resource group and every
// creation or policy step skipped, so no Azure call is required.
func rgLessSkipAllConfig() azurerm.Config {
	cfg := fullConfig()
	delete(cfg, "resource_group_name")
	delete(cfg, "enable_soft_delete")
	delete(cfg, "soft_delete_retention_days")
	// A role assignment is ARM work too, so it has to go for this fixture to
	// mean "nothing needs the management plane".
	delete(cfg, "assign_blob_data_role")
	delete(cfg, "principal_id")

	cfg["skip_storage_account_creation"] = true
	cfg["skip_versioning"] = true
	cfg["skip_container_creation"] = true

	return cfg
}

// TestMigrate_CrossCloudFromDestinationEnvRefused pins the case a config-only
// check misses: neither side sets `environment`, but the destination
// environment selects a different cloud through ARM_ENVIRONMENT. The source and
// destination each carry their own venv precisely because the same variable can
// differ per side, so the destination cloud must be resolved from dstV.
func TestMigrate_CrossCloudFromDestinationEnvRefused(t *testing.T) {
	t.Parallel()

	srcV := (venvtest.New()).WithEnv(map[string]string{"ARM_ENVIRONMENT": "public"})
	dstV := (venvtest.New()).WithEnv(map[string]string{"ARM_ENVIRONMENT": "usgovernment"})

	cfg := backend.Config(fullConfig()) // identical config on both sides

	err := azurerm.NewBackend().Migrate(
		t.Context(), logger.CreateLogger(), srcV, dstV, cfg, cfg, optsWithExperiment(t, true))

	var crossCloud *azurerm.CrossCloudMigrationError
	require.ErrorAs(t, err, &crossCloud)

	// The clouds came from env vars, not config keys, so the message must fall
	// back to the resolved identities instead of naming two empty strings.
	assert.NotEmpty(t, crossCloud.SrcEnvironment)
	assert.NotEmpty(t, crossCloud.DstEnvironment)
	assert.NotEqual(t, crossCloud.SrcEnvironment, crossCloud.DstEnvironment)
}
