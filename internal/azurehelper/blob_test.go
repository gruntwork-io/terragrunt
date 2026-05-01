//go:build azure

package azurehelper_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func TestNewBlobClient_NilConfig(t *testing.T) {
	t.Parallel()

	if _, err := azurehelper.NewBlobClient(context.Background(), nil, ""); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestNewBlobClient_MissingAccountName(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewBlobClient(context.Background(), &azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodSasToken,
		SasToken:      testSASToken,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	}, "")
	if err == nil {
		t.Fatal("expected error for missing storage account name")
	}
}

func TestNewBlobClient_NoCredentialForTokenMethod(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewBlobClient(context.Background(), &azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodAzureAD,
		AccountName:   testAccount,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	}, "")
	if err == nil {
		t.Fatal("expected error when token-method config has no credential")
	}
}

func TestNewBlobClient_SasToken(t *testing.T) {
	t.Parallel()

	c, err := azurehelper.NewBlobClient(context.Background(), &azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodSasToken,
		SasToken:      "?sv=2023-01-01&sig=x",
		AccountName:   testAccount,
		CloudConfig:   cloud.AzurePublic,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.AccountName() != testAccount {
		t.Errorf("AccountName = %q, want %q", c.AccountName(), testAccount)
	}

	if c.AzClient() == nil {
		t.Error("AzClient() is nil")
	}
}

func TestNewBlobClient_AccessKey(t *testing.T) {
	t.Parallel()

	c, err := azurehelper.NewBlobClient(context.Background(), &azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodAccessKey,
		AccessKey:     "dGVzdGtleQ==", // base64("testkey")
		AccountName:   testAccount,
		CloudConfig:   cloud.AzurePublic,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.AccountName() != testAccount {
		t.Errorf("AccountName = %q", c.AccountName())
	}
}

func TestNewBlobClient_AccessKeyInvalidBase64(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewBlobClient(context.Background(), &azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodAccessKey,
		AccessKey:     "!!!not-base64!!!",
		AccountName:   testAccount,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	}, "")
	if err == nil {
		t.Fatal("expected error for invalid base64 access key")
	}
}

func TestNewBlobClient_EndpointSuffixOverride(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewBlobClient(context.Background(), &azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodSasToken,
		SasToken:      testSASToken,
		AccountName:   testAccount,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	}, "core.usgovcloudapi.net")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBlobMethods_RequireNames(t *testing.T) {
	t.Parallel()

	c, err := azurehelper.NewBlobClient(context.Background(), &azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodSasToken,
		SasToken:      testSASToken,
		AccountName:   testAccount,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	}, "")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	ctx := context.Background()

	if _, err := c.ContainerExists(ctx, ""); err == nil {
		t.Error("ContainerExists(\"\") should error")
	}

	if err := c.CreateContainer(ctx, ""); err == nil {
		t.Error("CreateContainer(\"\") should error")
	}

	if err := c.DeleteContainer(ctx, ""); err == nil {
		t.Error("DeleteContainer(\"\") should error")
	}

	if _, err := c.GetBlob(ctx, "", "k"); err == nil {
		t.Error("GetBlob with empty container should error")
	}

	if _, err := c.GetBlob(ctx, "c", ""); err == nil {
		t.Error("GetBlob with empty key should error")
	}

	if err := c.PutBlob(ctx, "", "k", nil); err == nil {
		t.Error("PutBlob with empty container should error")
	}

	if err := c.PutBlobFromReader(ctx, "c", "", nil); err == nil {
		t.Error("PutBlobFromReader with empty key should error")
	}

	if err := c.DeleteBlob(ctx, "", "k"); err == nil {
		t.Error("DeleteBlob with empty container should error")
	}

	if err := c.CreateContainerIfNecessary(ctx, ""); err == nil {
		t.Error("CreateContainerIfNecessary(\"\") should error")
	}
}

func TestNewBlobClient_GovernmentCloud(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewBlobClient(context.Background(), &azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodSasToken,
		SasToken:      testSASToken,
		AccountName:   testAccount,
		CloudConfig:   cloud.AzureGovernment,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzureGovernment},
	}, "")
	if err != nil {
		t.Fatalf("government cloud client: %v", err)
	}
}

func TestNewBlobClient_ChinaCloud(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewBlobClient(context.Background(), &azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodSasToken,
		SasToken:      testSASToken,
		AccountName:   testAccount,
		CloudConfig:   cloud.AzureChina,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzureChina},
	}, "")
	if err != nil {
		t.Fatalf("china cloud client: %v", err)
	}
}

// TestBlob_LiveRoundTrip exercises BlobClient against a real Azure storage
// account. Skipped unless TG_AZURE_TEST_STORAGE_ACCOUNT and
// TG_AZURE_TEST_SUBSCRIPTION_ID are set. Auth uses the Azure AD default
// credential chain (az login, MSI, env vars). Creates a temporary container,
// writes a blob, reads it back, deletes the blob, then deletes the container.
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
		Build(ctx, log.New())
	if err != nil {
		t.Fatalf("Build config: %v", err)
	}

	bc, err := azurehelper.NewBlobClient(ctx, cfg, "")
	if err != nil {
		t.Fatalf("NewBlobClient: %v", err)
	}

	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)

	container := "tg-test-" + hex.EncodeToString(suffix)
	key := "roundtrip.txt"
	payload := []byte("hello from terragrunt azurehelper integration test")

	if err := bc.CreateContainer(ctx, container); err != nil {
		t.Fatalf("CreateContainer: %v", err)
	}

	t.Cleanup(func() {
		_ = bc.DeleteContainer(context.Background(), container)
	})

	exists, err := bc.ContainerExists(ctx, container)
	if err != nil || !exists {
		t.Fatalf("ContainerExists after create: exists=%v err=%v", exists, err)
	}

	if err := bc.PutBlob(ctx, container, key, payload); err != nil {
		t.Fatalf("PutBlob: %v", err)
	}

	body, err := bc.GetBlob(ctx, container, key)
	if err != nil {
		t.Fatalf("GetBlob: %v", err)
	}

	got, err := io.ReadAll(body)
	_ = body.Close()

	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %q want %q", got, payload)
	}

	if err := bc.DeleteBlob(ctx, container, key); err != nil {
		t.Errorf("DeleteBlob: %v", err)
	}

	// Idempotent delete of already-deleted blob should succeed.
	if err := bc.DeleteBlob(ctx, container, key); err != nil {
		t.Errorf("DeleteBlob (idempotent): %v", err)
	}
}
