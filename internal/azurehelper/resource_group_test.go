package azurehelper_test

import (
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func TestNewResourceGroupClient_NilConfig(t *testing.T) {
	t.Parallel()

	// Config presence is a caller invariant checked upstream, so it panics.
	assert.Panics(t, func() { _, _ = azurehelper.NewResourceGroupClient(nil) })
}

func TestNewResourceGroupClient_MissingSubscription(t *testing.T) {
	t.Parallel()

	// subscription_id is user supplied, so a missing value is a user error.
	_, err := azurehelper.NewResourceGroupClient(&azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodAzureAD,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	require.ErrorIs(t, err, azurehelper.ErrSubscriptionIDRequired)
}

func TestNewResourceGroupClient_MissingCredential(t *testing.T) {
	t.Parallel()

	// The auth method is user supplied, so an ARM-incapable one is a user error.
	_, err := azurehelper.NewResourceGroupClient(&azurehelper.AzureConfig{
		Method:         azurehelper.AuthMethodAccessKey,
		SubscriptionID: testSub,
		AccessKey:      "key",
		ClientOptions:  azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})

	var unsupported *azurehelper.UnsupportedAuthForOpError
	require.ErrorAs(t, err, &unsupported)
}

func TestResourceGroup_RequiresName(t *testing.T) {
	t.Parallel()

	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{})})

	// An empty name is a caller invariant violation, so it panics rather than errors.
	assert.Panics(t, func() { _, _ = c.Exists(t.Context(), "") })
	assert.Panics(t, func() { _ = c.EnsureResourceGroup(t.Context(), log.New(), "", "eastus") })
	assert.Panics(t, func() { _ = c.EnsureDeleted(t.Context(), log.New(), "") })
}

func TestResourceGroup_Exists_True(t *testing.T) {
	t.Parallel()

	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusNoContent, body: nil})

	exists, err := c.Exists(t.Context(), "rg")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestResourceGroup_Exists_False(t *testing.T) {
	t.Parallel()

	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusNotFound, body: jsonBody(map[string]any{
		"error": map[string]any{"code": "ResourceGroupNotFound", "message": "not found"},
	})})

	exists, err := c.Exists(t.Context(), "rg")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestResourceGroup_EnsureResourceGroup_RequiresLocation(t *testing.T) {
	t.Parallel()
	// 404 -> not exists -> location validation kicks in; location is user
	// supplied, so a missing value is a user error.
	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusNotFound, body: jsonBody(map[string]any{
		"error": map[string]any{"code": "ResourceGroupNotFound"},
	})})

	require.ErrorIs(t, c.EnsureResourceGroup(t.Context(), log.New(), "rg", ""), azurehelper.ErrLocationRequiredForRG)
}

func TestResourceGroup_EnsureResourceGroup_NoopWhenExists(t *testing.T) {
	t.Parallel()
	// 204 -> exists -> CreateOrUpdate must not be called, and missing location is fine.
	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusNoContent, body: nil})

	require.NoError(t, c.EnsureResourceGroup(t.Context(), log.New(), "rg", ""))
}

func newTestResourceGroupClient(t *testing.T, tr *stubTransport) *azurehelper.ResourceGroupClient {
	t.Helper()

	c, err := azurehelper.NewResourceGroupClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	return c
}
