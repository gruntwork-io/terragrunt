//go:build azure

package azurehelper_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func TestNewBlobClient_NilConfig(t *testing.T) {
	t.Parallel()

	// Config presence is a caller invariant, so it panics.
	assert.Panics(t, func() { _, _ = azurehelper.NewBlobClient(nil) })
}

func TestNewBlobClient_MissingAccountName(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		_, _ = azurehelper.NewBlobClient(&azurehelper.AzureConfig{
			Method:        azurehelper.AuthMethodSasToken,
			SasToken:      testSASToken,
			ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
		})
	})
}

func TestNewBlobClient_NoCredentialForTokenMethod(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		_, _ = azurehelper.NewBlobClient(&azurehelper.AzureConfig{
			Method:        azurehelper.AuthMethodAzureAD,
			AccountName:   testAccount,
			ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
		})
	})
}

func TestNewBlobClient_OIDCMissingTokenSource(t *testing.T) {
	t.Parallel()
	// OIDC with a nil credential is a resolved-config invariant, so it panics.
	assert.Panics(t, func() {
		_, _ = azurehelper.NewBlobClient(&azurehelper.AzureConfig{
			Method:        azurehelper.AuthMethodOIDC,
			AccountName:   testAccount,
			ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
		})
	})
}

func TestNewBlobClient_SasToken(t *testing.T) {
	t.Parallel()

	c, err := azurehelper.NewBlobClient(&azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodSasToken,
		SasToken:      "?sv=2023-01-01&sig=x",
		AccountName:   testAccount,
		CloudConfig:   cloud.AzurePublic,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	require.NoError(t, err)
	assert.Equal(t, testAccount, c.AccountName)
	assert.NotNil(t, c.Client)
}

func TestNewBlobClient_AccessKey(t *testing.T) {
	t.Parallel()

	c, err := azurehelper.NewBlobClient(&azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodAccessKey,
		AccessKey:     "dGVzdGtleQ==", // base64("testkey")
		AccountName:   testAccount,
		CloudConfig:   cloud.AzurePublic,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	require.NoError(t, err)
	assert.Equal(t, testAccount, c.AccountName)
}

func TestNewBlobClient_AccessKeyInvalidBase64(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewBlobClient(&azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodAccessKey,
		AccessKey:     "!!!not-base64!!!",
		AccountName:   testAccount,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	require.Error(t, err, "expected error for invalid base64 access key")
}

func TestBlobMethods_RequireNames(t *testing.T) {
	t.Parallel()

	c, err := azurehelper.NewBlobClient(&azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodSasToken,
		SasToken:      testSASToken,
		AccountName:   testAccount,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	require.NoError(t, err)

	ctx := t.Context()

	// Empty container/key values are caller invariants, so each method panics.
	assert.Panics(t, func() { _, _ = c.ContainerExists(ctx, "") })
	assert.Panics(t, func() { _ = c.CreateContainer(ctx, "") })
	assert.Panics(t, func() { _ = c.EnsureContainerDeleted(ctx, "") })
	assert.Panics(t, func() { _, _ = c.GetBlob(ctx, "", "k") })
	assert.Panics(t, func() { _, _ = c.GetBlob(ctx, "c", "") })
	assert.Panics(t, func() { _ = c.PutBlob(ctx, "", "k", nil) })
	assert.Panics(t, func() { _ = c.PutBlobFromReader(ctx, "c", "", nil) })
	assert.Panics(t, func() { _ = c.EnsureBlobDeleted(ctx, "", "k") })
	assert.Panics(t, func() { _ = c.EnsureContainer(ctx, "") })
}

func TestNewBlobClient_GovernmentCloud(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewBlobClient(&azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodSasToken,
		SasToken:      testSASToken,
		AccountName:   testAccount,
		CloudConfig:   cloud.AzureGovernment,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzureGovernment},
	})
	require.NoError(t, err, "government cloud client")
}

func TestNewBlobClient_ChinaCloud(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewBlobClient(&azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodSasToken,
		SasToken:      testSASToken,
		AccountName:   testAccount,
		CloudConfig:   cloud.AzureChina,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzureChina},
	})
	require.NoError(t, err, "china cloud client")
}

func TestBlobClient_ListBlobs_RequiresContainer(t *testing.T) {
	t.Parallel()

	c, err := azurehelper.NewBlobClient(&azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodSasToken,
		SasToken:      testSASToken,
		AccountName:   testAccount,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	require.NoError(t, err)

	assert.Panics(t, func() { _, _ = c.ListBlobs(t.Context(), log.New(), "") })
}

func TestBlobClient_CopyBlob_RequiresArgs(t *testing.T) {
	t.Parallel()

	c, err := azurehelper.NewBlobClient(&azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodSasToken,
		SasToken:      testSASToken,
		AccountName:   testAccount,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	require.NoError(t, err)

	cases := [][4]string{
		{"", "k", "dst", "k2"},
		{"src", "", "dst", "k2"},
		{"src", "k", "", "k2"},
		{"src", "k", "dst", ""},
	}

	for _, tc := range cases {
		assert.Panics(t, func() { _ = c.CopyBlob(t.Context(), tc[0], tc[1], tc[2], tc[3]) }, "CopyBlob%v should panic", tc)
	}
}

// TestBlob_LiveRoundTrip round-trips a blob against a real storage account;
// skipped unless TG_AZURE_TEST_STORAGE_ACCOUNT and TG_AZURE_TEST_SUBSCRIPTION_ID are set.
func TestBlob_LiveRoundTrip(t *testing.T) {
	t.Parallel()

	env := venv.OSVenv().Env
	account := env["TG_AZURE_TEST_STORAGE_ACCOUNT"]
	sub := env["TG_AZURE_TEST_SUBSCRIPTION_ID"]

	if account == "" || sub == "" {
		t.Skip("TG_AZURE_TEST_STORAGE_ACCOUNT and TG_AZURE_TEST_SUBSCRIPTION_ID are required for live test")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			SubscriptionID:     sub,
			StorageAccountName: account,
			UseAzureADAuth:     true,
		}).
		Build(log.New())
	require.NoError(t, err, "Build config")

	bc, err := azurehelper.NewBlobClient(cfg)
	require.NoError(t, err, "NewBlobClient")

	suffix := make([]byte, 4)
	_, err = rand.Read(suffix)
	require.NoError(t, err, "rand.Read")

	container := "tg-test-" + hex.EncodeToString(suffix)
	key := "roundtrip.txt"
	payload := []byte("hello from terragrunt azurehelper integration test")

	require.NoError(t, bc.CreateContainer(ctx, container), "CreateContainer")

	t.Cleanup(func() {
		// Fresh context because t.Context() is already cancelled during cleanup.
		_ = bc.EnsureContainerDeleted(context.Background(), container)
	})

	exists, err := bc.ContainerExists(ctx, container)
	require.NoError(t, err, "ContainerExists after create")
	require.True(t, exists, "ContainerExists after create should be true")

	require.NoError(t, bc.PutBlob(ctx, container, key, payload), "PutBlob")

	body, err := bc.GetBlob(ctx, container, key)
	require.NoError(t, err, "GetBlob")

	got, err := io.ReadAll(body)
	require.NoError(t, body.Close(), "body close")
	require.NoError(t, err, "read body")
	assert.Equal(t, payload, got, "payload mismatch")

	// Exercise ListBlobs and CopyBlob.
	names, err := bc.ListBlobs(ctx, log.New(), container)
	require.NoError(t, err, "ListBlobs")
	assert.Contains(t, names, key, "ListBlobs did not include %q", key)

	copyKey := "roundtrip-copy.txt"
	require.NoError(t, bc.CopyBlob(ctx, container, key, container, copyKey), "CopyBlob")

	if err := bc.EnsureBlobDeleted(ctx, container, copyKey); err != nil {
		t.Logf("cleanup EnsureBlobDeleted(copy): %v", err)
	}

	require.NoError(t, bc.EnsureBlobDeleted(ctx, container, key), "EnsureBlobDeleted")
	// Idempotent delete of already-deleted blob should succeed.
	require.NoError(t, bc.EnsureBlobDeleted(ctx, container, key), "EnsureBlobDeleted (idempotent)")
}
