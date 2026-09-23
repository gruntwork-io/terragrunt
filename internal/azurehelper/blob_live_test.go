//go:build azure

package azurehelper_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// TestAzureBlobLiveRoundTrip round-trips a blob against a real storage account;
// skipped unless TG_AZURE_TEST_STORAGE_ACCOUNT and TG_AZURE_TEST_SUBSCRIPTION_ID are set.
func TestAzureBlobLiveRoundTrip(t *testing.T) {
	t.Parallel()

	testVenv := venv.OSVenv()
	account := testVenv.Env["TG_AZURE_TEST_STORAGE_ACCOUNT"]
	sub := testVenv.Env["TG_AZURE_TEST_SUBSCRIPTION_ID"]

	if account == "" || sub == "" {
		t.Skip("TG_AZURE_TEST_STORAGE_ACCOUNT and TG_AZURE_TEST_SUBSCRIPTION_ID are required for live test")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			SubscriptionID:     sub,
			StorageAccountName: account,
			UseAzureADAuth:     new(true),
		}).
		Build(log.New(), testVenv)
	require.NoError(t, err, "Build config")

	bc, err := azurehelper.NewBlobClient(cfg)
	require.NoError(t, err, "NewBlobClient")

	suffix := make([]byte, 4)
	_, err = rand.Read(suffix)
	require.NoError(t, err, "rand.Read")

	container := "tg-test-" + hex.EncodeToString(suffix)
	key := "roundtrip.txt"
	payload := []byte("hello from terragrunt azurehelper integration test")

	cc := bc.Container(container)

	require.NoError(t, cc.Create(ctx), "Create")

	t.Cleanup(func() {
		// Fresh context because t.Context() is already cancelled during cleanup.
		_ = cc.EnsureDeleted(context.Background())
	})

	exists, err := cc.Exists(ctx)
	require.NoError(t, err, "Exists after create")
	require.True(t, exists, "Exists after create should be true")

	require.NoError(t, cc.PutBlob(ctx, key, payload), "PutBlob")

	body, err := cc.GetBlob(ctx, key)
	require.NoError(t, err, "GetBlob")

	got, err := io.ReadAll(body)
	require.NoError(t, body.Close(), "body close")
	require.NoError(t, err, "read body")
	assert.Equal(t, payload, got, "payload mismatch")

	// Exercise ListBlobs and CopyBlob.
	names, err := cc.ListBlobs(ctx, log.New())
	require.NoError(t, err, "ListBlobs")
	assert.Contains(t, names, key, "ListBlobs did not include %q", key)

	copyKey := "roundtrip-copy.txt"
	require.NoError(t, cc.CopyBlob(ctx, log.New(), key, cc, copyKey), "CopyBlob")

	if err := cc.EnsureBlobDeleted(ctx, copyKey); err != nil {
		t.Logf("cleanup EnsureBlobDeleted(copy): %v", err)
	}

	require.NoError(t, cc.EnsureBlobDeleted(ctx, key), "EnsureBlobDeleted")
	// Idempotent delete of already-deleted blob should succeed.
	require.NoError(t, cc.EnsureBlobDeleted(ctx, key), "EnsureBlobDeleted (idempotent)")
}
