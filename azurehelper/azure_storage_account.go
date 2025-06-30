// Package azurehelper provides Azure-specific helper functions
package azurehelper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/google/uuid"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// StorageAccountClient wraps Azure's armstorage client to provide a simpler interface
type StorageAccountClient struct {
	client               *armstorage.AccountsClient
	blobClient           *armstorage.BlobServicesClient
	roleAssignmentClient *armauthorization.RoleAssignmentsClient
	subscriptionID       string
	resourceGroupName    string
	storageAccountName   string
	location             string
	config               map[string]interface{}

	defaultAccountKind     string
	defaultAccountTier     string
	defaultAccountSKU      string
	defaultReplicationType string
}

// StorageAccountConfig represents the configuration for an Azure Storage Account.
// It contains all the necessary parameters to create or update a storage account.
type StorageAccountConfig struct {
	// Put map field first (larger alignment requirements)
	Tags map[string]string

	// String fields in alphabetical order
	AccessTier         string // Storage tier (Hot/Cool)
	AccountKind        string // Kind of storage account (e.g., StorageV2, BlobStorage)
	AccountSKU         string // SKU name for the storage account
	AccountTier        string // Performance tier (Standard/Premium)
	KeyEncryptionKey   string // Source of encryption key (e.g., "Microsoft.KeyVault")
	Location           string // Azure region where the storage account exists/will be created
	ReplicationType    string // Type of replication (LRS/GRS/etc)
	ResourceGroupName  string // Name of the resource group containing the storage account
	StorageAccountName string // Name of the storage account
	SubscriptionID     string // Azure subscription ID where the storage account exists

	// Boolean fields at the end
	AllowBlobPublicAccess bool // Whether to allow public access to blobs (not recommended)
	EnableVersioning      bool // Whether to enable blob versioning
}

// DefaultStorageAccountConfig returns the default configuration for a storage account
func DefaultStorageAccountConfig() StorageAccountConfig {
	return StorageAccountConfig{
		EnableVersioning:      true, // Blob versioning enabled by default
		AllowBlobPublicAccess: false,
		AccountKind:           "StorageV2",
		AccountTier:           "Standard",
		AccessTier:            AccessTierHot,
		ReplicationType:       "LRS",
		Tags:                  map[string]string{"created-by": "terragrunt"},
	}
}

// CreateStorageAccountClient creates a new StorageAccount client
func CreateStorageAccountClient(ctx context.Context, l log.Logger, config map[string]interface{}) (*StorageAccountClient, error) {
	if config == nil {
		return nil, errors.Errorf("config is required")
	}

	// Extract configuration values
	storageAccountName, ok := config["storage_account_name"].(string)
	if !ok || storageAccountName == "" {
		return nil, errors.Errorf("storage_account_name is required")
	}

	// Check if resource group is specified
	resourceGroupName, _ := config["resource_group_name"].(string)
	if resourceGroupName == "" {
		l.Warn("No resource_group_name specified in config, using storage account name as resource group")

		resourceGroupName = storageAccountName + "-rg"
	}

	// Extract subscription ID if provided in config
	subscriptionID, _ := config["subscription_id"].(string)
	location, _ := config["location"].(string)

	// Get Azure credentials, checking environment variables first
	cred, envSubscriptionID, err := GetAzureCredentials(ctx, l)
	if err != nil {
		return nil, fmt.Errorf("error getting azure credentials: %w", err)
	}

	// Use environment subscription ID if not provided in config
	if subscriptionID == "" && envSubscriptionID != "" {
		subscriptionID = envSubscriptionID

		l.Infof("Using subscription ID from environment: %s", envSubscriptionID)
	}

	// Still need a subscription ID at this point
	if subscriptionID == "" {
		return nil, errors.Errorf("subscription_id is required either:\n" +
			"  1. In the configuration as 'subscription_id'\n" +
			"  2. As an environment variable (AZURE_SUBSCRIPTION_ID or ARM_SUBSCRIPTION_ID)\n" +
			"Please provide at least one of these values to continue")
	}

	// Create storage accounts client
	accountsClient, err := armstorage.NewAccountsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, errors.Errorf("error creating storage accounts client: %w", err)
	}

	// Create blob services client
	blobClient, err := armstorage.NewBlobServicesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, errors.Errorf("error creating blob services client: %w", err)
	}

	// Create role assignments client with the latest API version
	// Azure requires at least API version 2018-01-01-preview for roles with data actions
	clientOptions := &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			APIVersion: defaultRoleAssignmentAPIVersion, // Use the latest API version for role assignments
		},
	}

	roleAssignmentClient, err := armauthorization.NewRoleAssignmentsClient(subscriptionID, cred, clientOptions)
	if err != nil {
		return nil, errors.Errorf("error creating role assignments client: %w", err)
	}

	return &StorageAccountClient{
		client:                 accountsClient,
		blobClient:             blobClient,
		roleAssignmentClient:   roleAssignmentClient,
		subscriptionID:         subscriptionID,
		resourceGroupName:      resourceGroupName,
		storageAccountName:     storageAccountName,
		location:               location,
		config:                 config,
		defaultAccountKind:     "StorageV2",
		defaultAccountTier:     "Standard",
		defaultAccountSKU:      "Standard_LRS",
		defaultReplicationType: "Standard_LRS",
	}, nil
}

// StorageAccountExists checks if a storage account exists
func (c *StorageAccountClient) StorageAccountExists(ctx context.Context) (bool, *armstorage.Account, error) {
	if c.storageAccountName == "" {
		return false, nil, errors.Errorf("storage account name is required")
	}

	resp, err := c.client.GetProperties(ctx, c.resourceGroupName, c.storageAccountName, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			if respErr.StatusCode == httpStatusNotFound {
				return false, nil, nil
			}

			return false, nil, errors.Errorf("error checking storage account existence: %w", err)
		}

		return false, nil, errors.Errorf("error checking storage account existence: %w", err)
	}

	return true, &resp.Account, nil
}

// GetStorageAccountVersioning checks if versioning is enabled on a storage account
func (c *StorageAccountClient) GetStorageAccountVersioning(ctx context.Context) (bool, error) {
	resp, err := c.blobClient.GetServiceProperties(ctx, c.resourceGroupName, c.storageAccountName, nil)
	if err != nil {
		return false, errors.Errorf("error getting storage account blob service properties: %w", err)
	}

	// Check if blob service properties exist and have versioning information
	if resp.BlobServiceProperties.BlobServiceProperties == nil {
		return false, nil
	}

	// Return the actual versioning status from the Azure SDK
	if resp.BlobServiceProperties.BlobServiceProperties.IsVersioningEnabled != nil {
		return *resp.BlobServiceProperties.BlobServiceProperties.IsVersioningEnabled, nil
	}

	// Default to false if versioning status is not set
	return false, nil
}

// listAndUpdateVersioning is a helper to get service properties, set IsVersioningEnabled, and update.
func (c *StorageAccountClient) listAndUpdateVersioning(ctx context.Context, enable bool) error {
	// Get current service properties to preserve other settings
	resp, err := c.blobClient.GetServiceProperties(ctx, c.resourceGroupName, c.storageAccountName, nil)
	if err != nil {
		return errors.Errorf("error getting current blob service properties: %w", err)
	}

	if resp.BlobServiceProperties.BlobServiceProperties == nil {
		resp.BlobServiceProperties.BlobServiceProperties = &armstorage.BlobServicePropertiesProperties{}
	}

	resp.BlobServiceProperties.BlobServiceProperties.IsVersioningEnabled = to.Ptr(enable)
	_, err = c.blobClient.SetServiceProperties(ctx, c.resourceGroupName, c.storageAccountName, resp.BlobServiceProperties, nil)

	if err != nil {
		return errors.Errorf("failed to set versioning on storage account %s: %w", c.storageAccountName, err)
	}

	return nil
}

func (c *StorageAccountClient) EnableStorageAccountVersioning(ctx context.Context, l log.Logger) error {
	l.Infof("Enabling versioning on storage account %s", c.storageAccountName)
	err := c.listAndUpdateVersioning(ctx, true)

	if err != nil {
		return err
	}

	l.Info("Successfully enabled versioning on storage account")

	return nil
}

func (c *StorageAccountClient) DisableStorageAccountVersioning(ctx context.Context, l log.Logger) error {
	l.Infof("Disabling versioning on storage account %s", c.storageAccountName)
	err := c.listAndUpdateVersioning(ctx, false)

	if err != nil {
		return err
	}

	l.Info("Successfully disabled versioning on storage account")

	return nil
}

// CreateStorageAccountIfNecessary creates a storage account if it doesn't exist
func (c *StorageAccountClient) CreateStorageAccountIfNecessary(ctx context.Context, l log.Logger, config StorageAccountConfig) error {
	// Use provided location or default
	location := config.Location
	if location == "" {
		location = c.location
		if location == "" {
			location = defaultLocation // Default location
			l.Warnf("No location specified, using default location: %s", location)
		}
	}

	// Ensure resource group exists
	if err := c.EnsureResourceGroup(ctx, l, location); err != nil {
		return err
	}

	// Check if storage account exists
	exists, account, err := c.StorageAccountExists(ctx)
	if err != nil {
		return err
	}

	if !exists {
		// Create storage account
		return c.createStorageAccount(ctx, l, config)
	}

	// If the account exists, check if settings match and update if needed
	return c.updateStorageAccountIfNeeded(ctx, l, config, account)
}

// createStorageAccount creates a new storage account
func (c *StorageAccountClient) createStorageAccount(ctx context.Context, l log.Logger, config StorageAccountConfig) error {
	// Default to Standard_LRS replication if not specified
	sku := armstorage.SKUNameStandardLRS

	l.Infof("Creating Azure Storage account %s in resource group %s", c.storageAccountName, c.resourceGroupName)

	// Map replication type if specified
	if config.ReplicationType != "" {
		switch config.ReplicationType {
		case "LRS":
			sku = armstorage.SKUNameStandardLRS
		case "GRS":
			sku = armstorage.SKUNameStandardGRS
		case "RAGRS":
			sku = armstorage.SKUNameStandardRAGRS
		case "ZRS":
			sku = armstorage.SKUNameStandardZRS
		case "GZRS":
			sku = armstorage.SKUNameStandardGZRS
		case "RAGZRS":
			sku = armstorage.SKUNameStandardRAGZRS
		default:
			l.Warnf("Unsupported replication type %s, using Standard_LRS", config.ReplicationType)
		}
	}

	// Map account kind if specified
	kind := armstorage.KindStorageV2

	if config.AccountKind != "" {
		switch config.AccountKind {
		case "StorageV2":
			kind = armstorage.KindStorageV2
		case "Storage":
			kind = armstorage.KindStorage
		case "BlobStorage":
			kind = armstorage.KindBlobStorage
		case "BlockBlobStorage":
			kind = armstorage.KindBlockBlobStorage
		case "FileStorage":
			kind = armstorage.KindFileStorage
		default:
			l.Warnf("Unsupported account kind %s, using StorageV2", config.AccountKind)
		}
	}

	// Map access tier if specified
	accessTierStr := config.AccessTier
	if accessTierStr == "" {
		accessTierStr = AccessTierHot // Default
	}

	switch accessTierStr {
	case AccessTierHot, AccessTierCool, AccessTierPremium:
		// Valid tier
	default:
		l.Warnf("Unsupported access tier %s, using Hot", accessTierStr)
		accessTierStr = AccessTierHot
	}

	// Convert tags map to pointer map
	l.Infof("Using access tier: %s", accessTierStr)

	tags := make(map[string]*string, len(config.Tags))

	if len(config.Tags) > 0 {
		for k, v := range config.Tags {
			value := v // Create a new variable to avoid capturing the loop variable
			tags[k] = &value
		}
	} else {
		// Set default tags if none provided
		defaultTag := "terragrunt"
		tags["created-by"] = &defaultTag
	}

	// Use provided location or default
	location := config.Location
	if location == "" {
		location = c.location
		if location == "" {
			location = defaultLocation // Default location
			l.Warnf("No location specified, using default location: %s", location)
		}
	}

	// Note: The actual structure depends on the SDK version
	// This is a simplified version that should work with most SDK versions
	parameters := armstorage.AccountCreateParameters{
		SKU: &armstorage.SKU{
			Name: &sku,
		},
		Kind: &kind,
		// Properties are set directly on AccountCreateParameters in some SDK versions
		Location: to.Ptr(location),
		Tags:     tags,
	}

	// Set properties for the storage account
	var accessTier *armstorage.AccessTier

	switch accessTierStr {
	case AccessTierHot:
		accessTier = to.Ptr(armstorage.AccessTierHot)
	case AccessTierCool:
		accessTier = to.Ptr(armstorage.AccessTierCool)
	case AccessTierPremium:
		accessTier = to.Ptr(armstorage.AccessTierPremium)
	}

	// Create properties object
	parameters.Properties = &armstorage.AccountPropertiesCreateParameters{
		EnableHTTPSTrafficOnly: to.Ptr(true),
		MinimumTLSVersion:      to.Ptr(armstorage.MinimumTLSVersionTLS12),
		AllowBlobPublicAccess:  to.Ptr(config.AllowBlobPublicAccess),
		AccessTier:             accessTier,
		// Add more properties as needed based on your requirements
	}

	l.Infof("Creating storage account %s in %s (Kind: %s, SKU: %s)",
		c.storageAccountName, location, *parameters.Kind, *parameters.SKU.Name)

	pollerResp, err := c.client.BeginCreate(ctx, c.resourceGroupName, c.storageAccountName, parameters, nil)
	if err != nil {
		return errors.Errorf("error creating storage account: %w", err)
	}

	_, err = pollerResp.PollUntilDone(ctx, nil)
	if err != nil {
		return errors.Errorf("error waiting for storage account creation: %w", err)
	}

	l.Infof("Successfully created storage account %s", c.storageAccountName)

	// Assign Storage Blob Data Owner role to the current user
	err = c.AssignStorageBlobDataOwnerRole(ctx, l)
	if err != nil {
		l.Warnf("Failed to assign Storage Blob Data Owner role: %v", err)
	}

	// If versioning is enabled, enable it on the storage account
	if config.EnableVersioning {
		err = c.EnableStorageAccountVersioning(ctx, l)
		if err != nil {
			return err
		}
	}

	return nil
}

// updateStorageAccountIfNeeded updates a storage account if settings don't match
func (c *StorageAccountClient) updateStorageAccountIfNeeded(ctx context.Context, l log.Logger, config StorageAccountConfig, account *armstorage.Account) error {
	var needsUpdate bool

	var updateParams armstorage.AccountUpdateParameters

	updateParams.Properties = &armstorage.AccountPropertiesUpdateParameters{}

	// Check updatable properties first

	// 1. Check AllowBlobPublicAccess
	if account.Properties.AllowBlobPublicAccess != nil &&
		*account.Properties.AllowBlobPublicAccess != config.AllowBlobPublicAccess {
		needsUpdate = true
		updateParams.Properties.AllowBlobPublicAccess = to.Ptr(config.AllowBlobPublicAccess)
		l.Infof("Updating AllowBlobPublicAccess from %t to %t on storage account %s",
			*account.Properties.AllowBlobPublicAccess, config.AllowBlobPublicAccess, c.storageAccountName)
	}

	// 2. Check AccessTier
	if !CompareAccessTier(account.Properties.AccessTier, config.AccessTier) && config.AccessTier != "" {
		needsUpdate = true

		var accessTier *armstorage.AccessTier

		switch config.AccessTier {
		case AccessTierHot:
			accessTier = to.Ptr(armstorage.AccessTierHot)
		case AccessTierCool:
			accessTier = to.Ptr(armstorage.AccessTierCool)
		case AccessTierPremium:
			accessTier = to.Ptr(armstorage.AccessTierPremium)
		default:
			l.Warnf("Unsupported access tier %s, skipping update", config.AccessTier)
		}

		if accessTier != nil {
			updateParams.Properties.AccessTier = accessTier
			currentTier := "Unknown"

			if account.Properties.AccessTier != nil {
				currentTier = string(*account.Properties.AccessTier)
			}

			l.Infof("Updating AccessTier from %s to %s on storage account %s", currentTier, config.AccessTier, c.storageAccountName)
		}
	}

	// 3. Check Tags
	if !CompareStringMaps(account.Tags, config.Tags) && len(config.Tags) > 0 {
		needsUpdate = true
		updateParams.Tags = ConvertToPointerMap(config.Tags)

		l.Infof("Updating tags on storage account %s", c.storageAccountName)
	}

	// Check read-only properties and warn if they differ

	// Check SKU/ReplicationType (read-only after creation)
	if account.SKU != nil && config.ReplicationType != "" {
		currentSKU := string(*account.SKU.Name)
		expectedSKU, _ := GetStorageAccountSKU(config.AccountTier, config.ReplicationType)

		if currentSKU != expectedSKU {
			l.Warnf("Storage account SKU cannot be changed after creation. Current: %s, Desired: %s",
				currentSKU, expectedSKU)
		}
	}

	// Check AccountKind (read-only after creation)
	if account.Kind != nil && config.AccountKind != "" {
		currentKind := string(*account.Kind)
		if currentKind != config.AccountKind {
			l.Warnf("Storage account kind cannot be changed after creation. Current: %s, Desired: %s",
				currentKind, config.AccountKind)
		}
	}

	// Check Location (read-only after creation)
	if account.Location != nil && config.Location != "" {
		if *account.Location != config.Location {
			l.Warnf("Storage account location cannot be changed after creation. Current: %s, Desired: %s",
				*account.Location, config.Location)
		}
	}

	// Apply updates if needed
	if needsUpdate {
		l.Infof("Updating storage account %s with new properties", c.storageAccountName)

		_, err := c.client.Update(ctx, c.resourceGroupName, c.storageAccountName, updateParams, nil)
		if err != nil {
			return errors.Errorf("error updating storage account properties: %w", err)
		}

		l.Infof("Successfully updated storage account %s", c.storageAccountName)
	} else {
		l.Infof("Storage account %s properties are already up to date", c.storageAccountName)
	}

	// Handle versioning separately (as it's a blob service property, not account property)
	isVersioningEnabled, err := c.GetStorageAccountVersioning(ctx)
	if err != nil {
		return err
	}

	// Only update versioning if it doesn't match expected state
	if config.EnableVersioning && !isVersioningEnabled {
		l.Infof("Enabling versioning on existing storage account %s", c.storageAccountName)

		if err := c.EnableStorageAccountVersioning(ctx, l); err != nil {
			return err
		}
	} else if !config.EnableVersioning && isVersioningEnabled {
		l.Infof("Disabling versioning on existing storage account %s", c.storageAccountName)

		if err := c.DisableStorageAccountVersioning(ctx, l); err != nil {
			return err
		}
	}

	return nil
}

// DeleteStorageAccount deletes a storage account
func (c *StorageAccountClient) DeleteStorageAccount(ctx context.Context, l log.Logger) error {
	l.Infof("Deleting storage account %s in resource group %s", c.storageAccountName, c.resourceGroupName)

	// First check if the storage account exists
	_, err := c.client.GetProperties(ctx, c.resourceGroupName, c.storageAccountName, nil)
	if err != nil {
		// If 404, it's already deleted
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == httpStatusNotFound {
			l.Infof("Storage account %s does not exist or is already deleted", c.storageAccountName)
			return nil
		}

		return errors.Errorf("error checking storage account: %w", err)
	}

	// Delete the storage account
	_, err = c.client.Delete(ctx, c.resourceGroupName, c.storageAccountName, nil)
	if err != nil {
		return errors.Errorf("error deleting storage account: %w", err)
	}

	l.Infof("Successfully deleted storage account %s", c.storageAccountName)

	return nil
}

// EnsureResourceGroup creates a resource group if it doesn't exist
func (c *StorageAccountClient) EnsureResourceGroup(ctx context.Context, l log.Logger, location string) error {
	l.Infof("Ensuring resource group %s exists in %s", c.resourceGroupName, location)
	resourceGroupClient, err := CreateResourceGroupClient(ctx, l, c.subscriptionID)

	if err != nil {
		return fmt.Errorf("error creating resource group client: %w", err)
	}

	// Default tags to use if not specified
	tags := map[string]string{
		"created-by": "terragrunt",
	}

	// Ensure the resource group exists
	err = resourceGroupClient.EnsureResourceGroup(ctx, l, c.resourceGroupName, location, tags)
	if err != nil {
		return fmt.Errorf("error ensuring resource group exists: %w", err)
	}

	return nil
}

// getCurrentUserObjectID gets the object ID of the current authenticated user
func (c *StorageAccountClient) getCurrentUserObjectID(ctx context.Context) (string, error) {
	// For service principals and managed identities, we can get the object ID from environment variables
	if objectID := os.Getenv("AZURE_CLIENT_OBJECT_ID"); objectID != "" {
		return objectID, nil
	}

	// Try to get from other common environment variables
	if objectID := os.Getenv("ARM_CLIENT_OBJECT_ID"); objectID != "" {
		return objectID, nil
	}

	// If no environment variables are set, try to get from Microsoft Graph API
	objectID, err := c.getUserObjectIDFromGraphAPI(ctx)
	if err == nil && objectID != "" {
		return objectID, nil
	}

	// If all else fails, return an error
	return "", fmt.Errorf("could not determine current user object ID. Please set AZURE_CLIENT_OBJECT_ID or ARM_CLIENT_OBJECT_ID environment variable with your user/service principal object ID. Graph API error: %w", err)
}

// getUserObjectIDFromGraphAPI gets the current user's object ID from Microsoft Graph API
func (c *StorageAccountClient) getUserObjectIDFromGraphAPI(ctx context.Context) (string, error) {
	// Get credentials for Microsoft Graph API
	cred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{})
	if err != nil {
		return "", errors.Errorf("error getting default azure credential: %w", err)
	}

	// Get an access token for Microsoft Graph API
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://graph.microsoft.com/.default"},
	})
	if err != nil {
		return "", errors.Errorf("error getting token for Microsoft Graph API: %w", err)
	}

	// Create HTTP client
	client := &http.Client{
		Timeout: defaultHTTPClientTimeout,
	}

	// Create request for Microsoft Graph API to get current user
	req, err := http.NewRequestWithContext(ctx, "GET", "https://graph.microsoft.com/v1.0/me", nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	// Add authorization header
	req.Header.Add("Authorization", "Bearer "+token.Token)
	req.Header.Add("Accept", "application/json")

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending request to Microsoft Graph API: %w", err)
	}
	// Simply handle the error properly by ignoring it in defer
	// This is sufficient to satisfy the errcheck linter
	defer func() {
		_ = resp.Body.Close() // explicitly ignoring the error
	}()

	// Check response status code
	if resp.StatusCode != httpStatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("error from Microsoft Graph API: %s - %s", resp.Status, string(bodyBytes))
	}

	// Parse response
	var graphResponse struct {
		ID string `json:"id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&graphResponse); err != nil {
		return "", fmt.Errorf("error decoding response from Microsoft Graph API: %w", err)
	}

	// Check if ID is empty
	if graphResponse.ID == "" {
		return "", errors.Errorf("microsoft graph API returned empty ID")
	}

	return graphResponse.ID, nil
}

// AssignStorageBlobDataOwnerRole assigns the Storage Blob Data Owner role to the current user
func (c *StorageAccountClient) AssignStorageBlobDataOwnerRole(ctx context.Context, l log.Logger) error {
	// Get current user object ID
	userObjectID, err := c.getCurrentUserObjectID(ctx)
	if err != nil {
		l.Warnf("Could not get current user object ID: %v. Skipping role assignment.", err)
		l.Info("To assign Storage Blob Data Owner role manually, use: az role assignment create --role 'Storage Blob Data Owner' --assignee <your-user-id> --scope /subscriptions/<sub-id>/resourceGroups/<rg>/providers/Microsoft.Storage/storageAccounts/<sa-name>")

		return nil // Don't fail the entire process
	}

	// Determine if this is a user or service principal
	isServicePrincipal := false
	if os.Getenv("AZURE_CLIENT_ID") != "" || os.Getenv("ARM_CLIENT_ID") != "" {
		isServicePrincipal = true

		l.Infof("Detected service principal authentication. Assigning role to service principal with object ID: %s", userObjectID)
	} else {
		l.Infof("Assigning Storage Blob Data Owner role to user with object ID: %s", userObjectID)
	}

	// Construct the storage account resource ID
	storageAccountResourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s",
		c.subscriptionID, c.resourceGroupName, c.storageAccountName)

	// Construct the role definition ID
	roleDefinitionID := fmt.Sprintf("/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/%s",
		c.subscriptionID, storageBlobDataOwnerRoleID)

	// Generate a proper UUID for the role assignment
	roleAssignmentID := GenerateUUID()

	// Log appropriate message based on principal type
	if isServicePrincipal {
		l.Infof("Assigning Storage Blob Data Owner role to service principal %s for storage account %s", userObjectID, c.storageAccountName)
	} else {
		l.Infof("Assigning Storage Blob Data Owner role to user %s for storage account %s", userObjectID, c.storageAccountName)
	}

	// Create role assignment
	roleAssignment := armauthorization.RoleAssignmentCreateParameters{
		Properties: &armauthorization.RoleAssignmentProperties{
			RoleDefinitionID: to.Ptr(roleDefinitionID),
			PrincipalID:      to.Ptr(userObjectID),
			// PrincipalType isn't a supported field in the RoleAssignmentProperties struct
			// Azure will automatically determine the principal type based on the principal ID
		},
	}

	// Add debug logging to help diagnose issues
	l.Debugf("Creating role assignment with ID: %s", roleAssignmentID)
	l.Debugf("Role definition ID: %s", roleDefinitionID)
	l.Debugf("Storage account resource ID: %s", storageAccountResourceID)

	// Create the role assignment
	_, err = c.roleAssignmentClient.Create(ctx, storageAccountResourceID, roleAssignmentID, roleAssignment, nil)
	if err != nil {
		// Check if the role assignment already exists
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == httpStatusConflict {
			if isServicePrincipal {
				l.Infof("Storage Blob Data Owner role already assigned to service principal %s", userObjectID)
			} else {
				l.Infof("Storage Blob Data Owner role already assigned to user %s", userObjectID)
			}

			return nil
		}

		// Check for permission issues
		if errors.As(err, &respErr) && (respErr.StatusCode == httpStatusForbidden || respErr.StatusCode == httpStatusUnauthorized) {
			l.Warnf("Permission denied when assigning Storage Blob Data Owner role. Principal %s doesn't have sufficient permissions.", userObjectID)
			l.Info("To assign Storage Blob Data Owner role manually, use: az role assignment create --role 'Storage Blob Data Owner' --assignee <principal-id> --scope /subscriptions/<sub-id>/resourceGroups/<rg>/providers/Microsoft.Storage/storageAccounts/<sa-name>")

			return nil // Don't fail the entire process
		}

		// Check for specific error: InvalidRoleAssignmentId
		if errors.As(err, &respErr) && respErr.ErrorCode == "InvalidRoleAssignmentId" {
			l.Warnf("Invalid role assignment ID format. Status: %d, Error code: %s", respErr.StatusCode, respErr.ErrorCode)
			l.Debugf("Full error: %+v", respErr)

			// Try with a different format for the role assignment ID
			// Generate a more standard GUID format
			roleAssignmentID := fmt.Sprintf("%s-%s-4000-8000-%s",
				GenerateUUID()[0:8],
				GenerateUUID()[0:4],
				GenerateUUID()[0:12])

			l.Infof("Retrying with alternative role assignment ID format: %s", roleAssignmentID)
			_, retryErr := c.roleAssignmentClient.Create(ctx, storageAccountResourceID, roleAssignmentID, roleAssignment, nil)

			if retryErr == nil {
				l.Info("Successfully created role assignment with alternative ID format")

				return nil
			}

			l.Warnf("Retry also failed. Consider creating the role assignment manually: az role assignment create --role 'Storage Blob Data Owner' --assignee %s --scope %s",
				userObjectID, storageAccountResourceID)

			return nil // Don't fail the entire process
		}

		return errors.Errorf("error creating role assignment: %w", err)
	}

	if isServicePrincipal {
		l.Infof("Successfully assigned Storage Blob Data Owner role to service principal %s", userObjectID)
	} else {
		l.Infof("Successfully assigned Storage Blob Data Owner role to user %s", userObjectID)
	}

	return nil
}

// GenerateUUID generates a random RFC 4122 UUID using github.com/google/uuid.
func GenerateUUID() string {
	return uuid.NewString()
}

// GetAzureCredentials checks for Azure environment variables and returns appropriate credentials.
// If no environment variables are set, it attempts to use default authentication methods.
func GetAzureCredentials(ctx context.Context, l log.Logger) (*azidentity.DefaultAzureCredential, string, error) {
	// Check for common Azure environment variables
	var envVarsFound []string

	var subscriptionID string

	// First check for Azure CLI environment variables (these take precedence)
	if envVal := os.Getenv("AZURE_SUBSCRIPTION_ID"); envVal != "" {
		subscriptionID = envVal // AZURE_* takes precedence

		envVarsFound = append(envVarsFound, "AZURE_SUBSCRIPTION_ID")
	} else if envVal := os.Getenv("ARM_SUBSCRIPTION_ID"); envVal != "" {
		// Only use ARM_SUBSCRIPTION_ID if AZURE_SUBSCRIPTION_ID is not set
		subscriptionID = envVal

		envVarsFound = append(envVarsFound, "ARM_SUBSCRIPTION_ID")
	}

	// Check for tenant ID
	if envVal := os.Getenv("AZURE_TENANT_ID"); envVal != "" {
		envVarsFound = append(envVarsFound, "AZURE_TENANT_ID")
	} else if envVal := os.Getenv("ARM_TENANT_ID"); envVal != "" {
		envVarsFound = append(envVarsFound, "ARM_TENANT_ID")
	}

	// Check for client ID
	if envVal := os.Getenv("AZURE_CLIENT_ID"); envVal != "" {
		envVarsFound = append(envVarsFound, "AZURE_CLIENT_ID")
	} else if envVal := os.Getenv("ARM_CLIENT_ID"); envVal != "" {
		envVarsFound = append(envVarsFound, "ARM_CLIENT_ID")
	}

	// Check for client secret
	if envVal := os.Getenv("AZURE_CLIENT_SECRET"); envVal != "" {
		envVarsFound = append(envVarsFound, "AZURE_CLIENT_SECRET")
	} else if envVal := os.Getenv("ARM_CLIENT_SECRET"); envVal != "" {
		envVarsFound = append(envVarsFound, "ARM_CLIENT_SECRET")
	}

	// Check for managed identity environment variables
	if envVal := os.Getenv("AZURE_MANAGED_IDENTITY_CLIENT_ID"); envVal != "" {
		envVarsFound = append(envVarsFound, "AZURE_MANAGED_IDENTITY_CLIENT_ID")
	}

	// Log what environment variables we found
	if len(envVarsFound) > 0 {
		l.Infof("Found Azure environment variables: %v", envVarsFound)
	} else {
		l.Info("No Azure environment variables found, attempting to use default authentication")
	}

	// Create credentials using DefaultAzureCredential, which will try multiple authentication methods
	options := &azidentity.DefaultAzureCredentialOptions{}

	// Create the credential
	cred, err := azidentity.NewDefaultAzureCredential(options)
	if err != nil {
		return nil, subscriptionID, errors.Errorf("failed to obtain Azure credentials: %w", err)
	}

	// If we don't have a subscription ID, we'll need the caller to provide one
	if subscriptionID == "" {
		l.Debug("No subscription ID found in environment variables (checked AZURE_SUBSCRIPTION_ID and ARM_SUBSCRIPTION_ID)")
	} else {
		l.Debug("Found subscription ID in environment variables: " + subscriptionID)
	}

	return cred, subscriptionID, nil
}

// GetStorageAccountSKU returns the SKU name for a storage account based on account tier and replication type
// If either parameter is empty, it uses sensible defaults (Standard tier, LRS replication)
func GetStorageAccountSKU(accountTier, replicationType string) (string, bool) {
	isDefault := false

	if accountTier == "" && replicationType == "" {
		isDefault = true
		return "Standard_LRS", isDefault
	}

	// Default to Standard tier if not specified
	if accountTier == "" {
		accountTier = "Standard"
	}

	// Default to LRS replication if not specified
	if replicationType == "" {
		replicationType = "LRS"
	}

	return accountTier + "_" + replicationType, isDefault
}

// Validate checks if all required fields are set
func (cfg StorageAccountConfig) Validate() error {
	if cfg.SubscriptionID == "" {
		return errors.Errorf("subscription_id is required")
	}

	if cfg.ResourceGroupName == "" {
		return errors.Errorf("resource_group_name is required")
	}

	if cfg.StorageAccountName == "" {
		return errors.Errorf("storage_account_name is required")
	}

	if cfg.Location == "" {
		return errors.Errorf("location is required")
	}

	return nil
}

// Helper functions for property comparison and conversion

// CompareStringMaps compares existing tags (map[string]*string) with desired tags (map[string]string)
func CompareStringMaps(existing map[string]*string, desired map[string]string) bool {
	if len(existing) != len(desired) {
		return false
	}

	for k, v := range desired {
		if existingVal, ok := existing[k]; !ok || existingVal == nil || *existingVal != v {
			return false
		}
	}

	return true
}

// ConvertToPointerMap converts map[string]string to map[string]*string for Azure SDK compatibility
func ConvertToPointerMap(input map[string]string) map[string]*string {
	result := make(map[string]*string, len(input))

	for k, v := range input {
		val := v // Create new variable to avoid capturing loop variable
		result[k] = &val
	}

	return result
}

// CompareAccessTier compares Azure access tier values
func CompareAccessTier(current *armstorage.AccessTier, desired string) bool {
	if current == nil && desired == "" {
		return true
	}

	if current == nil || desired == "" {
		return false
	}

	return string(*current) == desired
}
