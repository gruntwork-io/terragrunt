package azurehelper_test

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
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

	// An empty container name is a caller invariant, so Container panics.
	assert.Panics(t, func() { _ = c.Container("") })

	// Blob operations on a valid container reject an empty key by panicking.
	cc := c.Container("c")

	assert.Panics(t, func() { _, _ = cc.GetBlob(ctx, "") })
	assert.Panics(t, func() { _ = cc.PutBlob(ctx, "", nil) })
	assert.Panics(t, func() { _ = cc.PutBlobFromReader(ctx, "", nil) })
	assert.Panics(t, func() { _ = cc.EnsureBlobDeleted(ctx, "") })
	assert.Panics(t, func() { _, _ = cc.BlobExists(ctx, "") })
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
		assert.Panics(t, func() { _ = c.Container(tc[0]).CopyBlob(t.Context(), log.New(), tc[1], c.Container(tc[2]), tc[3]) }, "CopyBlob%v should panic", tc)
	}
}
