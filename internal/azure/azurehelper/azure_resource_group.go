// Package azurehelper provides Azure-specific helper functions
package azurehelper

import (
	"context"
	"fmt"
	"regexp"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/gruntwork-io/terragrunt/internal/azure/azureauth"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ResourceGroupClient wraps Azure's armresources client to provide a simpler interface.
type ResourceGroupClient struct {
	client         *armresources.ResourceGroupsClient
	subscriptionID string
}

// ResourceGroupConfig represents the configuration for an Azure Resource Group.
//
// Resource groups are logical containers for Azure resources that share the same
// lifecycle, permissions, and policies. They provide a way to manage and organize
// related Azure resources together.
//
// Required Fields:
//   - SubscriptionID: Must be a valid Azure subscription UUID
//   - ResourceGroupName: Must be 1-90 characters, alphanumeric, periods, underscores, hyphens, and parentheses
//   - Location: Must be a valid Azure region (e.g., "eastus", "westeurope")
//
// Naming Constraints:
//   - ResourceGroupName cannot end with a period
//   - Special characters allowed: periods, underscores, hyphens, parentheses
//   - Cannot contain spaces or other special characters
//
// Location Considerations:
//   - Resource group location is used for metadata storage
//   - Resources within the group can be in different regions
//   - Some Azure policies may require resources to be in the same region as the group
//
// Tags Usage:
//   - Applied to all resources in the group by default (inheritance)
//   - Used for cost tracking, automation, and governance
//   - Maximum 50 tags per resource group
//   - Each tag key and value limited to 512 characters
//
// Examples:
//
//	// Basic resource group for development
//	config := ResourceGroupConfig{
//	    SubscriptionID:    "12345678-1234-1234-1234-123456789abc",
//	    ResourceGroupName: "dev-terragrunt-rg",
//	    Location:          "eastus",
//	    Tags: map[string]string{
//	        "environment": "development",
//	        "team":        "platform",
//	        "created-by":  "terragrunt",
//	    },
//	}
//
//	// Production resource group with comprehensive tagging
//	config := ResourceGroupConfig{
//	    SubscriptionID:    "12345678-1234-1234-1234-123456789abc",
//	    ResourceGroupName: "prod-app-rg",
//	    Location:          "eastus",
//	    Tags: map[string]string{
//	        "environment":   "production",
//	        "application":   "web-app",
//	        "team":          "platform",
//	        "cost-center":   "engineering",
//	        "owner":         "platform-team@company.com",
//	        "backup-policy": "daily",
//	        "created-by":    "terragrunt",
//	    },
//	}
//
//	// Minimal configuration (tags are optional)
//	config := ResourceGroupConfig{
//	    SubscriptionID:    "12345678-1234-1234-1234-123456789abc",
//	    ResourceGroupName: "simple-rg",
//	    Location:          "westus2",
//	}
type ResourceGroupConfig struct {
	// Tags represents custom metadata applied to the resource group.
	// These tags are inherited by all resources created within the group by default.
	// Used for organization, cost tracking, automation, and compliance.
	// Maximum 50 tags allowed per resource group.
	// Keys and values must each be 512 characters or less.
	// Optional: Can be nil or empty map.
	Tags map[string]string

	// Location specifies the Azure region where the resource group's metadata will be stored.
	// Must be a valid Azure region name (e.g., "eastus", "westeurope", "southeastasia").
	// The resource group location determines where metadata about the group is stored,
	// but resources within the group can be deployed to different regions.
	// Consider data residency and compliance requirements when choosing location.
	// Required field.
	Location string

	// ResourceGroupName specifies the name of the Azure resource group.
	// Must be 1-90 characters long.
	// Can contain alphanumeric characters, periods, underscores, hyphens, and parentheses.
	// Cannot end with a period.
	// Must be unique within the subscription.
	// Once created, the name cannot be changed.
	// Required field.
	ResourceGroupName string

	// SubscriptionID specifies the Azure subscription ID where the resource group will be created.
	// Must be a valid UUID format (e.g., "12345678-1234-1234-1234-123456789abc").
	// The subscription determines billing, access control, and quota limits.
	// Required field.
	SubscriptionID string
}

// CreateResourceGroupClient creates a new ResourceGroup client
func CreateResourceGroupClient(ctx context.Context, l log.Logger, subscriptionID string) (*ResourceGroupClient, error) {
	// Create config with subscription ID if provided
	config := make(map[string]interface{})
	if subscriptionID != "" {
		config["subscription_id"] = subscriptionID
	}

	// Get auth config once - this retrieves subscription ID from environment if not provided
	authConfig, err := azureauth.GetAuthConfig(ctx, l, config)
	if err != nil {
		return nil, fmt.Errorf("error getting azure auth config: %w", err)
	}

	// Use subscription ID from auth config if we didn't have one
	if subscriptionID == "" {
		if authConfig.SubscriptionID != "" {
			subscriptionID = authConfig.SubscriptionID
			l.Infof("Using subscription ID from auth config: %s", subscriptionID)
		} else {
			return nil, errors.Errorf("subscription_id is required either in configuration or as an environment variable")
		}
	}

	// Validate subscription ID format
	matched, err := regexp.MatchString(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to validate subscription ID format: %w", err)
	}

	if !matched {
		return nil, errors.Errorf("invalid subscription ID format: %s", subscriptionID)
	}

	// Get Azure credentials using the auth config we already retrieved
	authResult, err := azureauth.GetTokenCredential(ctx, l, authConfig)
	if err != nil {
		return nil, fmt.Errorf("error getting azure credentials: %w", err)
	}

	// Create resource group client
	resourceGroupClient, err := armresources.NewResourceGroupsClient(subscriptionID, authResult.Credential, nil)
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
