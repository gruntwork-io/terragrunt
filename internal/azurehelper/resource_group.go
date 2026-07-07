// Resource group management.
//
// ResourceGroupClient wraps Azure's armresources ResourceGroupsClient and
// exposes existence checks and the small CRUD surface the remote-state
// bootstrap needs (create-if-missing, delete).

package azurehelper

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ResourceGroupClient wraps the Azure resource-groups management API.
type ResourceGroupClient struct {
	client         *armresources.ResourceGroupsClient
	subscriptionID string
}

// NewResourceGroupClient creates a resource-groups client. This requires a
// token credential; SAS-token and access-key configs are data-plane only.
func NewResourceGroupClient(cfg *AzureConfig) (*ResourceGroupClient, error) {
	if cfg == nil {
		return nil, ErrAzureConfigRequired
	}

	if cfg.SubscriptionID == "" {
		return nil, ErrSubscriptionIDRequired
	}

	if cfg.Credential == nil {
		return nil, &UnsupportedAuthForOpError{Method: cfg.Method, Operation: "resource group operations"}
	}

	client, err := armresources.NewResourceGroupsClient(cfg.SubscriptionID, cfg.Credential, &arm.ClientOptions{
		ClientOptions: cfg.ClientOptions,
	})
	if err != nil {
		return nil, fmt.Errorf("creating resource groups client: %w", err)
	}

	return &ResourceGroupClient{client: client, subscriptionID: cfg.SubscriptionID}, nil
}

// Exists reports whether the named resource group exists in the subscription.
func (c *ResourceGroupClient) Exists(ctx context.Context, name string) (bool, error) {
	if name == "" {
		return false, ErrResourceGroupNameRequired
	}

	resp, err := c.client.CheckExistence(ctx, name, nil)
	if err != nil {
		if IsNotFound(err) {
			return false, nil
		}

		return false, fmt.Errorf("checking resource group existence: %w", err)
	}

	return resp.Success, nil
}

// EnsureResourceGroup creates a resource group at location if it does not
// already exist. location is required when the resource group does not
// yet exist; an empty location yields an error rather than a silent
// geographic default, mirroring StorageAccountClient.Create.
func (c *ResourceGroupClient) EnsureResourceGroup(ctx context.Context, l log.Logger, name, location string) error {
	if name == "" {
		return ErrResourceGroupNameRequired
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
		return fmt.Errorf("%w %q", ErrLocationRequiredForRG, name)
	}

	_, err = c.client.CreateOrUpdate(ctx, name, armresources.ResourceGroup{
		Location: &location,
	}, nil)
	if err != nil {
		return fmt.Errorf("creating resource group %s: %w", name, err)
	}

	l.Debugf("azurehelper: created resource group %s in %s", name, location)

	return nil
}

// EnsureDeleted deletes the named resource group and waits for the
// long-running operation to complete. Idempotent: missing resource groups
// return nil.
func (c *ResourceGroupClient) EnsureDeleted(ctx context.Context, l log.Logger, name string) error {
	if name == "" {
		return ErrResourceGroupNameRequired
	}

	poller, err := c.client.BeginDelete(ctx, name, nil)
	if err != nil {
		if IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("starting resource group delete %s: %w", name, err)
	}

	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		if IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("waiting for resource group delete %s: %w", name, err)
	}

	l.Debugf("azurehelper: deleted resource group %s", name)

	return nil
}
