package azurerm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
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

// TestBootstrap_AssignBlobDataRoleRequiresARM refuses silent success when the
// flag is set under data-plane-only auth that cannot call roleAssignments.
func TestBootstrap_AssignBlobDataRoleRequiresARM(t *testing.T) {
	t.Parallel()

	cfg := fullConfig()
	cfg["access_key"] = "dGVzdGtleQ=="
	cfg["assign_blob_data_role"] = true
	delete(cfg, "use_azuread_auth")

	err := azurerm.NewBackend().Bootstrap(
		t.Context(),
		logger.CreateLogger(), venvtest.New(), backend.Config(cfg), optsWithExperiment(t, true))

	var requiresARM *azurerm.AssignBlobDataRoleRequiresARMError
	require.ErrorAs(t, err, &requiresARM)
	assert.Equal(t, azurehelper.AuthMethodAccessKey, requiresARM.Method)
}

// TestNeedsBootstrap_AssignBlobDataRoleRequiresARM mirrors Bootstrap: the flag
// must not be soft-skipped under SAS/access-key auth.
func TestNeedsBootstrap_AssignBlobDataRoleRequiresARM(t *testing.T) {
	t.Parallel()

	cfg := fullConfig()
	cfg["sas_token"] = "sv=test"
	cfg["assign_blob_data_role"] = true
	delete(cfg, "access_key")
	delete(cfg, "use_azuread_auth")

	_, err := azurerm.NewBackend().NeedsBootstrap(
		t.Context(),
		logger.CreateLogger(), venvtest.New(), backend.Config(cfg), optsWithExperiment(t, true))

	var requiresARM *azurerm.AssignBlobDataRoleRequiresARMError
	require.ErrorAs(t, err, &requiresARM)
	assert.Equal(t, azurehelper.AuthMethodSasToken, requiresARM.Method)
}

// TestNeedsBootstrap_RoleOnlyDriftRequiresBootstrap locks the regression where
// an otherwise healthy account with assign_blob_data_role set and the blob-data
// role absent returned NeedsBootstrap false, so --backend-bootstrap skipped the
// grant and OpenTofu/Terraform then failed data-plane auth.
func TestNeedsBootstrap_RoleOnlyDriftRequiresBootstrap(t *testing.T) {
	t.Parallel()

	t.Run("role absent", func(t *testing.T) {
		t.Parallel()

		needs, err := azurerm.NewBackend().NeedsBootstrap(
			t.Context(),
			logger.CreateLogger(),
			venvtest.New().WithHTTP(roleDriftHTTP(false)),
			backend.Config(roleOnlyDriftConfig()),
			optsWithExperiment(t, true),
		)
		require.NoError(t, err)
		assert.True(t, needs, "missing blob-data role must require bootstrap")
	})

	t.Run("role present", func(t *testing.T) {
		t.Parallel()

		needs, err := azurerm.NewBackend().NeedsBootstrap(
			t.Context(),
			logger.CreateLogger(),
			venvtest.New().WithHTTP(roleDriftHTTP(true)),
			backend.Config(roleOnlyDriftConfig()),
			optsWithExperiment(t, true),
		)
		require.NoError(t, err)
		assert.False(t, needs, "assigned blob-data role must not require bootstrap")
	})
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

// roleOnlyDriftConfig is a pre-created account with every policy converged
// except optional blob-data role assignment: the only ARM work left is RBAC.
func roleOnlyDriftConfig() azurerm.Config {
	cfg := fullConfig()
	delete(cfg, "enable_soft_delete")
	delete(cfg, "soft_delete_retention_days")
	delete(cfg, "use_azuread_auth")

	cfg["skip_storage_account_creation"] = true
	cfg["skip_versioning"] = true
	cfg["skip_container_creation"] = true
	cfg["assign_blob_data_role"] = true
	cfg["principal_id"] = "11111111-2222-3333-4444-555555555555"
	// Service principal auth reaches ARM through the stubbed HTTP transport.
	cfg["client_id"] = "00000000-0000-0000-0000-000000000001"
	cfg["client_secret"] = "test-secret"
	cfg["tenant_id"] = "00000000-0000-0000-0000-000000000002"

	return cfg
}

// roleDriftHTTP answers Entra token issuance, storage-account Exists, and
// role-assignment list so NeedsBootstrap can exercise blobDataRoleMissing
// without a live subscription.
func roleDriftHTTP(rolePresent bool) vhttp.Client {
	jsonHeaders := http.Header{"Content-Type": []string{"application/json"}}

	return vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		path := req.URL.Path

		switch {
		case strings.HasSuffix(path, "/discovery/instance"):
			return vhttp.Respond(http.StatusOK, []byte(
				`{"tenant_discovery_endpoint":"https://login.microsoftonline.com/00000000-0000-0000-0000-000000000002/v2.0/.well-known/openid-configuration","metadata":[{"preferred_network":"login.microsoftonline.com","preferred_cache":"login.microsoftonline.com","aliases":["login.microsoftonline.com"]}]}`,
			), jsonHeaders), nil
		case strings.HasSuffix(path, "/openid-configuration"):
			return vhttp.Respond(http.StatusOK, []byte(
				`{"token_endpoint":"https://login.microsoftonline.com/00000000-0000-0000-0000-000000000002/oauth2/v2.0/token","issuer":"https://login.microsoftonline.com/00000000-0000-0000-0000-000000000002/v2.0","authorization_endpoint":"https://login.microsoftonline.com/00000000-0000-0000-0000-000000000002/oauth2/v2.0/authorize"}`,
			), jsonHeaders), nil
		case strings.HasSuffix(path, "/token"):
			return vhttp.Respond(http.StatusOK, []byte(
				`{"token_type":"Bearer","expires_in":3600,"access_token":"test-token"}`,
			), jsonHeaders), nil
		case strings.Contains(path, "/roleAssignments"):
			body := map[string]any{"value": []any{}}
			if rolePresent {
				body["value"] = []any{
					map[string]any{
						"id": "/ra/1",
						"properties": map[string]any{
							"principalId": "11111111-2222-3333-4444-555555555555",
							"roleDefinitionId": "/subscriptions/00000000-0000-0000-0000-000000000000" +
								"/providers/Microsoft.Authorization/roleDefinitions/" +
								azurehelper.RoleStorageBlobDataContributor,
						},
					},
				}
			}

			raw, err := json.Marshal(body)
			if err != nil {
				return nil, err
			}

			return vhttp.Respond(http.StatusOK, raw, jsonHeaders), nil
		case strings.Contains(path, "/Microsoft.Storage/storageAccounts/"):
			return vhttp.Respond(http.StatusOK, []byte(
				`{"name":"tfstate1234","location":"eastus"}`,
			), jsonHeaders), nil
		default:
			// 400 is terminal for the Azure SDK retry policy.
			return vhttp.Respond(http.StatusBadRequest, []byte(
				`{"error":{"code":"UnmatchedTestRequest","message":"`+path+`"}}`,
			), jsonHeaders), nil
		}
	})
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
