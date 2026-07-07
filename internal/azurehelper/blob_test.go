//go:build azure

package azurehelper_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func TestNewBlobClient_NilConfig(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewBlobClient(nil)
	require.Error(t, err, "expected error for nil config")
}

func TestNewBlobClient_MissingAccountName(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewBlobClient(&azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodSasToken,
		SasToken:      testSASToken,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	require.Error(t, err, "expected error for missing storage account name")
}

func TestNewBlobClient_NoCredentialForTokenMethod(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewBlobClient(&azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodAzureAD,
		AccountName:   testAccount,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	require.Error(t, err, "expected error when token-method config has no credential")
}

func TestNewBlobClient_OIDCMissingTokenSource(t *testing.T) {
	t.Parallel()
	// OIDC with a nil credential must be rejected, not panic.
	_, err := azurehelper.NewBlobClient(&azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodOIDC,
		AccountName:   testAccount,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	require.Error(t, err, "expected error when OIDC config has no credential")
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

	_, err = c.ContainerExists(ctx, "")
	require.Error(t, err, "ContainerExists(\"\") should error")

	require.Error(t, c.CreateContainer(ctx, ""), "CreateContainer(\"\") should error")
	require.Error(t, c.EnsureContainerDeleted(ctx, ""), "EnsureContainerDeleted(\"\") should error")

	_, err = c.GetBlob(ctx, "", "k")
	require.Error(t, err, "GetBlob with empty container should error")

	_, err = c.GetBlob(ctx, "c", "")
	require.Error(t, err, "GetBlob with empty key should error")

	require.Error(t, c.PutBlob(ctx, "", "k", nil), "PutBlob with empty container should error")
	require.Error(t, c.PutBlobFromReader(ctx, "c", "", nil), "PutBlobFromReader with empty key should error")
	require.Error(t, c.EnsureBlobDeleted(ctx, "", "k"), "EnsureBlobDeleted with empty container should error")
	require.Error(t, c.EnsureContainer(ctx, ""), "EnsureContainer(\"\") should error")
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

	_, err = c.ListBlobs(t.Context(), log.New(), "")
	require.Error(t, err, "ListBlobs with empty container should error")
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
		require.Error(t, c.CopyBlob(t.Context(), tc[0], tc[1], tc[2], tc[3]), "CopyBlob%v should error", tc)
	}
}

// TestBlob_LiveRoundTrip round-trips a blob against a real storage account;
// skipped unless TG_AZURE_TEST_STORAGE_ACCOUNT and TG_AZURE_TEST_SUBSCRIPTION_ID are set.
func TestBlob_LiveRoundTrip(t *testing.T) {
	t.Parallel()

	account := os.Getenv("TG_AZURE_TEST_STORAGE_ACCOUNT")
	sub := os.Getenv("TG_AZURE_TEST_SUBSCRIPTION_ID")

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
