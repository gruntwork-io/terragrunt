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

func TestNewBlobClient_OIDCMissingTokenSource(t *testing.T) {
	t.Parallel()
	// A user that asks for OIDC auth but never wired a token source through
	// the builder lands here with Method=OIDC and a nil Credential. The
	// blob constructor must reject this rather than panic dereferencing.
	_, err := azurehelper.NewBlobClient(context.Background(), &azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodOIDC,
		AccountName:   testAccount,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	}, "")
	if err == nil {
		t.Fatal("expected error when OIDC config has no credential")
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

func TestBlobClient_BindAndGetObject(t *testing.T) {
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

	if c.Container() != "" {
		t.Errorf("Container() before bind = %q, want empty", c.Container())
	}

	// GetObject without bound container errors.
	if _, err := c.GetObject(context.Background(), "k"); err == nil {
		t.Error("GetObject without BindContainer should error")
	}

	c.BindContainer("state")

	if c.Container() != "state" {
		t.Errorf("Container() after bind = %q, want %q", c.Container(), "state")
	}
}

func TestBlobClient_ListBlobs_RequiresContainer(t *testing.T) {
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

	if _, err := c.ListBlobs(context.Background(), "", ""); err == nil {
		t.Error("ListBlobs with empty container should error")
	}
}

func TestBlobClient_CopyBlob_RequiresArgs(t *testing.T) {
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

	cases := [][4]string{
		{"", "k", "dst", "k2"},
		{"src", "", "dst", "k2"},
		{"src", "k", "", "k2"},
		{"src", "k", "dst", ""},
	}

	for _, tc := range cases {
		if err := c.CopyBlob(context.Background(), tc[0], tc[1], tc[2], tc[3]); err == nil {
			t.Errorf("CopyBlob%v should error", tc)
		}
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

	// Exercise GetObject via bound container.
	bc.BindContainer(container)

	body2, err := bc.GetObject(ctx, key)
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}

	got2, _ := io.ReadAll(body2)
	_ = body2.Close()

	if !bytes.Equal(got2, payload) {
		t.Errorf("GetObject payload mismatch: got %q want %q", got2, payload)
	}

	// Exercise ListBlobs and CopyBlob.
	names, err := bc.ListBlobs(ctx, container, "")
	if err != nil {
		t.Errorf("ListBlobs: %v", err)
	}

	found := false

	for _, n := range names {
		if n == key {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("ListBlobs did not include %q; got %v", key, names)
	}

	copyKey := "roundtrip-copy.txt"
	if err := bc.CopyBlob(ctx, container, key, container, copyKey); err != nil {
		t.Errorf("CopyBlob: %v", err)
	}

	if err := bc.DeleteBlob(ctx, container, copyKey); err != nil {
		t.Logf("cleanup DeleteBlob(copy): %v", err)
	}

	if err := bc.DeleteBlob(ctx, container, key); err != nil {
		t.Errorf("DeleteBlob: %v", err)
	}

	// Idempotent delete of already-deleted blob should succeed.
	if err := bc.DeleteBlob(ctx, container, key); err != nil {
		t.Errorf("DeleteBlob (idempotent): %v", err)
	}
}
