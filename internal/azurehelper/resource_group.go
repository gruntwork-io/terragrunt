package azurehelper

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ResourceGroupClient wraps the Azure resource-groups management API.
type ResourceGroupClient struct {
	client         *armresources.ResourceGroupsClient
	subscriptionID string
}

// NewResourceGroupClient creates a resource-groups client. Like RBACClient,
// this requires a token credential — SAS-token and access-key configs are
// data-plane only.
func NewResourceGroupClient(cfg *AzureConfig) (*ResourceGroupClient, error) {
	if cfg == nil {
		return nil, errors.Errorf("azure config is required")
	}

	if cfg.SubscriptionID == "" {
		return nil, errors.Errorf("subscription_id is required for resource group operations")
	}

	if cfg.Credential == nil {
		return nil, errors.Errorf("resource group operations require a token credential (auth method %q is not supported)", cfg.Method)
	}

	client, err := armresources.NewResourceGroupsClient(cfg.SubscriptionID, cfg.Credential, &arm.ClientOptions{
		ClientOptions: cfg.ClientOptions,
	})
	if err != nil {
		return nil, errors.Errorf("creating resource groups client: %w", err)
	}

	return &ResourceGroupClient{client: client, subscriptionID: cfg.SubscriptionID}, nil
}

// Exists reports whether the named resource group exists in the subscription.
func (c *ResourceGroupClient) Exists(ctx context.Context, name string) (bool, error) {
	if name == "" {
		return false, errors.Errorf("resource group name is required")
	}

	resp, err := c.client.CheckExistence(ctx, name, nil)
	if err != nil {
		if IsNotFound(err) {
			return false, nil
		}

		return false, WrapError(err, "checking resource group existence")
	}

	return resp.Success, nil
}

// CreateIfNecessary creates a resource group at location if it does not
// already exist. If location is empty, "westeurope" is used.
func (c *ResourceGroupClient) CreateIfNecessary(ctx context.Context, l log.Logger, name, location string) error {
	if name == "" {
		return errors.Errorf("resource group name is required")
	}

	exists, err := c.Exists(ctx, name)
	if err != nil {
		return err
	}

	if exists {
		l.Debugf("azurehelper: resource group %s already exists", name)
		return nil
	}

	if location == "" {
		location = "westeurope"
	}

	_, err = c.client.CreateOrUpdate(ctx, name, armresources.ResourceGroup{
		Location: to.Ptr(location),
	}, nil)
	if err != nil {
		return WrapError(err, "creating resource group "+name)
	}

	l.Debugf("azurehelper: created resource group %s in %s", name, location)

	return nil
}

// Delete deletes the named resource group and waits for the long-running
// operation to complete. Missing resource groups return nil.
func (c *ResourceGroupClient) Delete(ctx context.Context, l log.Logger, name string) error {
	if name == "" {
		return errors.Errorf("resource group name is required")
	}

	poller, err := c.client.BeginDelete(ctx, name, nil)
	if err != nil {
		if IsNotFound(err) {
			return nil
		}

		return WrapError(err, "starting resource group delete "+name)
	}

	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		if IsNotFound(err) {
			return nil
		}

		return WrapError(err, "waiting for resource group delete "+name)
	}

	l.Debugf("azurehelper: deleted resource group %s", name)

	return nil
}
