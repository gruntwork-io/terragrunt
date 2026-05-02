//go:build azure

// Package test_test — Azure integration tests.
//
// Build tag `azure` keeps these tests out of the default `go test ./...`
// run because they require live Azure credentials and create real
// resources. Run them explicitly with:
//
//	GOFLAGS='-tags=azure' go test -count=1 -v ./test -run 'TestAzure'
//
// The credentials must be able to create resource groups, storage
// accounts, blob containers, and (for TestAzureBackendBootstrapAssignRBAC)
// role assignments at the storage-account scope.
//
// Required env: ARM_SUBSCRIPTION_ID + DefaultAzureCredential-compatible
// auth (e.g. `az login`, or AZURE_TENANT_ID + AZURE_CLIENT_ID +
// AZURE_CLIENT_SECRET, or workload identity).
//
// Optional env:
//   - TERRAGRUNT_AZURE_TEST_LOCATION (default: eastus)
//   - TERRAGRUNT_AZURE_TEST_PRINCIPAL_ID (skips RBAC test if unset)
//
// Each test reserves an isolated resource group, storage account, and
// container with names derived from the test name + a unique suffix, so
// tests can run in parallel without colliding. Cleanup is best-effort
// and runs on test completion regardless of pass/fail.
package test_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	azurermbackend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

const (
	azureTestEnvSubscription = "ARM_SUBSCRIPTION_ID"
	azureTestEnvLocation     = "TERRAGRUNT_AZURE_TEST_LOCATION"
	azureTestEnvPrincipalID  = "TERRAGRUNT_AZURE_TEST_PRINCIPAL_ID"
	azureTestDefaultLocation = "eastus"

	azureTestStateBlobKey = "integration/terraform.tfstate"
)

// azureTestEnv captures shared per-process Azure test settings.
type azureTestEnv struct {
	SubscriptionID string
	Location       string
}

// loadAzureTestEnv returns the shared test environment, or skips the test
// if mandatory variables are missing.
func loadAzureTestEnv(t *testing.T) azureTestEnv {
	t.Helper()

	sub := os.Getenv(azureTestEnvSubscription)
	if sub == "" {
		t.Skipf("Skipping Azure integration test: %s is not set", azureTestEnvSubscription)
	}

	loc := os.Getenv(azureTestEnvLocation)
	if loc == "" {
		loc = azureTestDefaultLocation
	}

	return azureTestEnv{SubscriptionID: sub, Location: loc}
}

// azureTestResources holds the per-test isolated resource names.
type azureTestResources struct {
	ResourceGroup  string
	StorageAccount string
	Container      string
}

// reserveAzureResources allocates unique-per-test names but does NOT create
// any Azure resources. Cleanup is registered with t.Cleanup so the resource
// group (and any storage account / container under it) is deleted when the
// test exits, regardless of which test phases ran.
func reserveAzureResources(t *testing.T, env azureTestEnv) azureTestResources {
	t.Helper()

	suffix := strings.ToLower(strings.ReplaceAll(uuid.NewString(), "-", ""))[:10]

	rg := "tg-it-" + sanitizeRGName(t.Name()) + "-" + suffix
	if len(rg) > 90 {
		rg = rg[:90]
	}

	sa := "tgit" + suffix + sanitizeSAName(t.Name())
	if len(sa) > 24 {
		sa = sa[:24]
	}

	container := "tgit-" + suffix
	if len(container) > 63 {
		container = container[:63]
	}

	res := azureTestResources{
		ResourceGroup:  rg,
		StorageAccount: sa,
		Container:      container,
	}

	t.Cleanup(func() {
		cleanupAzureResources(t, env, res)
	})

	return res
}

// cleanupAzureResources tears down any resources still present in the
// reserved resource group. Errors are logged but never fail the test.
func cleanupAzureResources(t *testing.T, env azureTestEnv, res azureTestResources) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	azCfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			SubscriptionID:     env.SubscriptionID,
			ResourceGroupName:  res.ResourceGroup,
			StorageAccountName: res.StorageAccount,
			Location:           env.Location,
			UseAzureADAuth:     true,
		}).
		Build(ctx, logger.CreateLogger())
	if err != nil {
		t.Logf("[cleanup %s] build azure config: %v", t.Name(), err)
		return
	}

	rgClient, err := azurehelper.NewResourceGroupClient(azCfg)
	if err != nil {
		t.Logf("[cleanup %s] resource group client: %v", t.Name(), err)
		return
	}

	if err := rgClient.Delete(ctx, logger.CreateLogger(), res.ResourceGroup); err != nil {
		t.Logf("[cleanup %s] delete resource group %q: %v", t.Name(), res.ResourceGroup, err)
		return
	}

	t.Logf("[cleanup %s] deleted resource group %q", t.Name(), res.ResourceGroup)
}

// sanitizeRGName lowercases and dash-encodes a Go test name so it can be
// embedded in an Azure resource group name.
func sanitizeRGName(name string) string {
	out := strings.ToLower(name)
	out = strings.ReplaceAll(out, "/", "-")
	out = strings.ReplaceAll(out, "_", "-")

	out = strings.Trim(out, "-")
	if len(out) > 40 {
		out = out[:40]
	}

	return out
}

// sanitizeSAName lowercases a Go test name and strips everything except
// [a-z0-9] so it can be embedded in a storage account name.
func sanitizeSAName(name string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}

	return b.String()
}

// azureBackendOpts builds backend.Options with the azure-backend
// experiment enabled, non-interactive prompts, and discarded writers.
func azureBackendOpts(t *testing.T) *backend.Options {
	t.Helper()

	exps := experiment.NewExperiments()
	require.NoError(t, exps.EnableExperiment(experiment.AzureBackend))

	return &backend.Options{
		Experiments:    exps,
		NonInteractive: true,
		Env:            map[string]string{},
		Writers: writer.Writers{
			Writer:    io.Discard,
			ErrWriter: io.Discard,
		},
	}
}

// azureBackendOptsDisabled returns options with the azure-backend
// experiment off, used to verify the experiment gate.
func azureBackendOptsDisabled() *backend.Options {
	return &backend.Options{
		Experiments:    experiment.NewExperiments(),
		NonInteractive: true,
		Env:            map[string]string{},
		Writers: writer.Writers{
			Writer:    io.Discard,
			ErrWriter: io.Discard,
		},
	}
}

// azureBackendConfig assembles a backend.Config map from env + reserved
// resources, optionally overlaid with extra keys.
func azureBackendConfig(env azureTestEnv, res azureTestResources, extra map[string]any) backend.Config {
	cfg := backend.Config{
		"storage_account_name": res.StorageAccount,
		"container_name":       res.Container,
		"resource_group_name":  res.ResourceGroup,
		"key":                  azureTestStateBlobKey,
		"subscription_id":      env.SubscriptionID,
		"location":             env.Location,
		"use_azuread_auth":     true,
	}

	for k, v := range extra {
		cfg[k] = v
	}

	return cfg
}

// fetchAzureAccessKey returns the primary access key of the reserved
// storage account. The signed-in identity needs Microsoft.Storage/
// storageAccounts/listKeys (granted by subscription Owner) — it does NOT
// need any Storage Blob Data role. Used by data-plane lifecycle tests
// (Delete, Migrate) so they can run for principals that have control
// plane Owner but no data role.
func fetchAzureAccessKey(t *testing.T, env azureTestEnv, res azureTestResources) string {
	t.Helper()

	ctx := context.Background()
	azCfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			SubscriptionID:     env.SubscriptionID,
			ResourceGroupName:  res.ResourceGroup,
			StorageAccountName: res.StorageAccount,
			Location:           env.Location,
			UseAzureADAuth:     true,
		}).
		Build(ctx, logger.CreateLogger())
	require.NoError(t, err)

	saClient, err := azurehelper.NewStorageAccountClient(azCfg)
	require.NoError(t, err)

	keys, err := saClient.GetKeys(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, keys)

	return keys[0]
}

// newAzureBlobClient constructs a data-plane blob client targeting the
// reserved storage account (used by tests for direct fixture upload /
// inspection).
//
// The client authenticates with the storage account's primary access
// key rather than the AAD identity that runs the test, because Azure
// RBAC for blob data plane requires a Storage Blob Data Contributor /
// Owner role on the storage account scope — subscription-level Owner
// alone is not sufficient. Test runners (and CI) typically have control
// plane Owner but no data role; fetching the access key via the
// management API (which Owner can do) sidesteps this entirely.
func newAzureBlobClient(t *testing.T, env azureTestEnv, res azureTestResources) *azurehelper.BlobClient {
	t.Helper()

	ctx := context.Background()
	l := logger.CreateLogger()

	keyCfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			SubscriptionID:     env.SubscriptionID,
			ResourceGroupName:  res.ResourceGroup,
			StorageAccountName: res.StorageAccount,
			Location:           env.Location,
			UseAzureADAuth:     true,
		}).
		Build(ctx, l)
	require.NoError(t, err)

	saClient, err := azurehelper.NewStorageAccountClient(keyCfg)
	require.NoError(t, err)

	keys, err := saClient.GetKeys(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, keys, "storage account returned no access keys")

	azCfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			SubscriptionID:     env.SubscriptionID,
			ResourceGroupName:  res.ResourceGroup,
			StorageAccountName: res.StorageAccount,
			Location:           env.Location,
			AccessKey:          keys[0],
		}).
		Build(ctx, l)
	require.NoError(t, err)

	bc, err := azurehelper.NewBlobClient(ctx, azCfg, "")
	require.NoError(t, err)

	return bc
}

// TestAzureBackendLifecycleGatedByExperiment confirms that, against a real
// subscription with valid credentials, every lifecycle method still
// refuses to do anything when the azure-backend experiment is disabled.
// This complements the unit-level gating tests by proving no Azure SDK
// call escapes the gate even when credentials would otherwise succeed.
func TestAzureBackendLifecycleGatedByExperiment(t *testing.T) {
	t.Parallel()

	env := loadAzureTestEnv(t)
	res := reserveAzureResources(t, env)

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurermbackend.NewBackend()
	cfg := azureBackendConfig(env, res, nil)
	opts := azureBackendOptsDisabled()

	_, err := b.NeedsBootstrap(ctx, l, cfg, opts)
	assertExperimentDisabled(t, err)
	assertExperimentDisabled(t, b.Bootstrap(ctx, l, cfg, opts))
	_, err = b.IsVersionControlEnabled(ctx, l, cfg, opts)
	assertExperimentDisabled(t, err)
	assertExperimentDisabled(t, b.Migrate(ctx, l, cfg, cfg, opts))
	assertExperimentDisabled(t, b.Delete(ctx, l, cfg, opts))
	assertExperimentDisabled(t, b.DeleteBucket(ctx, l, cfg, opts))
}

func assertExperimentDisabled(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental")
}

// TestAzureBackendBootstrap exercises Bootstrap end to end: resource group
// creation, storage account creation (with versioning), container
// creation, NeedsBootstrap idempotency, and IsVersionControlEnabled.
func TestAzureBackendBootstrap(t *testing.T) {
	t.Parallel()

	env := loadAzureTestEnv(t)
	res := reserveAzureResources(t, env)

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurermbackend.NewBackend()
	cfg := azureBackendConfig(env, res, nil)
	opts := azureBackendOpts(t)

	needs, err := b.NeedsBootstrap(ctx, l, cfg, opts)
	require.NoError(t, err)
	require.True(t, needs, "fresh storage account should need bootstrap")

	require.NoError(t, b.Bootstrap(ctx, l, cfg, opts))

	versioning, err := b.IsVersionControlEnabled(ctx, l, cfg, opts)
	require.NoError(t, err)
	assert.True(t, versioning, "Bootstrap should leave blob versioning enabled by default")

	needs, err = b.NeedsBootstrap(ctx, l, cfg, opts)
	require.NoError(t, err)
	assert.False(t, needs, "second NeedsBootstrap after Bootstrap should be false")
}

// TestAzureBackendBootstrapSoftDelete confirms enable_soft_delete is
// honoured during Bootstrap and that the configured retention lands on
// both the blob and container delete-retention policies.
func TestAzureBackendBootstrapSoftDelete(t *testing.T) {
	t.Parallel()

	env := loadAzureTestEnv(t)
	res := reserveAzureResources(t, env)

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurermbackend.NewBackend()

	const retentionDays = 14

	cfg := azureBackendConfig(env, res, map[string]any{
		"enable_soft_delete":         true,
		"soft_delete_retention_days": retentionDays,
	})

	require.NoError(t, b.Bootstrap(ctx, l, cfg, azureBackendOpts(t)))

	// Read back the soft-delete policy via the management API to prove
	// the configured retention actually landed on the storage account.
	azCfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			SubscriptionID:     env.SubscriptionID,
			ResourceGroupName:  res.ResourceGroup,
			StorageAccountName: res.StorageAccount,
			Location:           env.Location,
			UseAzureADAuth:     true,
		}).
		Build(ctx, l)
	require.NoError(t, err)

	saClient, err := azurehelper.NewStorageAccountClient(azCfg)
	require.NoError(t, err)

	policy, err := saClient.GetSoftDeletePolicy(ctx)
	require.NoError(t, err)
	assert.True(t, policy.BlobEnabled, "blob soft-delete should be enabled")
	assert.True(t, policy.ContainerEnabled, "container soft-delete should be enabled")
	assert.Equal(t, int32(retentionDays), policy.BlobRetentionDays, "blob retention days should match config")
	assert.Equal(t, int32(retentionDays), policy.ContainerRetentionDays, "container retention days should match config")
}

// TestAzureBackendBootstrapIdempotent confirms that a second Bootstrap
// call against an already-bootstrapped account is a successful no-op:
// the in-memory IsConfigInited cache short-circuits before any Azure
// API call, and a fresh Backend instance (cold cache) re-detects the
// existing resources and exits cleanly without trying to recreate them.
func TestAzureBackendBootstrapIdempotent(t *testing.T) {
	t.Parallel()

	env := loadAzureTestEnv(t)
	res := reserveAzureResources(t, env)

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurermbackend.NewBackend()
	cfg := azureBackendConfig(env, res, nil)
	opts := azureBackendOpts(t)

	require.NoError(t, b.Bootstrap(ctx, l, cfg, opts))

	// Same Backend instance: hits the IsConfigInited cache.
	require.NoError(t, b.Bootstrap(ctx, l, cfg, opts), "second Bootstrap on same instance must be a no-op")

	// Fresh Backend instance: cache is cold, so this exercises the
	// real "resource already exists" branches in
	// CreateStorageAccountIfNecessary / CreateContainerIfNecessary.
	b2 := azurermbackend.NewBackend()
	require.NoError(t, b2.Bootstrap(ctx, l, cfg, opts), "third Bootstrap on a fresh Backend must detect existing resources and succeed")

	needs, err := b2.NeedsBootstrap(ctx, l, cfg, opts)
	require.NoError(t, err)
	assert.False(t, needs, "NeedsBootstrap must be false after a successful re-bootstrap on a fresh Backend")
}

// TestAzureBackendBootstrapAssignRBAC exercises the
// assign_storage_blob_data_owner code path. Skipped unless the operator
// supplies a principal ID via TERRAGRUNT_AZURE_TEST_PRINCIPAL_ID.
func TestAzureBackendBootstrapAssignRBAC(t *testing.T) {
	t.Parallel()

	principal := os.Getenv(azureTestEnvPrincipalID)
	if principal == "" {
		t.Skipf("Skipping: %s not set", azureTestEnvPrincipalID)
	}

	env := loadAzureTestEnv(t)
	res := reserveAzureResources(t, env)

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurermbackend.NewBackend()
	opts := azureBackendOpts(t)
	cfg := azureBackendConfig(env, res, map[string]any{
		"assign_storage_blob_data_owner": true,
	})

	require.NoError(t, b.Bootstrap(ctx, l, cfg, opts))

	// Backend.Bootstrap does not call AssignBlobDataOwnerIfNecessary —
	// that is invoked separately by the runner — so we drive it directly
	// through Client to cover the code path explicitly.
	extCfg, err := azurermbackend.Config(cfg).ExtendedAzureRMConfig()
	require.NoError(t, err)

	client, err := azurermbackend.NewClient(ctx, l, extCfg, opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.AssignBlobDataOwnerIfNecessary(ctx, l, principal))

	// Idempotency: a second call must succeed (HasRoleAssignment short-circuit).
	require.NoError(t, client.AssignBlobDataOwnerIfNecessary(ctx, l, principal))
}

// TestAzureBackendBootstrapSkipFlags verifies that
// skip_resource_group_creation + skip_storage_account_creation are
// honoured: against a pre-bootstrapped account we may opt out of all ARM
// operations and still reach a "no bootstrap needed" state for the
// container.
func TestAzureBackendBootstrapSkipFlags(t *testing.T) {
	t.Parallel()

	env := loadAzureTestEnv(t)
	res := reserveAzureResources(t, env)

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurermbackend.NewBackend()
	opts := azureBackendOpts(t)

	// Bootstrap once with full creation to seed RG + SA + container.
	require.NoError(t, b.Bootstrap(ctx, l, azureBackendConfig(env, res, nil), opts))

	// Re-bootstrap with skip flags set: must be a no-op and must not error.
	skipCfg := azureBackendConfig(env, res, map[string]any{
		"skip_resource_group_creation":  true,
		"skip_storage_account_creation": true,
		"skip_container_creation":       true,
	})

	needs, err := b.NeedsBootstrap(ctx, l, skipCfg, opts)
	require.NoError(t, err)
	assert.False(t, needs, "all-skip config should never need bootstrap")

	require.NoError(t, b.Bootstrap(ctx, l, skipCfg, opts))
}

// TestAzureBackendMigrate verifies Migrate copies a state blob from the
// source key to the destination key within the same storage account and
// removes the source blob.
func TestAzureBackendMigrate(t *testing.T) {
	t.Parallel()

	env := loadAzureTestEnv(t)
	res := reserveAzureResources(t, env)

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurermbackend.NewBackend()
	opts := azureBackendOpts(t)

	srcCfg := azureBackendConfig(env, res, map[string]any{"key": "src/terraform.tfstate"})

	require.NoError(t, b.Bootstrap(ctx, l, srcCfg, opts))

	// Backend bootstrap is done with AAD (control plane); switch the
	// data-plane lifecycle ops to access-key auth so the test runner
	// does not need a Storage Blob Data role on the storage account.
	key := fetchAzureAccessKey(t, env, res)
	srcCfg = azureBackendConfig(env, res, map[string]any{"key": "src/terraform.tfstate", "access_key": key, "use_azuread_auth": false})
	dstCfg := azureBackendConfig(env, res, map[string]any{"key": "dst/terraform.tfstate", "access_key": key, "use_azuread_auth": false})

	// Seed a fake state blob at the source key directly via the data plane.
	bc := newAzureBlobClient(t, env, res)
	require.NoError(t, bc.PutBlob(ctx, res.Container, "src/terraform.tfstate", []byte("{\"version\":4}")))

	require.NoError(t, b.Migrate(ctx, l, srcCfg, dstCfg, opts))

	// Source must be gone, destination must contain the seeded payload.
	src, err := bc.GetBlob(ctx, res.Container, "src/terraform.tfstate")
	if err == nil {
		_ = src.Close()

		t.Fatalf("source blob still exists after migrate")
	}

	dst, err := bc.GetBlob(ctx, res.Container, "dst/terraform.tfstate")
	require.NoError(t, err)
	t.Cleanup(func() { _ = dst.Close() })

	body, err := io.ReadAll(dst)
	require.NoError(t, err)
	assert.Equal(t, "{\"version\":4}", string(body))
}

// TestAzureBackendMigrateRejectsCrossAccount verifies that Migrate refuses
// to operate when the source and destination storage accounts differ.
func TestAzureBackendMigrateRejectsCrossAccount(t *testing.T) {
	t.Parallel()

	env := loadAzureTestEnv(t)
	res := reserveAzureResources(t, env)

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurermbackend.NewBackend()
	opts := azureBackendOpts(t)

	srcCfg := azureBackendConfig(env, res, nil)
	dstCfg := azureBackendConfig(env, res, map[string]any{"storage_account_name": res.StorageAccount + "x"})

	err := b.Migrate(ctx, l, srcCfg, dstCfg, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cross-account")
}

// TestAzureBackendDelete verifies Delete removes the configured state
// blob but leaves the container intact.
func TestAzureBackendDelete(t *testing.T) {
	t.Parallel()

	env := loadAzureTestEnv(t)
	res := reserveAzureResources(t, env)

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurermbackend.NewBackend()
	opts := azureBackendOpts(t)
	cfg := azureBackendConfig(env, res, nil)

	require.NoError(t, b.Bootstrap(ctx, l, cfg, opts))

	// Switch to access-key for the data-plane Delete so the test runner
	// does not need a Storage Blob Data role on the storage account.
	cfg = azureBackendConfig(env, res, map[string]any{"access_key": fetchAzureAccessKey(t, env, res), "use_azuread_auth": false})

	bc := newAzureBlobClient(t, env, res)
	require.NoError(t, bc.PutBlob(ctx, res.Container, azureTestStateBlobKey, []byte("{}")))

	require.NoError(t, b.Delete(ctx, l, cfg, opts))

	if rc, err := bc.GetBlob(ctx, res.Container, azureTestStateBlobKey); err == nil {
		_ = rc.Close()

		t.Fatalf("blob still present after Delete")
	}

	exists, err := bc.ContainerExists(ctx, res.Container)
	require.NoError(t, err)
	assert.True(t, exists, "container must survive Delete")
}

// TestAzureBackendDeleteBucket verifies DeleteBucket removes both the
// container and the storage account.
func TestAzureBackendDeleteBucket(t *testing.T) {
	t.Parallel()

	env := loadAzureTestEnv(t)
	res := reserveAzureResources(t, env)

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurermbackend.NewBackend()
	opts := azureBackendOpts(t)
	cfg := azureBackendConfig(env, res, nil)

	require.NoError(t, b.Bootstrap(ctx, l, cfg, opts))

	require.NoError(t, b.DeleteBucket(ctx, l, cfg, opts))

	// Storage account must be gone.
	azCfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			SubscriptionID:     env.SubscriptionID,
			ResourceGroupName:  res.ResourceGroup,
			StorageAccountName: res.StorageAccount,
			Location:           env.Location,
			UseAzureADAuth:     true,
		}).
		Build(ctx, l)
	require.NoError(t, err)

	saClient, err := azurehelper.NewStorageAccountClient(azCfg)
	require.NoError(t, err)

	exists, err := saClient.Exists(ctx)
	require.NoError(t, err)
	assert.False(t, exists, "storage account must be deleted by DeleteBucket")
}

// TestAzureSASTokenAuthIsDataPlaneOnly verifies that a SAS-token config
// cannot reach control-plane operations: NewClient succeeds (data-plane
// only) but Bootstrap surfaces the helpful error from requireControlPlane.
func TestAzureSASTokenAuthIsDataPlaneOnly(t *testing.T) {
	t.Parallel()

	env := loadAzureTestEnv(t)
	res := reserveAzureResources(t, env)

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurermbackend.NewBackend()
	opts := azureBackendOpts(t)

	cfg := azureBackendConfig(env, res, map[string]any{
		"sas_token":        "?sv=2024-01-01&ss=b&srt=sco&sp=rwdlacx&se=2099-12-31T00:00:00Z&sig=" + uuid.NewString(),
		"use_azuread_auth": false,
	})

	err := b.Bootstrap(ctx, l, cfg, opts)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "data-plane")
}
