// Package azurehelper provides Azure-specific helper functions
package azurehelper

import (
	"context"
	"fmt"
	"regexp"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ResourceGroupClient wraps Azure's armresources client to provide a simpler interface.
type ResourceGroupClient struct {
	client         *armresources.ResourceGroupsClient
	subscriptionID string
}

// ResourceGroupConfig represents the configuration for an Azure Resource Group.
type ResourceGroupConfig struct {
	// Put map field first (larger alignment requirements)
	Tags map[string]string

	// Then string fields in alphabetical order
	Location          string
	ResourceGroupName string
	SubscriptionID    string
}

// CreateResourceGroupClient creates a new ResourceGroup client
func CreateResourceGroupClient(ctx context.Context, l log.Logger, subscriptionID string) (*ResourceGroupClient, error) {
	// Validate subscription ID format
	matched, err := regexp.MatchString(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to validate subscription ID format: %w", err)
	}

	if !matched {
		return nil, errors.Errorf("invalid subscription ID format: %s", subscriptionID)
	}

	if subscriptionID == "" {
		// Try to get subscription ID from environment variables
		_, envSubscriptionID, err := GetAzureCredentials(ctx, l)
		if err != nil {
			return nil, fmt.Errorf("error getting azure credentials: %w", err)
		}

		if envSubscriptionID != "" {
			subscriptionID = envSubscriptionID
			l.Infof("Using subscription ID from environment: %s", subscriptionID)
		} else {
			return nil, errors.Errorf("subscription_id is required either in configuration or as an environment variable")
		}
	}

	// Get Azure credentials
	cred, _, err := GetAzureCredentials(ctx, l)
	if err != nil {
		return nil, fmt.Errorf("error getting azure credentials: %w", err)
	}

	// Create resource group client
	resourceGroupClient, err := armresources.NewResourceGroupsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating resource group client: %w", err)
	}

	return &ResourceGroupClient{
		client:         resourceGroupClient,
		subscriptionID: subscriptionID,
	}, nil
}

// EnsureResourceGroup creates a resource group if it doesn't exist
func (c *ResourceGroupClient) EnsureResourceGroup(ctx context.Context, l log.Logger, resourceGroupName, location string, tags map[string]string) error {
	l.Infof("Ensuring resource group %s exists in %s", resourceGroupName, location)

	// Check if resource group exists
	_, err := c.client.Get(ctx, resourceGroupName, nil)
	if err == nil {
		l.Infof("Resource group %s already exists", resourceGroupName)
		return nil
	}

	// If it doesn't exist, create it
	l.Infof("Creating resource group %s in %s", resourceGroupName, location)

	// Convert tags to Azure SDK format
	azureTags := make(map[string]*string)

	for k, v := range tags {
		value := v // Create a new variable to avoid capturing the loop variable
		azureTags[k] = to.Ptr(value)
	}

	// Set default tag if none provided
	if len(azureTags) == 0 {
		azureTags["created-by"] = to.Ptr("terragrunt")
	}

	resourceGroup := armresources.ResourceGroup{
		Location: to.Ptr(location),
		Tags:     azureTags,
	}

	_, err = c.client.CreateOrUpdate(ctx, resourceGroupName, resourceGroup, nil)
	if err != nil {
		return fmt.Errorf("error creating resource group: %w", err)
	}

	l.Infof("Successfully created resource group %s", resourceGroupName)

	return nil
}

// DeleteResourceGroup deletes a resource group
func (c *ResourceGroupClient) DeleteResourceGroup(ctx context.Context, l log.Logger, resourceGroupName string) error {
	l.Infof("Deleting resource group %s", resourceGroupName)

	// Check if it exists before deleting
	_, err := c.client.Get(ctx, resourceGroupName, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == 404 {
			// Resource group doesn't exist, nothing to delete
			l.Infof("Resource group %s doesn't exist, nothing to delete", resourceGroupName)
			return nil
		}
		// Return any other error
		return fmt.Errorf("error checking resource group existence: %w", err)
	}

	// Start the delete operation
	poller, err := c.client.BeginDelete(ctx, resourceGroupName, nil)
	if err != nil {
		return fmt.Errorf("error starting resource group deletion: %w", err)
	}

	// Wait for the delete operation to complete
	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("error deleting resource group: %w", err)
	}

	l.Infof("Successfully deleted resource group %s", resourceGroupName)

	return nil
}

// GetResourceGroup gets information about a resource group
func (c *ResourceGroupClient) GetResourceGroup(ctx context.Context, resourceGroupName string) (*armresources.ResourceGroup, error) {
	resp, err := c.client.Get(ctx, resourceGroupName, nil)
	if err != nil {
		return nil, fmt.Errorf("error getting resource group: %w", err)
	}

	return &resp.ResourceGroup, nil
}

// ResourceGroupExists checks if a resource group exists
func (c *ResourceGroupClient) ResourceGroupExists(ctx context.Context, resourceGroupName string) (bool, error) {
	_, err := c.client.Get(ctx, resourceGroupName, nil)
	if err != nil {
		// Check if the error is a "not found" error
		if IsNotFoundError(err) {
			return false, nil
		}

		return false, fmt.Errorf("error checking if resource group exists: %w", err)
	}

	return true, nil
}

// IsNotFoundError checks if the error is a "not found" error
func IsNotFoundError(err error) bool {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		return respErr.StatusCode == httpStatusNotFound
	}

	return false
}

// Validate checks if all required fields are set
func (cfg ResourceGroupConfig) Validate() error {
	if cfg.SubscriptionID == "" {
		return errors.Errorf("subscription_id is required")
	}

	if cfg.ResourceGroupName == "" {
		return errors.Errorf("resource_group_name is required")
	}

	// Azure resource group name must not exceed 90 characters
	if len(cfg.ResourceGroupName) > 90 { //nolint: mnd
		return errors.Errorf("resource_group_name exceeds maximum length (90 characters)")
	}

	if cfg.Location == "" {
		return errors.Errorf("location is required")
	}

	// Azure location must match allowed format: only letters, numbers, and hyphens
	matched, err := regexp.MatchString(`^[a-zA-Z0-9-]+$`, cfg.Location)
	if err != nil {
		return errors.Errorf("failed to validate location format: %v", err)
	}

	if !matched {
		return errors.Errorf("invalid location format")
	}

	return nil
}
