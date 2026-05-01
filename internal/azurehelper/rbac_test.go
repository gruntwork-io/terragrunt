//go:build azure

package azurehelper_test

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
)

func TestNewRBACClient_NilConfig(t *testing.T) {
	t.Parallel()

	if _, err := azurehelper.NewRBACClient(nil); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestNewRBACClient_MissingSubscription(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewRBACClient(&azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodAzureAD,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	if err == nil {
		t.Fatal("expected error for missing subscription_id")
	}
}

func TestNewRBACClient_MissingCredential(t *testing.T) {
	t.Parallel()
	// SAS token has no token credential — RBAC needs one.
	_, err := azurehelper.NewRBACClient(&azurehelper.AzureConfig{
		Method:         azurehelper.AuthMethodSasToken,
		SubscriptionID: testSub,
		SasToken:       testSASToken,
		ClientOptions:  azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	if err == nil {
		t.Fatal("expected error when SAS-token config used for RBAC")
	}
}
