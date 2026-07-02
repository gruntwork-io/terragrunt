//go:build azure

// Package test_test — Azure backend end-to-end + matrix tests.
//
// This file complements integration_azure_test.go (which covers the
// SDK / control-plane surface) by exercising the azurerm backend via
// the full Terragrunt CLI on real fixtures, plus a NeedsBootstrap
// state-machine matrix and a blob-lease-based state lock contention
// test.
//
// All tests are gated by build tag `azure` and skip when
// ARM_SUBSCRIPTION_ID is unset. Each test reserves an isolated
// resource group / storage account / container; cleanup is best-effort
// via t.Cleanup in reserveAzureResources.
package test_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/lease"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	azurermbackend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

const (
	testFixtureAzureBackend            = "fixtures/azure-backend"
	testFixtureAzureBackendUnit        = "fixtures/azure-backend/unit"
	testFixtureAzureBackendDirectState = "fixtures/azure-backend/direct-state"
)

type azureE2EAuthMode struct {
	UseAccessKey   bool
	AccessKey      string
	ResourceGroup  string
	StorageAccount string
}

// loadAzureE2EAuthMode enables a local-friendly data-plane mode when
// ARM_ACCESS_KEY + ARM_ACCESS_KEY_SA + ARM_ACCESS_KEY_RG are set.
// In that mode tests reuse the provided account and skip control-plane
// storage-account creation so no Blob RBAC is required for the caller.
func loadAzureE2EAuthMode() azureE2EAuthMode {
	accessKey := os.Getenv(azureTestEnvAccessKey)
	sa := os.Getenv(azureTestEnvAccessKeySA)
	rg := os.Getenv(azureTestEnvAccessKeyRG)

	if accessKey != "" && sa != "" && rg != "" {
		return azureE2EAuthMode{
			UseAccessKey:   true,
			AccessKey:      accessKey,
			ResourceGroup:  rg,
			StorageAccount: sa,
		}
	}

	return azureE2EAuthMode{}
}

func reserveAzureE2EResources(t *testing.T, env azureTestEnv, mode azureE2EAuthMode) azureTestResources {
	t.Helper()

	if !mode.UseAccessKey {
		return reserveAzureResources(t, env)
	}

	suffix := strings.ToLower(strings.ReplaceAll(uuid.NewString(), "-", ""))[:10]
	res := azureTestResources{
		ResourceGroup:  mode.ResourceGroup,
		StorageAccount: mode.StorageAccount,
		Container:      "tgit-" + suffix,
	}

	blob := newAzureBlobClientWithAccessKey(t, env, res, mode.AccessKey)
	t.Cleanup(func() {
		if err := blob.EnsureContainerDeleted(context.Background(), res.Container); err != nil {
			t.Logf("[cleanup %s] delete container %q: %v", t.Name(), res.Container, err)
		}
	})

	return res
}

func newAzureBlobClientWithAccessKey(t *testing.T, env azureTestEnv, res azureTestResources, accessKey string) *azurehelper.BlobClient {
	t.Helper()

	ctx := context.Background()
	azCfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			SubscriptionID:     env.SubscriptionID,
			ResourceGroupName:  res.ResourceGroup,
			StorageAccountName: res.StorageAccount,
			Location:           env.Location,
			AccessKey:          accessKey,
		}).
		Build(ctx, logger.CreateLogger())
	require.NoError(t, err)

	b, err := azurehelper.NewBlobClient(azCfg)
	require.NoError(t, err)

	return b
}

func configureAzureFixtureRoot(t *testing.T, rootHCL string, env azureTestEnv, res azureTestResources, mode azureE2EAuthMode) {
	t.Helper()

	if !mode.UseAccessKey {
		fillAzureFixturePlaceholders(t, rootHCL, rootHCL, env, res)
		return
	}

	contents := fmt.Sprintf(`remote_state {
  backend = "azurerm"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    storage_account_name          = %q
    container_name                = %q
    resource_group_name           = %q
    subscription_id               = %q
    location                      = %q
    access_key                    = %q
    use_azuread_auth              = false
    skip_resource_group_creation  = true
    skip_storage_account_creation = true
    key                           = "${path_relative_to_include()}/terraform.tfstate"
  }
}
`, res.StorageAccount, res.Container, res.ResourceGroup, env.SubscriptionID, env.Location, mode.AccessKey)

	require.NoError(t, os.WriteFile(rootHCL, []byte(contents), 0o644))
}

// fillAzureFixturePlaceholders rewrites the placeholders in a root.hcl
// (or any other fixture file) so it points at the per-test reserved
// resources. Returns the path it wrote to (same as dest).
func fillAzureFixturePlaceholders(t *testing.T, src, dest string, env azureTestEnv, res azureTestResources) string {
	t.Helper()

	helpers.CopyAndFillMapPlaceholders(t, src, dest, map[string]string{
		"__FILL_IN_STORAGE_ACCOUNT__": res.StorageAccount,
		"__FILL_IN_CONTAINER__":       res.Container,
		"__FILL_IN_RESOURCE_GROUP__":  res.ResourceGroup,
		"__FILL_IN_SUBSCRIPTION_ID__": env.SubscriptionID,
		"__FILL_IN_LOCATION__":        env.Location,
	})

	return dest
}

// TestAzureBackendCLIInitApplyState runs `terragrunt run -- apply` end
// to end against the azurerm backend on a real fixture and verifies
// that:
//   - the storage account + container are bootstrapped,
//   - terraform state is written to the configured blob key,
//   - the blob is valid JSON tfstate containing the expected outputs.
//
// This is the only test that exercises the CLI → backend wiring (init
// args, generate "backend.tf", bootstrap before init), so a regression
// in any of those layers will show up here even though every layer
// has unit / SDK coverage individually.
func TestAzureBackendCLIInitApplyState(t *testing.T) {
	t.Parallel()

	env := loadAzureTestEnv(t)
	mode := loadAzureE2EAuthMode()
	res := reserveAzureE2EResources(t, env, mode)

	helpers.CleanupTerraformFolder(t, testFixtureAzureBackend)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAzureBackend)
	rootPath := filepath.Join(tmpEnvPath, testFixtureAzureBackend)

	rootHCL := filepath.Join(rootPath, "root.hcl")
	configureAzureFixtureRoot(t, rootHCL, env, res, mode)

	unitDir := filepath.Join(rootPath, "unit")

	cmd := fmt.Sprintf(
		"terragrunt --experiment azure-backend run --backend-bootstrap --non-interactive --log-level debug --working-dir %s -- apply -auto-approve",
		unitDir,
	)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err, "apply failed.\nstdout:\n%s\nstderr:\n%s", stdout, stderr)

	// State blob is at "<path_relative_to_include()>/terraform.tfstate"
	// which for unit/ resolves to "unit/terraform.tfstate".
	expectedKey := "unit/terraform.tfstate"

	ctx := context.Background()
	var blob *azurehelper.BlobClient
	if mode.UseAccessKey {
		blob = newAzureBlobClientWithAccessKey(t, env, res, mode.AccessKey)
	} else {
		blob = newAzureBlobClient(t, env, res)
	}

	names, err := blob.ListBlobs(ctx, res.Container)
	require.NoError(t, err)
	require.Contains(t, names, expectedKey, "state blob %q missing from container; saw %v", expectedKey, names)

	rc, err := blob.GetBlob(ctx, res.Container, expectedKey)
	require.NoError(t, err)
	raw, err := io.ReadAll(rc)
	require.NoError(t, rc.Close())
	require.NoError(t, err)

	var stateDoc struct {
		Outputs map[string]struct {
			Value any `json:"value"`
		} `json:"outputs"`
	}
	require.NoError(t, json.Unmarshal(raw, &stateDoc), "state blob is not valid JSON: %s", string(raw))

	msg, ok := stateDoc.Outputs["message"]
	require.True(t, ok, "expected output 'message' in state; outputs=%v", stateDoc.Outputs)
	assert.Equal(t, "hello from unit", msg.Value, "output value round-trip via azurerm backend")
}

// TestAzureBackendDirectStateLookup exercises the data-plane read path
// used by terragrunt dependency resolution with
// `--dependency-fetch-output-from-state`: a downstream module reads
// its upstream's outputs directly from the state blob, bypassing
// `tofu output`. This validates that the blob client constructed for
// dependency reads can authenticate, locate, and parse a state blob
// produced by an unrelated apply.
func TestAzureBackendDirectStateLookup(t *testing.T) {
	t.Parallel()

	env := loadAzureTestEnv(t)
	mode := loadAzureE2EAuthMode()
	res := reserveAzureE2EResources(t, env, mode)

	helpers.CleanupTerraformFolder(t, testFixtureAzureBackend)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAzureBackend)
	rootPath := filepath.Join(tmpEnvPath, testFixtureAzureBackend)

	rootHCL := filepath.Join(rootPath, "root.hcl")
	configureAzureFixtureRoot(t, rootHCL, env, res, mode)

	directRoot := filepath.Join(rootPath, "direct-state")

	// Apply the producer module first.
	app1Cmd := fmt.Sprintf(
		"terragrunt --experiment azure-backend run --backend-bootstrap --non-interactive --working-dir %s -- apply -auto-approve",
		filepath.Join(directRoot, "app1"),
	)
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, app1Cmd)
	require.NoError(t, err, "app1 apply failed.\nstdout:\n%s\nstderr:\n%s", stdout, stderr)

	// Apply the consumer; with --dependency-fetch-output-from-state the
	// dependency block on app1 must resolve by reading app1's state
	// blob directly (not by re-running `tofu output`).
	app2Cmd := fmt.Sprintf(
		"terragrunt --experiment azure-backend run --backend-bootstrap --dependency-fetch-output-from-state --non-interactive --working-dir %s -- apply -auto-approve",
		filepath.Join(directRoot, "app2"),
	)
	stdout, stderr, err = helpers.RunTerragruntCommandWithOutput(t, app2Cmd)
	require.NoError(t, err, "app2 apply failed.\nstdout:\n%s\nstderr:\n%s", stdout, stderr)

	// Re-fetch app2 outputs and assert the upstream value reached the
	// downstream module — proves the data-plane state read worked.
	outputCmd := fmt.Sprintf(
		"terragrunt --experiment azure-backend output --dependency-fetch-output-from-state --non-interactive --working-dir %s app2_text",
		filepath.Join(directRoot, "app2"),
	)
	stdout, stderr, err = helpers.RunTerragruntCommandWithOutput(t, outputCmd)
	require.NoError(t, err, "app2 output failed.\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	assert.Contains(t, stdout, "app2 saw: app1 output",
		"app2 output should reflect app1's value read from the azurerm state blob")
}

// TestAzureBackendStateLockContention verifies that the azurerm
// backend honours blob leases as the state lock: while one process
// holds a lease on the state blob, a second `terraform apply` must
// fail with a lock error rather than racing.
//
// We acquire the lease directly via the Azure SDK (skipping
// Terragrunt) so the contention is deterministic and does not require
// orchestrating two CLI invocations.
func TestAzureBackendStateLockContention(t *testing.T) {
	t.Parallel()

	env := loadAzureTestEnv(t)
	mode := loadAzureE2EAuthMode()
	res := reserveAzureE2EResources(t, env, mode)

	helpers.CleanupTerraformFolder(t, testFixtureAzureBackend)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAzureBackend)
	rootPath := filepath.Join(tmpEnvPath, testFixtureAzureBackend)

	rootHCL := filepath.Join(rootPath, "root.hcl")
	configureAzureFixtureRoot(t, rootHCL, env, res, mode)

	unitDir := filepath.Join(rootPath, "unit")

	// First apply: bootstrap + create state blob.
	firstCmd := fmt.Sprintf(
		"terragrunt --experiment azure-backend run --backend-bootstrap --non-interactive --working-dir %s -- apply -auto-approve",
		unitDir,
	)
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, firstCmd)
	require.NoError(t, err, "initial apply failed.\nstdout:\n%s\nstderr:\n%s", stdout, stderr)

	const stateKey = "unit/terraform.tfstate"

	// Acquire an infinite-duration lease on the state blob via the SDK.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var blob *azurehelper.BlobClient
	if mode.UseAccessKey {
		blob = newAzureBlobClientWithAccessKey(t, env, res, mode.AccessKey)
	} else {
		blob = newAzureBlobClient(t, env, res)
	}
	blobLeaseClient, err := lease.NewBlobClient(
		blob.AzClient().ServiceClient().NewContainerClient(res.Container).NewBlobClient(stateKey),
		nil,
	)
	require.NoError(t, err, "construct blob lease client")

	leaseResp, err := blobLeaseClient.AcquireLease(ctx, -1, nil) // -1 = infinite duration
	require.NoError(t, err, "acquire blob lease")
	require.NotNil(t, leaseResp.LeaseID, "lease id should be returned")

	// Ensure the lease is released even if the test fails.
	t.Cleanup(func() {
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 1*time.Minute)
		defer releaseCancel()

		if _, err := blobLeaseClient.ReleaseLease(releaseCtx, nil); err != nil {
			t.Logf("[cleanup %s] release blob lease: %v", t.Name(), err)
		}
	})

	// Second apply must fail because the lease is held.
	secondCmd := fmt.Sprintf(
		"terragrunt --experiment azure-backend run --non-interactive --working-dir %s -- apply -auto-approve -lock-timeout=10s",
		unitDir,
	)
	stdout, stderr, err = helpers.RunTerragruntCommandWithOutput(t, secondCmd)
	require.Error(t, err, "second apply should fail while lease is held.\nstdout:\n%s\nstderr:\n%s", stdout, stderr)

	combined := strings.ToLower(stdout + "\n" + stderr)
	assert.True(t,
		strings.Contains(combined, "lease") ||
			strings.Contains(combined, "lock") ||
			strings.Contains(combined, "conflict"),
		"second apply error should reference the lock/lease.\nstdout:\n%s\nstderr:\n%s", stdout, stderr,
	)
}

// TestAzureBackendNeedsBootstrapMatrix table-drives NeedsBootstrap
// across the state-machine the backend implements: account missing,
// container missing, everything present, and full passthrough.
//
// One reserved resource set is reused across sub-tests in sequence so
// the test exercises real Bootstrap → mutate → re-check transitions
// (cheaper and more realistic than re-bootstrapping per case).
func TestAzureBackendNeedsBootstrapMatrix(t *testing.T) {
	t.Parallel()
	skipIfAccessKeyMode(t)

	env := loadAzureTestEnv(t)
	res := reserveAzureResources(t, env)

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurermbackend.NewBackend()
	opts := azureBackendOpts(t)
	cfg := azureBackendConfig(env, res, nil)

	// 1) Nothing exists yet — needs bootstrap.
	needs, err := b.NeedsBootstrap(ctx, l, cfg, opts)
	require.NoError(t, err)
	require.True(t, needs, "fresh reservation should require bootstrap")

	// 2) Bootstrap everything, then re-check — must be idempotent false.
	require.NoError(t, b.Bootstrap(ctx, l, cfg, opts))
	needs, err = b.NeedsBootstrap(ctx, l, cfg, opts)
	require.NoError(t, err)
	require.False(t, needs, "after Bootstrap, NeedsBootstrap should be false")

	// 3) Delete just the container — must report needs bootstrap again.
	blob := newAzureBlobClient(t, env, res)
	require.NoError(t, blob.EnsureContainerDeleted(ctx, res.Container))

	// Invalidate the backend's in-process init cache so the next call
	// actually queries Azure. NewBackend gives us a fresh instance.
	freshBackend := azurermbackend.NewBackend()
	needs, err = freshBackend.NeedsBootstrap(ctx, l, cfg, opts)
	require.NoError(t, err)
	require.True(t, needs, "after container delete, NeedsBootstrap should be true again")

	// 4) Full passthrough — all skip_*_creation true — early returns
	//    false without touching Azure even when the account is gone.
	passCfg := azureBackendConfig(env, res, map[string]any{
		"skip_resource_group_creation":  true,
		"skip_storage_account_creation": true,
		"skip_container_creation":       true,
	})
	needs, err = freshBackend.NeedsBootstrap(ctx, l, passCfg, opts)
	require.NoError(t, err)
	assert.False(t, needs, "passthrough config should never need bootstrap")

	// 5) Only skip_storage_account_creation set — container check still
	//    runs; since the container was deleted in step 3, this must be
	//    true.
	saSkipCfg := azureBackendConfig(env, res, map[string]any{
		"skip_storage_account_creation": true,
	})
	needs, err = freshBackend.NeedsBootstrap(ctx, l, saSkipCfg, opts)
	require.NoError(t, err)
	assert.True(t, needs, "skip_storage_account_creation alone must still detect missing container")
}
