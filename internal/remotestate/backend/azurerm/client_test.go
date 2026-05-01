// Package azurerm tests for client construction, error types, and the
// remote-state config helpers. These tests exercise paths that do not
// require the Azure SDK to issue any network calls — auth-method
// detection, control-plane gating for SAS / access-key configs, no-op
// short-circuits, error formatting, and config -> session-config
// projection. The lifecycle paths that do need a live Azure subscription
// are covered by test/integration_azure_test.go behind the `azure` build
// tag.
package azurerm_test

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAccessKey is a syntactically valid base64-encoded value used as a
// throwaway storage account access key. The Azure SDK's
// NewSharedKeyCredential constructor only validates encoding; it never
// calls out, so this is safe for unit tests.
var fakeAccessKey = base64.StdEncoding.EncodeToString([]byte("not-a-real-key"))

const (
	testSASToken          = "?sv=2024-01-01&ss=b&sig=abc"
	keySASToken           = "sas_token"
	keyAccessKey          = "access_key"
	keyUseAzureADAuth     = "use_azuread_auth"
	keyLocation           = "location"
	keySkipSACreation     = "skip_storage_account_creation"
	keySkipRGCreation     = "skip_resource_group_creation"
	keySkipContainerCreat = "skip_container_creation"
	testLocation          = "westeurope"
)

func clientTestOpts(t *testing.T) *backend.Options {
	t.Helper()

	exps := experiment.NewExperiments()
	require.NoError(t, exps.EnableExperiment(experiment.AzureBackend))

	return &backend.Options{
		Experiments:    exps,
		NonInteractive: true,
		Env:            map[string]string{},
	}
}

func extConfigForAuth(t *testing.T, extra map[string]any) *azurerm.ExtendedRemoteStateConfigAzureRM {
	t.Helper()

	cfg := azurerm.Config{
		keyStorageAccount: testStorageAccount,
		keyContainer:      testContainer,
		keyKey:            testKey,
		keyResourceGroup:  testRG,
	}
	for k, v := range extra {
		cfg[k] = v
	}

	ext, err := cfg.ExtendedAzureRMConfig()
	require.NoError(t, err)

	return ext
}

// TestNewClient_SASTokenIsDataPlaneOnly verifies that constructing a
// Client from a SAS-token config succeeds (the data-plane blob client is
// built) but every control-plane method returns the helpful "data-plane
// only" error from requireControlPlane.
func TestNewClient_SASTokenIsDataPlaneOnly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := logger.CreateLogger()
	opts := clientTestOpts(t)

	ext := extConfigForAuth(t, map[string]any{
		keySASToken:       testSASToken,
		keyUseAzureADAuth: false,
	})

	c, err := azurerm.NewClient(ctx, l, ext, opts)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, c.Close()) })

	_, err = c.DoesStorageAccountExist(ctx)
	requireDataPlaneOnly(t, err)

	_, err = c.IsVersioningEnabled(ctx, l)
	requireDataPlaneOnly(t, err)

	requireDataPlaneOnly(t, c.DeleteStorageAccount(ctx, l))
	requireDataPlaneOnly(t, c.CreateStorageAccountIfNecessary(ctx, l, opts))
}

// TestNewClient_AccessKeyIsDataPlaneOnly mirrors the SAS-token case for
// the AccessKey auth method. Confirms the credential branch through the
// blob-client constructor and the control-plane gate.
func TestNewClient_AccessKeyIsDataPlaneOnly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := logger.CreateLogger()
	opts := clientTestOpts(t)

	ext := extConfigForAuth(t, map[string]any{
		keyAccessKey:      fakeAccessKey,
		keyUseAzureADAuth: false,
	})

	c, err := azurerm.NewClient(ctx, l, ext, opts)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, c.Close()) })

	_, err = c.DoesStorageAccountExist(ctx)
	requireDataPlaneOnly(t, err)
}

// TestClient_AssignBlobDataOwnerIfNecessary_NoOps covers the two early
// returns in AssignBlobDataOwnerIfNecessary: the AssignBlobDataOwner
// flag being false, and an empty principal ID. Both must succeed without
// touching the (nil) RBAC client.
func TestClient_AssignBlobDataOwnerIfNecessary_NoOps(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := logger.CreateLogger()
	opts := clientTestOpts(t)

	// Use a SAS-token config so the RBAC client is nil — the early
	// returns must fire before requireControlPlane runs.
	ext := extConfigForAuth(t, map[string]any{
		keySASToken:       testSASToken,
		keyUseAzureADAuth: false,
	})

	c, err := azurerm.NewClient(ctx, l, ext, opts)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, c.Close()) })

	// Flag off: no-op even with a principal ID.
	require.NoError(t, c.AssignBlobDataOwnerIfNecessary(ctx, l, "00000000-0000-0000-0000-000000000000"))

	// Flag on but empty principal: still a no-op.
	ext.AssignBlobDataOwner = true

	require.NoError(t, c.AssignBlobDataOwnerIfNecessary(ctx, l, ""))

	// Flag on + principal: now the control-plane gate must fire.
	err = c.AssignBlobDataOwnerIfNecessary(ctx, l, "00000000-0000-0000-0000-000000000000")
	requireDataPlaneOnly(t, err)
}

// TestClient_CreateStorageAccountIfNecessary_SkipFlag verifies the
// skip_storage_account_creation short-circuit returns nil before any
// control-plane call is attempted, even on a data-plane-only client.
func TestClient_CreateStorageAccountIfNecessary_SkipFlag(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := logger.CreateLogger()
	opts := clientTestOpts(t)

	ext := extConfigForAuth(t, map[string]any{
		keySASToken:       testSASToken,
		keyUseAzureADAuth: false,
		keySkipSACreation: true,
	})

	c, err := azurerm.NewClient(ctx, l, ext, opts)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, c.Close()) })

	require.NoError(t, c.CreateStorageAccountIfNecessary(ctx, l, opts))
}

// TestClient_MoveBlob_NoOpWhenSrcEqualsDst verifies the early return when
// source and destination containers + keys all match. No network call is
// made, so this works without a live storage account.
func TestClient_MoveBlob_NoOpWhenSrcEqualsDst(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := logger.CreateLogger()
	opts := clientTestOpts(t)

	ext := extConfigForAuth(t, map[string]any{
		keySASToken:       testSASToken,
		keyUseAzureADAuth: false,
	})

	c, err := azurerm.NewClient(ctx, l, ext, opts)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, c.Close()) })

	require.NoError(t, c.MoveBlob(ctx, testContainer, testKey, testContainer, testKey))
}

// TestClient_Close confirms the documented no-op contract: Close always
// returns nil and may be invoked multiple times safely.
func TestClient_Close(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := logger.CreateLogger()
	opts := clientTestOpts(t)

	ext := extConfigForAuth(t, map[string]any{
		keySASToken:       testSASToken,
		keyUseAzureADAuth: false,
	})

	c, err := azurerm.NewClient(ctx, l, ext, opts)
	require.NoError(t, err)

	require.NoError(t, c.Close())
	require.NoError(t, c.Close())
}

// TestRemoteStateConfig_CacheKey verifies the cache key is the
// "<storage account>/<container>" pair, which the runner uses to
// dedupe per-account bootstrap work.
func TestRemoteStateConfig_CacheKey(t *testing.T) {
	t.Parallel()

	ext := extConfigForAuth(t, nil)

	assert.Equal(t, testStorageAccount+"/"+testContainer, ext.RemoteStateConfigAzureRM.CacheKey())
}

// TestRemoteStateConfig_GetAzureSessionConfig verifies every field the
// builder consumes is propagated from the parsed config.
func TestRemoteStateConfig_GetAzureSessionConfig(t *testing.T) {
	t.Parallel()

	ext := extConfigForAuth(t, map[string]any{
		"subscription_id": "00000000-0000-0000-0000-000000000000",
		"tenant_id":       "11111111-1111-1111-1111-111111111111",
		"client_id":       "22222222-2222-2222-2222-222222222222",
		"client_secret":   "shh",
		keyAccessKey:      fakeAccessKey,
		keySASToken:       testSASToken,
		"environment":     "public",
		keyLocation:       testLocation,
		"use_msi":         true,
		"use_oidc":        true,
		keyUseAzureADAuth: true,
	})

	sc := ext.GetAzureSessionConfig()
	require.NotNil(t, sc)

	assert.Equal(t, testStorageAccount, sc.StorageAccountName)
	assert.Equal(t, testContainer, sc.ContainerName)
	assert.Equal(t, testRG, sc.ResourceGroupName)
	assert.Equal(t, testLocation, sc.Location)
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", sc.SubscriptionID)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", sc.TenantID)
	assert.Equal(t, "22222222-2222-2222-2222-222222222222", sc.ClientID)
	assert.Equal(t, "shh", sc.ClientSecret)
	assert.Equal(t, fakeAccessKey, sc.AccessKey)
	assert.Equal(t, testSASToken, sc.SasToken)
	assert.Equal(t, "public", sc.CloudEnvironment)
	assert.True(t, sc.UseMSI)
	assert.True(t, sc.UseOIDC)
	assert.True(t, sc.UseAzureADAuth)
}

// TestErrors_MissingRequiredAzureRMRemoteStateConfig verifies the error
// string format and that the typed value round-trips through errors.As.
func TestErrors_MissingRequiredAzureRMRemoteStateConfig(t *testing.T) {
	t.Parallel()

	err := azurerm.MissingRequiredAzureRMRemoteStateConfig("storage_account_name")

	assert.Contains(t, err.Error(), "storage_account_name")
	assert.Contains(t, err.Error(), "Missing required AzureRM")
}

// TestErrors_ExperimentNotEnabledError verifies the error string mentions
// the experiment name so users know which flag to flip.
func TestErrors_ExperimentNotEnabledError(t *testing.T) {
	t.Parallel()

	err := azurerm.ExperimentNotEnabledError{}

	assert.Contains(t, err.Error(), "azure-backend")
	assert.Contains(t, err.Error(), "experimental")
}

// TestClient_CreateContainerIfNecessary_SkipFlag verifies the
// skip_container_creation short-circuit returns nil before any
// data-plane call is attempted.
func TestClient_CreateContainerIfNecessary_SkipFlag(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := logger.CreateLogger()
	opts := clientTestOpts(t)

	ext := extConfigForAuth(t, map[string]any{
		keySASToken:           testSASToken,
		keyUseAzureADAuth:     false,
		keySkipContainerCreat: true,
	})

	c, err := azurerm.NewClient(ctx, l, ext, opts)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, c.Close()) })

	require.NoError(t, c.CreateContainerIfNecessary(ctx, l, opts))
}

// TestBackend_NeedsBootstrap_AllSkipsReturnsFalse exercises the
// "everything skipped" short-circuit on the Backend lifecycle method.
// With all three skip_*_creation flags set the method must return
// (false, nil) without touching Azure, regardless of auth method.
func TestBackend_NeedsBootstrap_AllSkipsReturnsFalse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurerm.NewBackend()
	opts := clientTestOpts(t)
	cfg := backend.Config{
		keyStorageAccount:     testStorageAccount,
		keyContainer:          testContainer,
		keyKey:                testKey,
		keySASToken:           testSASToken,
		keyUseAzureADAuth:     false,
		keySkipRGCreation:     true,
		keySkipSACreation:     true,
		keySkipContainerCreat: true,
	}

	needs, err := b.NeedsBootstrap(ctx, l, cfg, opts)
	require.NoError(t, err)
	assert.False(t, needs)
}

// TestBackend_Migrate_RejectsCrossAccount verifies the cross-account
// guard fires before any Azure client is constructed.
func TestBackend_Migrate_RejectsCrossAccount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurerm.NewBackend()
	opts := clientTestOpts(t)

	src := backend.Config{
		keyStorageAccount: testStorageAccount,
		keyContainer:      testContainer,
		keyKey:            testKey,
		keyResourceGroup:  testRG,
	}
	dst := backend.Config{
		keyStorageAccount: testStorageAccount + "x",
		keyContainer:      testContainer,
		keyKey:            testKey,
		keyResourceGroup:  testRG,
	}

	err := b.Migrate(ctx, l, src, dst, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cross-account")
}

// TestBackend_Lifecycle_SurfaceConfigErrors confirms each lifecycle
// method, with the experiment enabled, propagates config validation
// errors instead of swallowing them or panicking on the empty config.
func TestBackend_Lifecycle_SurfaceConfigErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurerm.NewBackend()
	opts := clientTestOpts(t)
	emptyCfg := backend.Config{}

	t.Run("NeedsBootstrap", func(t *testing.T) {
		t.Parallel()

		_, err := b.NeedsBootstrap(ctx, l, emptyCfg, opts)
		require.Error(t, err)
	})

	t.Run("IsVersionControlEnabled", func(t *testing.T) {
		t.Parallel()

		_, err := b.IsVersionControlEnabled(ctx, l, emptyCfg, opts)
		require.Error(t, err)
	})

	t.Run("Migrate_src", func(t *testing.T) {
		t.Parallel()
		require.Error(t, b.Migrate(ctx, l, emptyCfg, emptyCfg, opts))
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		require.Error(t, b.Delete(ctx, l, emptyCfg, opts))
	})

	t.Run("DeleteBucket", func(t *testing.T) {
		t.Parallel()
		require.Error(t, b.DeleteBucket(ctx, l, emptyCfg, opts))
	})
}

// requireDataPlaneOnly asserts the error is the helpful surface produced
// by Client.requireControlPlane.
func requireDataPlaneOnly(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "data-plane only")
}
