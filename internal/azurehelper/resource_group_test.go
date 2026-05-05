//go:build azure

package azurehelper_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func TestNewResourceGroupClient_NilConfig(t *testing.T) {
	t.Parallel()

	if _, err := azurehelper.NewResourceGroupClient(nil); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestNewResourceGroupClient_MissingSubscription(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewResourceGroupClient(&azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodAzureAD,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	if err == nil {
		t.Fatal("expected error for missing subscription_id")
	}
}

func TestNewResourceGroupClient_MissingCredential(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewResourceGroupClient(&azurehelper.AzureConfig{
		Method:         azurehelper.AuthMethodAccessKey,
		SubscriptionID: testSub,
		AccessKey:      "key",
		ClientOptions:  azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	if err == nil {
		t.Fatal("expected error when access-key config used for RG ops")
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

func TestResourceGroup_RequiresName(t *testing.T) {
	t.Parallel()

	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{})})

	if _, err := c.Exists(context.Background(), ""); err == nil {
		t.Error("Exists with empty name should error")
	}

	if err := c.CreateIfNecessary(context.Background(), log.New(), "", "eastus"); err == nil {
		t.Error("CreateIfNecessary with empty name should error")
	}

	if err := c.Delete(context.Background(), log.New(), ""); err == nil {
		t.Error("Delete with empty name should error")
	}
}

func TestResourceGroup_Exists_True(t *testing.T) {
	t.Parallel()

	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusNoContent, body: nil})

	exists, err := c.Exists(context.Background(), "rg")
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

	exists, err := c.Exists(context.Background(), "rg")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if exists {
		t.Errorf("Exists = true, want false")
	}
}

func TestResourceGroup_CreateIfNecessary_RequiresLocation(t *testing.T) {
	t.Parallel()
	// 404 → not exists → location validation kicks in.
	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusNotFound, body: jsonBody(map[string]any{
		"error": map[string]any{"code": "ResourceGroupNotFound"},
	})})

	if err := c.CreateIfNecessary(context.Background(), log.New(), "rg", ""); err == nil {
		t.Error("CreateIfNecessary with empty location on missing RG should error")
	}
}

func TestResourceGroup_CreateIfNecessary_NoopWhenExists(t *testing.T) {
	t.Parallel()
	// 204 → exists → CreateOrUpdate must not be called, and missing location is fine.
	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusNoContent, body: nil})

	if err := c.CreateIfNecessary(context.Background(), log.New(), "rg", ""); err != nil {
		t.Errorf("CreateIfNecessary on existing RG: %v", err)
	}
}
