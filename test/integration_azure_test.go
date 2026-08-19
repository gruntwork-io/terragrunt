//go:build azure

// Package test_test contains the Azure end-to-end backend tests.
//
// These tests create and destroy real Azure resources behind the `azure` build tag.
//
// Unlike the hermetic tests, these fail loudly when credentials are absent
// instead of calling t.Skip. A silently skipped live test reports success and
// hides the fact that nothing was verified against Azure.
package test_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureAzureBackend         = "fixtures/azure-backend"
	testFixtureAzureDependencyState = "fixtures/output-from-remote-state-azure"

	// azureTestLocation is the region the test resource group and storage
	// account are created in.
	azureTestLocation = "eastus"

	// azureCleanupTimeout bounds the post-test teardown, which runs with a
	// fresh context because the test context is already cancelled by then.
	azureCleanupTimeout = 5 * time.Minute

	// azureLookupTimeout bounds the resource group lookup.
	azureLookupTimeout = 2 * time.Minute
)

// TestAzureDependencyFetchOutputFromState proves that dependency outputs are read from the
// Azure state blob without invoking the producer's configured OpenTofu/Terraform binary.
func TestAzureDependencyFetchOutputFromState(t *testing.T) {
	t.Parallel()

	_, _, rootPath := setupAzureFixture(t, testFixtureAzureDependencyState)
	producerPath := filepath.Join(rootPath, "producer")
	consumerPath := filepath.Join(rootPath, "consumer")
	consumerPlan := "terragrunt run plan --backend-bootstrap --experiment azure-backend " +
		"--dependency-fetch-output-from-state --non-interactive --log-level debug --working-dir " + consumerPath

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, consumerPlan)
	require.NoError(t, err)
	assert.Contains(t, stdout, "mock-azure-value")
	assert.NotContains(t, stdout, "from-azure-state")

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run apply --backend-bootstrap --experiment azure-backend --non-interactive --tf-path "+
			helpers.WrappedBinary(t.Context())+" --working-dir "+producerPath+" -- -auto-approve",
	)
	require.NoError(t, err)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, consumerPlan)
	require.NoError(t, err)
	assert.Contains(t, stdout, "from-azure-state")
	assert.NotContains(t, stdout, "mock-azure-value")

	// The value alone proves nothing: only the direct reader emits this line.
	assert.Contains(t, stderr+stdout, "Fetching outputs directly from azurerm://",
		"outputs must come from the state blob, not from running tofu output")
}

// Environment variables the live tests read, most specific first. The ARM_* /
// AZURE_* names are the ones the azurerm backend and the Azure SDK already
// honor, so a developer who can run `tofu init` against Azure can run these
// tests unchanged; the TG_AZURE_TEST_* names let CI scope a dedicated test
// subscription without redirecting every Azure tool on the runner.
var (
	envAzureSubscriptionID = []string{"TG_AZURE_TEST_SUBSCRIPTION_ID", "ARM_SUBSCRIPTION_ID", "AZURE_SUBSCRIPTION_ID"}
	envAzureResourceGroup  = []string{"TG_AZURE_TEST_RESOURCE_GROUP", "AZURE_RES_GROUP_NAME"}
	envAzureStorageAccount = []string{"TG_AZURE_TEST_STORAGE_ACCOUNT", "ARM_STORAGE_ACCOUNT_NAME"}
)

// TestAzureBootstrapBackend verifies the three ways a unit can reach a
// bootstrapped azurerm backend: not at all without the flag, via
// --backend-bootstrap, and via the explicit `backend bootstrap` command.
func TestAzureBootstrapBackend(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		checkResult func(t *testing.T, ctx context.Context, stderr string, account, container string, err error)
		name        string
		args        string
	}{
		{
			name: "no bootstrap without flag",
			args: "run apply",
			checkResult: func(t *testing.T, _ context.Context, stderr string, _, _ string, err error) {
				t.Helper()

				require.Error(t, err, "a missing container must not be created without --backend-bootstrap")
				assert.NotEmpty(t, stderr)
			},
		},
		{
			name: "bootstrap with flag",
			args: "run apply --backend-bootstrap",
			checkResult: func(t *testing.T, ctx context.Context, _ string, account, container string, err error) {
				t.Helper()

				require.NoError(t, err)
				assertAzureContainerExists(t, ctx, account, container)
			},
		},
		{
			name: "bootstrap by backend command",
			args: "backend bootstrap",
			checkResult: func(t *testing.T, ctx context.Context, _ string, account, container string, err error) {
				t.Helper()

				require.NoError(t, err)
				assertAzureContainerExists(t, ctx, account, container)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			account, container, rootPath := setupAzureBackendFixture(t)

			_, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt "+tc.args+" --all --non-interactive --experiment azure-backend --log-level debug --working-dir "+rootPath,
			)

			tc.checkResult(t, ctx, stderr, account, container, err)
		})
	}
}

// TestAzureBackendVersioningConverges verifies that bootstrap enables blob
// versioning on the storage account backing the state.
func TestAzureBackendVersioningConverges(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	account, container, rootPath := setupAzureBackendFixture(t)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt backend bootstrap --all --non-interactive --experiment azure-backend --working-dir "+rootPath,
	)
	require.NoError(t, err)

	assertAzureContainerExists(t, ctx, account, container)

	cfg := azureTestConfig(ctx, t, account)

	// Blob versioning is an ARM management-plane property. Access-key and SAS
	// auth cannot reach ARM at all, so there is nothing to assert under them.
	if cfg.Method == azurehelper.AuthMethodAccessKey || cfg.Method == azurehelper.AuthMethodSasToken {
		t.Skipf("versioning convergence needs the ARM control plane, which %s auth cannot reach", cfg.Method)
	}

	saClient, err := azurehelper.NewStorageAccountClient(cfg)
	require.NoError(t, err)

	enabled, err := saClient.IsVersioningEnabled(ctx)
	require.NoError(t, err)
	assert.True(t, enabled, "bootstrap must enable blob versioning on the state account")
}

// TestAzureDeleteBackend verifies that `backend delete` removes the state blob
// and that the operation is idempotent.
func TestAzureDeleteBackend(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	account, container, rootPath := setupAzureBackendFixture(t)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run apply --all --non-interactive --backend-bootstrap --experiment azure-backend --working-dir "+rootPath,
	)
	require.NoError(t, err)

	assertAzureContainerExists(t, ctx, account, container)

	// Capture what apply actually wrote, so the delete assertion below is about
	// real state blobs rather than an assumed key layout.
	blobClient, err := azurehelper.NewBlobClient(azureTestConfig(ctx, t, account))
	require.NoError(t, err)

	stateBlobs, err := blobClient.Container(container).ListBlobs(ctx, log.New())
	require.NoError(t, err)
	require.NotEmpty(t, stateBlobs, "apply must have written state for the delete to be meaningful")

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt backend delete --all --non-interactive --force --experiment azure-backend --working-dir "+rootPath,
	)
	require.NoError(t, err)

	// A successful exit is not proof of deletion; the blobs must be gone.
	for _, key := range stateBlobs {
		exists, err := blobClient.Container(container).BlobExists(ctx, key)
		require.NoError(t, err)
		assert.False(t, exists, "state blob %s must be removed by backend delete", key)
	}

	// Deleting again must be a no-op rather than an error.
	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt backend delete --all --non-interactive --force --experiment azure-backend --working-dir "+rootPath,
	)
	require.NoError(t, err, "backend delete must be idempotent")
}

// TestAzureBackendRequiresExperiment verifies the experiment gate end to end:
// an explicit backend command refuses to run without the experiment enabled.
func TestAzureBackendRequiresExperiment(t *testing.T) {
	t.Parallel()

	_, _, rootPath := setupAzureBackendFixture(t)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt backend delete --all --non-interactive --force --working-dir "+rootPath,
	)

	require.Error(t, err)

	// The CLI routes the failure through the returned error; stderr carries only
	// the troubleshooting tip, so assert on the error the user actually gets.
	assert.Contains(t, err.Error()+stderr, "azure-backend",
		"the failure must name the experiment the user needs to enable")
}

// setupAzureBackendFixture copies the fixture, fills in the live account
// details, and registers cleanup of the container it will create. It returns
// the storage account, the container name, and the working directory.
func setupAzureBackendFixture(t *testing.T) (string, string, string) {
	t.Helper()

	return setupAzureFixture(t, testFixtureAzureBackend)
}

// setupAzureFixture copies an Azure fixture, fills in the live account details, and registers
// cleanup of the unique container it will create.
func setupAzureFixture(t *testing.T, fixture string) (string, string, string) {
	t.Helper()

	subscriptionID := requireAzureEnv(t, envAzureSubscriptionID)
	account := requireAzureEnv(t, envAzureStorageAccount)
	resourceGroup := azureResourceGroup(t.Context(), t, account)

	// Container names are lowercase alphanumeric with dashes, 3-63 chars.
	container := "tg-test-" + strings.ToLower(helpers.UniqueID())

	tmpEnvPath := helpers.CopyEnvironment(t, fixture)
	rootPath := filepath.Join(tmpEnvPath, fixture)
	helpers.CleanupTerraformFolder(t, rootPath)

	commonConfigPath := filepath.Join(rootPath, "common.hcl")
	helpers.CopyAndFillMapPlaceholders(t, commonConfigPath, commonConfigPath, map[string]string{
		"__FILL_IN_STORAGE_ACCOUNT__": account,
		"__FILL_IN_CONTAINER__":       container,
		"__FILL_IN_RESOURCE_GROUP__":  resourceGroup,
		"__FILL_IN_SUBSCRIPTION_ID__": subscriptionID,
		"__FILL_IN_LOCATION__":        azureTestLocation,
	})

	t.Cleanup(func() { deleteAzureContainer(t, account, container) })

	return account, container, rootPath
}

// azureResourceGroup returns the resource group holding the test storage
// account. It is looked up from the subscription when no environment variable
// names it, so running these tests needs only a subscription and an account:
// the group is a property of the account, not a separate thing to configure.
func azureResourceGroup(ctx context.Context, t *testing.T, account string) string {
	t.Helper()

	for _, name := range envAzureResourceGroup {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}

	// The lookup needs the ARM control plane. Data-plane-only auth (access key,
	// SAS) cannot reach it, so those runs must name the group explicitly.
	// A config with no resource group is enough to reach ARM and ask.
	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			SubscriptionID:     requireAzureEnv(t, envAzureSubscriptionID),
			StorageAccountName: account,
			UseAzureADAuth:     new(true),
		}).
		Build(log.New(), venv.OSVenv())
	require.NoError(t, err, "resolving Azure credentials")

	lookupCtx, cancel := context.WithTimeout(ctx, azureLookupTimeout)
	defer cancel()

	group, err := azurehelper.FindResourceGroupForAccount(lookupCtx, cfg, account)
	require.NoErrorf(t, err,
		"could not determine the resource group for storage account %q; set %s to name it explicitly",
		account, strings.Join(envAzureResourceGroup, " or "))

	return group
}

// azureTestConfig resolves an AzureConfig against the live environment for the
// given storage account, using the same builder the backend uses.
func azureTestConfig(ctx context.Context, t *testing.T, account string) *azurehelper.AzureConfig {
	t.Helper()

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			SubscriptionID:     requireAzureEnv(t, envAzureSubscriptionID),
			ResourceGroupName:  azureResourceGroup(ctx, t, account),
			StorageAccountName: account,
			UseAzureADAuth:     new(true),
		}).
		Build(log.New(), venv.OSVenv())
	require.NoError(t, err, "resolving Azure credentials")

	return cfg
}

func assertAzureContainerExists(t *testing.T, ctx context.Context, account, container string) {
	t.Helper()

	blobClient, err := azurehelper.NewBlobClient(azureTestConfig(ctx, t, account))
	require.NoError(t, err)

	exists, err := blobClient.Container(container).Exists(ctx)
	require.NoError(t, err)
	assert.True(t, exists, "container %s must exist in account %s", container, account)
}

// deleteAzureContainer removes the container a test created. Failures are
// logged rather than failing the test: the assertions have already run, and a
// leaked container should not turn a passing test red. The nightly cleanup
// still reclaims anything left behind.
func deleteAzureContainer(t *testing.T, account, container string) {
	t.Helper()

	// A fresh context: the test context is cancelled by the time cleanup runs.
	ctx, cancel := context.WithTimeout(context.Background(), azureCleanupTimeout)
	defer cancel()

	blobClient, err := azurehelper.NewBlobClient(azureTestConfig(ctx, t, account))
	if err != nil {
		t.Logf("cleanup: building blob client for %s: %v", account, err)

		return
	}

	if err := blobClient.Container(container).EnsureDeleted(ctx); err != nil {
		t.Logf("cleanup: deleting container %s/%s: %v", account, container, err)
	}
}

// requireAzureEnv returns the first non-empty value among names, failing the
// test when none is set. These tests deliberately do NOT skip: a skipped live
// test reports a green run while proving nothing was verified against Azure.
func requireAzureEnv(t *testing.T, names []string) string {
	t.Helper()

	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}

	require.FailNowf(t, "missing Azure credentials",
		"set one of %s to run the Azure integration tests", strings.Join(names, ", "))

	return ""
}
