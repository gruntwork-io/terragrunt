//go:build azure

package azurehelper_test

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
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
