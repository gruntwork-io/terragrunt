//go:build azure

package azurehelper_test

import (
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/stretchr/testify/assert"

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

	assert.Panics(t, func() {
		_, _ = azurehelper.NewResourceGroupClient(&azurehelper.AzureConfig{
			Method:        azurehelper.AuthMethodAzureAD,
			ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
		})
	})
}

func TestNewResourceGroupClient_MissingCredential(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		_, _ = azurehelper.NewResourceGroupClient(&azurehelper.AzureConfig{
			Method:         azurehelper.AuthMethodAccessKey,
			SubscriptionID: testSub,
			AccessKey:      "key",
			ClientOptions:  azcore.ClientOptions{Cloud: cloud.AzurePublic},
		})
	})
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
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if !exists {
		t.Errorf("Exists = false, want true")
	}
}

func TestResourceGroup_Exists_False(t *testing.T) {
	t.Parallel()

	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusNotFound, body: jsonBody(map[string]any{
		"error": map[string]any{"code": "ResourceGroupNotFound", "message": "not found"},
	})})

	exists, err := c.Exists(t.Context(), "rg")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if exists {
		t.Errorf("Exists = true, want false")
	}
}

func TestResourceGroup_EnsureResourceGroup_RequiresLocation(t *testing.T) {
	t.Parallel()
	// 404 -> not exists -> location validation kicks in and panics.
	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusNotFound, body: jsonBody(map[string]any{
		"error": map[string]any{"code": "ResourceGroupNotFound"},
	})})

	assert.Panics(t, func() { _ = c.EnsureResourceGroup(t.Context(), log.New(), "rg", "") })
}

func TestResourceGroup_EnsureResourceGroup_NoopWhenExists(t *testing.T) {
	t.Parallel()
	// 204 -> exists -> CreateOrUpdate must not be called, and missing location is fine.
	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusNoContent, body: nil})

	if err := c.EnsureResourceGroup(t.Context(), log.New(), "rg", ""); err != nil {
		t.Errorf("EnsureResourceGroup on existing RG: %v", err)
	}
}

func newTestResourceGroupClient(t *testing.T, tr *stubTransport) *azurehelper.ResourceGroupClient {
	t.Helper()

	c, err := azurehelper.NewResourceGroupClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	return c
}
