// Package azurehelper provides Azure-specific helper functions
package azurehelper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/gruntwork-io/terragrunt/internal/azure/azureauth"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
)

// StorageAccountClient wraps Azure's armstorage client to provide a simpler interface
type StorageAccountClient struct {
	// Management plane clients
	client               *armstorage.AccountsClient
	blobClient           *armstorage.BlobServicesClient
	roleAssignmentClient *armauthorization.RoleAssignmentsClient

	// Data plane client for blob operations
	dataPlaneBlobClient *azblob.Client

	// Configuration
	subscriptionID     string
	resourceGroupName  string
	storageAccountName string
	location           string
	config             map[string]interface{}
	credential         azcore.TokenCredential // Store credential for lazy data plane client creation

	defaultAccountKind     string
	defaultAccountTier     string
	defaultAccountSKU      string
	defaultReplicationType string
}

const (
	defaultSASExpiryHours = 24
	// tagKeyCreatedBy is the tag key used to identify resources created by terragrunt.
	tagKeyCreatedBy = "created-by"
)

// StorageAccountConfig represents the configuration for an Azure Storage Account.
//
// This struct contains all the necessary parameters to create, update, or configure
// an Azure Storage Account. It supports various storage tiers, replication types,
// encryption options, and access controls.
//
// Required Fields:
//   - SubscriptionID: Must be a valid Azure subscription UUID
//   - ResourceGroupName: Must be 1-90 characters, alphanumeric, periods, underscores, hyphens, and parentheses
//   - StorageAccountName: Must be 3-24 characters, lowercase letters and numbers only, globally unique
//   - Location: Must be a valid Azure region (e.g., "eastus", "westeurope")
//
// Field Dependencies:
//   - AccountTier and ReplicationType are combined to form AccountSKU
//   - Premium tier only supports LRS and ZRS replication
//   - Cool and Archive access tiers are only available for StorageV2 accounts
//   - KeyEncryptionKey requires appropriate Azure Key Vault permissions
//
// Default Values (applied when fields are empty):
//   - AccountKind: "StorageV2" (General Purpose v2)
//   - AccountTier: "Standard" (Standard performance tier)
//   - AccessTier: "Hot" (Hot access tier for frequent access)
//   - ReplicationType: "LRS" (Locally Redundant Storage)
//   - EnableVersioning: true (Blob versioning enabled)
//   - AllowBlobPublicAccess: false (Public access disabled for security)
//   - Tags: {"created-by": "terragrunt"}
//
// Examples:
//
//	// Minimal configuration with defaults
//	config := StorageAccountConfig{
//	    SubscriptionID:     "12345678-1234-1234-1234-123456789abc",
//	    ResourceGroupName:  "my-rg",
//	    StorageAccountName: "mystorageaccount",
//	    Location:           "eastus",
//	}
//
//	// Production configuration with hot tier
//	config := StorageAccountConfig{
//	    SubscriptionID:     "12345678-1234-1234-1234-123456789abc",
//	    ResourceGroupName:  "prod-rg",
//	    StorageAccountName: "prodstorageaccount",
//	    Location:           "eastus",
//	    AccountKind:        "StorageV2",
//	    AccountTier:        "Standard",
//	    AccessTier:         "Hot",
//	    ReplicationType:    "GRS",
//	    EnableVersioning:   true,
//	    Tags: map[string]string{
//	        "environment": "production",
//	        "team":        "platform",
//	        "created-by":  "terragrunt",
//	    },
//	}
//
//	// Archive storage for long-term backup
//	config := StorageAccountConfig{
//	    SubscriptionID:     "12345678-1234-1234-1234-123456789abc",
//	    ResourceGroupName:  "backup-rg",
//	    StorageAccountName: "archivestorage",
//	    Location:           "westus2",
//	    AccessTier:         "Cool", // Use Cool for infrequent access
//	    ReplicationType:    "GRS",  // Geo-redundant for durability
//	    EnableVersioning:   false,  // Disable versioning for archive
//	}
type StorageAccountConfig struct {
	// Tags represents custom metadata tags applied to the storage account.
	// Keys and values must each be 512 characters or less.
	// Cannot exceed 50 tags per storage account.
	// Default: {"created-by": "terragrunt"}
	Tags map[string]string

	// AccessTier specifies the access tier for blob storage data.
	// Valid values: "Hot", "Cool"
	// - Hot: Optimized for frequent access, higher storage cost, lower access cost
	// - Cool: Optimized for infrequent access, lower storage cost, higher access cost
	// - Archive tier is set per-blob, not per-account
	// Only applies to StorageV2 and BlobStorage account kinds.
	// Default: "Hot"
	AccessTier string

	// AccountKind specifies the type of storage account.
	// Valid values:
	// - "StorageV2": General Purpose v2 (recommended, supports all features)
	// - "Storage": General Purpose v1 (legacy, limited features)
	// - "BlobStorage": Blob-only storage (legacy, use StorageV2 instead)
	// - "FileStorage": Premium file shares only
	// - "BlockBlobStorage": Premium block blobs and append blobs only
	// Default: "StorageV2"
	AccountKind string

	// AccountSKU specifies the SKU name for the storage account.
	// This is automatically computed from AccountTier and ReplicationType.
	// Format: "{AccountTier}_{ReplicationType}" (e.g., "Standard_LRS", "Premium_ZRS")
	// Valid combinations:
	// - Standard: LRS, GRS, RAGRS, ZRS, GZRS, RAGZRS
	// - Premium: LRS, ZRS (limited regions)
	// Default: "Standard_LRS"
	AccountSKU string

	// AccountTier specifies the performance tier of the storage account.
	// Valid values:
	// - "Standard": Lower cost, higher latency, supports all replication types
	// - "Premium": Higher cost, lower latency, SSD-based, limited replication (LRS/ZRS only)
	// Premium tier has region limitations and higher costs.
	// Default: "Standard"
	AccountTier string

	// KeyEncryptionKey specifies the source of the encryption key for the storage account.
	// Valid values:
	// - "" (empty): Microsoft-managed keys (default)
	// - "Microsoft.KeyVault": Customer-managed keys in Azure Key Vault
	// When using Key Vault, ensure the storage account has proper access permissions.
	// Optional: Default is Microsoft-managed encryption
	KeyEncryptionKey string

	// Location specifies the Azure region where the storage account will be created.
	// Must be a valid Azure region name (e.g., "eastus", "westeurope", "southeastasia").
	// This cannot be changed after creation.
	// Consider data residency, compliance, and latency requirements.
	// Required field.
	Location string

	// ReplicationType specifies the replication strategy for data durability.
	// Valid values:
	// - "LRS": Locally Redundant Storage (3 copies in single datacenter)
	// - "GRS": Geo-Redundant Storage (LRS + async copy to paired region)
	// - "RAGRS": Read-Access Geo-Redundant Storage (GRS + read access to secondary)
	// - "ZRS": Zone-Redundant Storage (3 copies across availability zones)
	// - "GZRS": Geo-Zone-Redundant Storage (ZRS + async copy to paired region)
	// - "RAGZRS": Read-Access Geo-Zone-Redundant Storage (GZRS + read access to secondary)
	// Premium tier only supports LRS and ZRS.
	// ZRS, GZRS, RAGZRS have limited region availability.
	// Default: "LRS"
	ReplicationType string

	// ResourceGroupName specifies the name of the Azure resource group containing the storage account.
	// Must be 1-90 characters long.
	// Can contain alphanumeric characters, periods, underscores, hyphens, and parentheses.
	// Cannot end with period.
	// Required field.
	ResourceGroupName string

	// StorageAccountName specifies the name of the Azure storage account.
	// Must be 3-24 characters long.
	// Must contain only lowercase letters and numbers.
	// Must be globally unique across all Azure storage accounts.
	// Cannot be changed after creation.
	// Required field.
	StorageAccountName string

	// SubscriptionID specifies the Azure subscription ID where the storage account exists or will be created.
	// Must be a valid UUID format (e.g., "12345678-1234-1234-1234-123456789abc").
	// Required field.
	SubscriptionID string

	// AllowBlobPublicAccess controls whether public access to blobs is allowed.
	// When false (recommended), all blob access requires authentication.
	// When true, individual containers can be configured for public access.
	// For security reasons, public access should be disabled unless specifically required.
	// Default: false (public access disabled)
	AllowBlobPublicAccess bool

	// EnableVersioning controls whether blob versioning is enabled for the storage account.
	// When enabled, Azure automatically creates a version when a blob is modified.
	// Provides protection against accidental deletion or modification.
	// Has cost implications as old versions consume storage space.
	// Can be combined with lifecycle management policies for cost optimization.
	// Default: true (versioning enabled)
	EnableVersioning bool
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
		Tags:                  map[string]string{tagKeyCreatedBy: "terragrunt"},
	}
}

// storageAccountClientConfig holds the extracted configuration for creating a storage account client.
type storageAccountClientConfig struct {
	storageAccountName string
	resourceGroupName  string
	subscriptionID     string
	location           string
}

// extractStorageClientConfig extracts configuration values from the config map.
func extractStorageClientConfig(l log.Logger, config map[string]interface{}) (*storageAccountClientConfig, error) {
	storageAccountName, ok := config["storage_account_name"].(string)
	if !ok || storageAccountName == "" {
		return nil, errors.Errorf("storage_account_name is required")
	}

	resourceGroupName, ok := config["resource_group_name"].(string)
	if !ok || resourceGroupName == "" {
		l.Warn("No resource_group_name specified in config, using storage account name as resource group")

		resourceGroupName = storageAccountName + "-rg"
	}

	var subscriptionID, location string

	if v, ok := config["subscription_id"].(string); ok {
		subscriptionID = v
	}

	if v, ok := config["location"].(string); ok {
		location = v
	}

	return &storageAccountClientConfig{
		storageAccountName: storageAccountName,
		resourceGroupName:  resourceGroupName,
		subscriptionID:     subscriptionID,
		location:           location,
	}, nil
}

// CreateStorageAccountClient creates a new StorageAccount client
func CreateStorageAccountClient(ctx context.Context, l log.Logger, config map[string]interface{}) (*StorageAccountClient, error) {
	if config == nil {
		return nil, errors.Errorf("config is required")
	}

	clientCfg, err := extractStorageClientConfig(l, config)
	if err != nil {
		return nil, err
	}

	// Use centralized authentication logic
	authConfig, err := azureauth.GetAuthConfig(ctx, l, config)
	if err != nil {
		return nil, fmt.Errorf("error getting azure auth config: %w", err)
	}

	authResult, err := azureauth.GetTokenCredential(ctx, l, authConfig)
	if err != nil {
		return nil, fmt.Errorf("error getting azure credentials: %w", err)
	}

	// Reject SAS token authentication for management-plane operations
	if authResult.Method == azureauth.AuthMethodSasToken {
		return nil, errors.Errorf("sas_token authentication is only supported for data-plane (blob) operations, " +
			"not storage account management; use Azure AD or a service principal instead")
	}

	subscriptionID := resolveSubscriptionID(l, clientCfg.subscriptionID, authConfig.SubscriptionID)
	if subscriptionID == "" {
		return nil, errors.Errorf("subscription_id is required either:\n" +
			"  1. In the configuration as 'subscription_id'\n" +
			"  2. As an environment variable (AZURE_SUBSCRIPTION_ID or ARM_SUBSCRIPTION_ID)\n" +
			"Please provide at least one of these values to continue")
	}

	return createStorageAccountClientWithCred(subscriptionID, clientCfg, authResult.Credential, config)
}

// resolveSubscriptionID returns the subscription ID from config or auth config.
func resolveSubscriptionID(l log.Logger, configSubscriptionID, authSubscriptionID string) string {
	if configSubscriptionID != "" {
		return configSubscriptionID
	}

	if authSubscriptionID != "" {
		logInfo(l, "Using subscription ID from auth config: %s", authSubscriptionID)

		return authSubscriptionID
	}

	return ""
}

// createStorageAccountClientWithCred creates the storage account client with the given credential.
func createStorageAccountClientWithCred(subscriptionID string, cfg *storageAccountClientConfig, cred azcore.TokenCredential, config map[string]interface{}) (*StorageAccountClient, error) {
	accountsClient, err := armstorage.NewAccountsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, errors.Errorf("error creating storage accounts client: %w", err)
	}

	blobClient, err := armstorage.NewBlobServicesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, errors.Errorf("error creating blob services client: %w", err)
	}

	clientOptions := &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			APIVersion: defaultRoleAssignmentAPIVersion,
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
		credential:             cred, // Store credential for lazy data plane client creation
		subscriptionID:         subscriptionID,
		resourceGroupName:      cfg.resourceGroupName,
		storageAccountName:     cfg.storageAccountName,
		location:               cfg.location,
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
	logInfo(l, "Enabling versioning on storage account %s", c.storageAccountName)

	if err := c.listAndUpdateVersioning(ctx, true); err != nil {
		return err
	}

	logInfo(l, "Successfully enabled versioning on storage account %s", c.storageAccountName)

	return nil
}

func (c *StorageAccountClient) DisableStorageAccountVersioning(ctx context.Context, l log.Logger) error {
	logInfo(l, "Disabling versioning on storage account %s", c.storageAccountName)

	if err := c.listAndUpdateVersioning(ctx, false); err != nil {
		return err
	}

	logInfo(l, "Successfully disabled versioning on storage account %s", c.storageAccountName)

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
			logWarn(l, "No location specified, using default location: %s", location)
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

// mapReplicationType maps a replication type string to the ARM storage SKU.
// If accountTier is "Premium", it maps to Premium SKUs (only LRS and ZRS are supported).
// Otherwise, it maps to Standard SKUs.
func mapReplicationType(accountTier, replicationType string, l log.Logger) armstorage.SKUName {
	if replicationType == "" {
		replicationType = "LRS"
	}

	// Premium tier only supports LRS and ZRS
	if accountTier == "Premium" {
		premiumSkuMapping := map[string]armstorage.SKUName{
			"LRS": armstorage.SKUNamePremiumLRS,
			"ZRS": armstorage.SKUNamePremiumZRS,
		}

		if sku, ok := premiumSkuMapping[replicationType]; ok {
			return sku
		}

		logWarn(l, "Premium tier only supports LRS and ZRS replication. Requested %s, using Premium_LRS", replicationType)

		return armstorage.SKUNamePremiumLRS
	}

	// Standard tier (default)
	skuMapping := map[string]armstorage.SKUName{
		"LRS":    armstorage.SKUNameStandardLRS,
		"GRS":    armstorage.SKUNameStandardGRS,
		"RAGRS":  armstorage.SKUNameStandardRAGRS,
		"ZRS":    armstorage.SKUNameStandardZRS,
		"GZRS":   armstorage.SKUNameStandardGZRS,
		"RAGZRS": armstorage.SKUNameStandardRAGZRS,
	}

	if sku, ok := skuMapping[replicationType]; ok {
		return sku
	}

	logWarn(l, "Unsupported replication type %s, using Standard_LRS", replicationType)

	return armstorage.SKUNameStandardLRS
}

// mapAccountKind maps an account kind string to the ARM storage kind.
func mapAccountKind(accountKind string, l log.Logger) armstorage.Kind {
	if accountKind == "" {
		return armstorage.KindStorageV2
	}

	kindMapping := map[string]armstorage.Kind{
		"StorageV2":        armstorage.KindStorageV2,
		"Storage":          armstorage.KindStorage,
		"BlobStorage":      armstorage.KindBlobStorage,
		"BlockBlobStorage": armstorage.KindBlockBlobStorage,
		"FileStorage":      armstorage.KindFileStorage,
	}

	if kind, ok := kindMapping[accountKind]; ok {
		return kind
	}

	logWarn(l, "Unsupported account kind %s, using StorageV2", accountKind)

	return armstorage.KindStorageV2
}

// validateAccessTier validates and returns a valid access tier string.
func validateAccessTier(accessTier string, l log.Logger) string {
	if accessTier == "" {
		return AccessTierHot
	}

	validTiers := map[string]bool{
		AccessTierHot:     true,
		AccessTierCool:    true,
		AccessTierPremium: true,
	}

	if validTiers[accessTier] {
		return accessTier
	}

	logWarn(l, "Unsupported access tier %s, using Hot", accessTier)

	return AccessTierHot
}

// buildStorageAccountTags builds the tags map for storage account creation.
func buildStorageAccountTags(configTags map[string]string) map[string]*string {
	tags := make(map[string]*string, len(configTags))

	if len(configTags) > 0 {
		for k, v := range configTags {
			value := v
			tags[k] = &value
		}
	} else {
		defaultTag := "terragrunt"
		tags[tagKeyCreatedBy] = &defaultTag
	}

	return tags
}

// resolveLocation returns the location to use for storage account creation.
func (c *StorageAccountClient) resolveLocation(configLocation string, l log.Logger) string {
	if configLocation != "" {
		return configLocation
	}

	if c.location != "" {
		return c.location
	}

	logWarn(l, "No location specified, using default location: %s", defaultLocation)

	return defaultLocation
}

// buildStorageAccountParameters builds the parameters for storage account creation.
func (c *StorageAccountClient) buildStorageAccountParameters(config StorageAccountConfig, l log.Logger) armstorage.AccountCreateParameters {
	sku := mapReplicationType(config.AccountTier, config.ReplicationType, l)
	kind := mapAccountKind(config.AccountKind, l)
	accessTierStr := validateAccessTier(config.AccessTier, l)
	tags := buildStorageAccountTags(config.Tags)
	location := c.resolveLocation(config.Location, l)

	logInfo(l, "Using access tier: %s", accessTierStr)

	parameters := armstorage.AccountCreateParameters{
		SKU:      &armstorage.SKU{Name: &sku},
		Kind:     &kind,
		Location: to.Ptr(location),
		Tags:     tags,
		Properties: &armstorage.AccountPropertiesCreateParameters{
			EnableHTTPSTrafficOnly: to.Ptr(true),
			MinimumTLSVersion:      to.Ptr(armstorage.MinimumTLSVersionTLS12),
			AllowBlobPublicAccess:  to.Ptr(config.AllowBlobPublicAccess),
			AccessTier:             convertAccessTierToARM(accessTierStr),
		},
	}

	return parameters
}

// createStorageAccount creates a new storage account
func (c *StorageAccountClient) createStorageAccount(ctx context.Context, l log.Logger, config StorageAccountConfig) error {
	logInfo(l, "Creating Azure Storage account %s in resource group %s", c.storageAccountName, c.resourceGroupName)

	parameters := c.buildStorageAccountParameters(config, l)

	logInfo(l, "Creating storage account %s in %s (Kind: %s, SKU: %s)",
		c.storageAccountName, *parameters.Location, *parameters.Kind, *parameters.SKU.Name)

	pollerResp, err := c.client.BeginCreate(ctx, c.resourceGroupName, c.storageAccountName, parameters, nil)
	if err != nil {
		return errors.Errorf("error creating storage account: %w", err)
	}

	_, err = pollerResp.PollUntilDone(ctx, nil)
	if err != nil {
		return errors.Errorf("error waiting for storage account creation: %w", err)
	}

	logInfo(l, "Successfully created storage account %s", c.storageAccountName)

	if err := c.AssignStorageBlobDataOwnerRole(ctx, l); err != nil {
		logWarn(l, "Failed to assign Storage Blob Data Owner role: %v", err)
	}

	if config.EnableVersioning {
		if err := c.EnableStorageAccountVersioning(ctx, l); err != nil {
			return err
		}
	}

	return nil
}

// checkBlobPublicAccessUpdate checks and prepares AllowBlobPublicAccess update if needed.
func (c *StorageAccountClient) checkBlobPublicAccessUpdate(l log.Logger, config StorageAccountConfig, account *armstorage.Account, updateParams *armstorage.AccountUpdateParameters) bool {
	if account == nil || account.Properties == nil || account.Properties.AllowBlobPublicAccess == nil {
		return false
	}

	if *account.Properties.AllowBlobPublicAccess == config.AllowBlobPublicAccess {
		return false
	}

	updateParams.Properties.AllowBlobPublicAccess = to.Ptr(config.AllowBlobPublicAccess)
	logInfo(l, "Updating AllowBlobPublicAccess from %t to %t on storage account %s",
		*account.Properties.AllowBlobPublicAccess, config.AllowBlobPublicAccess, c.storageAccountName)

	return true
}

// checkAccessTierUpdate checks and prepares AccessTier update if needed.
func (c *StorageAccountClient) checkAccessTierUpdate(l log.Logger, config StorageAccountConfig, account *armstorage.Account, updateParams *armstorage.AccountUpdateParameters) bool {
	if account == nil || account.Properties == nil ||
		config.AccessTier == "" || CompareAccessTier(account.Properties.AccessTier, config.AccessTier) {
		return false
	}

	accessTier := convertAccessTierToARM(config.AccessTier)
	if accessTier == nil {
		logWarn(l, "Unsupported access tier %s, skipping update", config.AccessTier)

		return false
	}

	updateParams.Properties.AccessTier = accessTier
	currentTier := "Unknown"

	if account.Properties.AccessTier != nil {
		currentTier = string(*account.Properties.AccessTier)
	}

	logInfo(l, "Updating AccessTier from %s to %s on storage account %s", currentTier, config.AccessTier, c.storageAccountName)

	return true
}

// convertAccessTierToARM converts a string access tier to the ARM storage type.
func convertAccessTierToARM(tier string) *armstorage.AccessTier {
	switch tier {
	case AccessTierHot:
		return to.Ptr(armstorage.AccessTierHot)
	case AccessTierCool:
		return to.Ptr(armstorage.AccessTierCool)
	case AccessTierPremium:
		return to.Ptr(armstorage.AccessTierPremium)
	default:
		return nil
	}
}

// checkTagsUpdate checks and prepares Tags update if needed.
func (c *StorageAccountClient) checkTagsUpdate(l log.Logger, config StorageAccountConfig, account *armstorage.Account, updateParams *armstorage.AccountUpdateParameters) bool {
	if len(config.Tags) == 0 || CompareStringMaps(account.Tags, config.Tags) {
		return false
	}

	updateParams.Tags = ConvertToPointerMap(config.Tags)

	logInfo(l, "Updating tags on storage account %s", c.storageAccountName)

	return true
}

// warnReadOnlyPropertyDiffs logs warnings for immutable properties that differ from desired configuration.
func (c *StorageAccountClient) warnReadOnlyPropertyDiffs(l log.Logger, config StorageAccountConfig, account *armstorage.Account) {
	c.warnSKUDiff(l, config, account)
	c.warnAccountKindDiff(l, config, account)
	c.warnLocationDiff(l, config, account)
}

// warnSKUDiff logs a warning if SKU/ReplicationType differs from desired configuration.
func (c *StorageAccountClient) warnSKUDiff(l log.Logger, config StorageAccountConfig, account *armstorage.Account) {
	if account.SKU == nil || config.ReplicationType == "" {
		return
	}

	currentSKU := string(*account.SKU.Name)
	expectedSKU, valid := GetStorageAccountSKU(config.AccountTier, config.ReplicationType)

	if !valid {
		logWarn(l, "Could not determine expected SKU for tier %s and replication %s", config.AccountTier, config.ReplicationType)

		return
	}

	if currentSKU != expectedSKU {
		logWarn(l, "Storage account SKU cannot be changed after creation. Current: %s, Desired: %s",
			currentSKU, expectedSKU)
	}
}

// warnAccountKindDiff logs a warning if AccountKind differs from desired configuration.
func (c *StorageAccountClient) warnAccountKindDiff(l log.Logger, config StorageAccountConfig, account *armstorage.Account) {
	if account.Kind == nil || config.AccountKind == "" {
		return
	}

	currentKind := string(*account.Kind)
	if currentKind != config.AccountKind {
		logWarn(l, "Storage account kind cannot be changed after creation. Current: %s, Desired: %s",
			currentKind, config.AccountKind)
	}
}

// warnLocationDiff logs a warning if Location differs from desired configuration.
func (c *StorageAccountClient) warnLocationDiff(l log.Logger, config StorageAccountConfig, account *armstorage.Account) {
	if account.Location == nil || config.Location == "" {
		return
	}

	if *account.Location != config.Location {
		logWarn(l, "Storage account location cannot be changed after creation. Current: %s, Desired: %s",
			*account.Location, config.Location)
	}
}

// syncVersioningState ensures the versioning state matches the desired configuration.
func (c *StorageAccountClient) syncVersioningState(ctx context.Context, l log.Logger, enableVersioning bool) error {
	isVersioningEnabled, err := c.GetStorageAccountVersioning(ctx)
	if err != nil {
		return err
	}

	if enableVersioning && !isVersioningEnabled {
		logInfo(l, "Enabling versioning on existing storage account %s", c.storageAccountName)

		return c.EnableStorageAccountVersioning(ctx, l)
	}

	if !enableVersioning && isVersioningEnabled {
		logInfo(l, "Disabling versioning on existing storage account %s", c.storageAccountName)

		return c.DisableStorageAccountVersioning(ctx, l)
	}

	return nil
}

// updateStorageAccountIfNeeded updates a storage account if settings don't match
func (c *StorageAccountClient) updateStorageAccountIfNeeded(ctx context.Context, l log.Logger, config StorageAccountConfig, account *armstorage.Account) error {
	var updateParams armstorage.AccountUpdateParameters

	updateParams.Properties = &armstorage.AccountPropertiesUpdateParameters{}

	// Check updatable properties
	needsUpdate := c.checkBlobPublicAccessUpdate(l, config, account, &updateParams) ||
		c.checkAccessTierUpdate(l, config, account, &updateParams) ||
		c.checkTagsUpdate(l, config, account, &updateParams)

	// Warn about read-only properties that differ
	c.warnReadOnlyPropertyDiffs(l, config, account)

	// Apply updates if needed
	if needsUpdate {
		logInfo(l, "Updating storage account %s with new properties", c.storageAccountName)

		_, err := c.client.Update(ctx, c.resourceGroupName, c.storageAccountName, updateParams, nil)
		if err != nil {
			return errors.Errorf("error updating storage account properties: %w", err)
		}

		logInfo(l, "Successfully updated storage account %s", c.storageAccountName)
	} else {
		logInfo(l, "Storage account %s properties are already up to date", c.storageAccountName)
	}

	// Handle versioning separately (as it's a blob service property, not account property)
	return c.syncVersioningState(ctx, l, config.EnableVersioning)
}

// DeleteStorageAccount deletes a storage account
func (c *StorageAccountClient) DeleteStorageAccount(ctx context.Context, l log.Logger) error {
	logInfo(l, "Deleting storage account %s in resource group %s", c.storageAccountName, c.resourceGroupName)

	// First check if the storage account exists
	if _, err := c.client.GetProperties(ctx, c.resourceGroupName, c.storageAccountName, nil); err != nil {
		// If 404, it's already deleted
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == httpStatusNotFound {
			logInfo(l, "Storage account %s does not exist or is already deleted", c.storageAccountName)
			return nil
		}

		return errors.Errorf("error checking storage account: %w", err)
	}

	// Delete the storage account
	if _, err := c.client.Delete(ctx, c.resourceGroupName, c.storageAccountName, nil); err != nil {
		return errors.Errorf("error deleting storage account: %w", err)
	}

	logInfo(l, "Successfully deleted storage account %s", c.storageAccountName)

	return nil
}

// EnsureResourceGroup creates a resource group if it doesn't exist
func (c *StorageAccountClient) EnsureResourceGroup(ctx context.Context, l log.Logger, location string) error {
	logInfo(l, "Ensuring resource group %s exists in %s", c.resourceGroupName, location)

	resourceGroupClient, err := CreateResourceGroupClient(ctx, l, c.subscriptionID)
	if err != nil {
		return fmt.Errorf("error creating resource group client: %w", err)
	}

	// Default tags to use if not specified
	tags := map[string]string{
		tagKeyCreatedBy: "terragrunt",
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
		bodyBytes, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error message, failure is acceptable
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
	userObjectID, err := c.getCurrentUserObjectID(ctx)
	if err != nil {
		c.handleMissingUserObjectID(l, err)
		return nil
	}

	isServicePrincipal := c.isServicePrincipalAndLog(l, userObjectID)

	storageAccountResourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s",
		c.subscriptionID, c.resourceGroupName, c.storageAccountName)
	roleDefinitionID := fmt.Sprintf("/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/%s",
		c.subscriptionID, storageBlobDataOwnerRoleID)
	roleAssignmentID := util.GenerateUUID()

	c.logPrincipalAssignment(l, isServicePrincipal, userObjectID)

	roleAssignment := c.createRoleAssignmentParams(roleDefinitionID, userObjectID)

	c.logRoleAssignmentDebug(l, roleAssignmentID, roleDefinitionID, storageAccountResourceID)

	err = c.createRoleAssignmentWithRetry(ctx, l, storageAccountResourceID, roleAssignmentID, roleAssignment, userObjectID, isServicePrincipal)
	if err != nil {
		return errors.Errorf("error creating role assignment: %w", err)
	}

	c.logRoleAssignmentSuccess(l, isServicePrincipal, userObjectID)

	// Wait for RBAC permissions to propagate with retry logic
	err = c.waitForRBACPermissions(ctx, l)
	if err != nil {
		logWarn(l, "RBAC permissions may not have fully propagated: %v", err)
		l.Info("If you encounter permission errors, please wait a few minutes and try again")
	}

	return nil
}

// handleMissingUserObjectID logs and handles the case where the user object ID could not be retrieved.
func (c *StorageAccountClient) handleMissingUserObjectID(l log.Logger, err error) {
	logWarn(l, "Could not get current user object ID: %v. Skipping role assignment.", err)
	l.Info("To assign Storage Blob Data Owner role manually, use: az role assignment create --role 'Storage Blob Data Owner' --assignee <your-user-id> --scope /subscriptions/<sub-id>/resourceGroups/<rg>/providers/Microsoft.Storage/storageAccounts/<sa-name>")
}

// isServicePrincipalAndLog determines if the principal is a service principal and logs accordingly.
func (c *StorageAccountClient) isServicePrincipalAndLog(l log.Logger, userObjectID string) bool {
	isServicePrincipal := false
	if os.Getenv("AZURE_CLIENT_ID") != "" || os.Getenv("ARM_CLIENT_ID") != "" {
		isServicePrincipal = true

		logInfo(l, "Detected service principal authentication. Assigning role to service principal with object ID: %s", userObjectID)
	} else {
		logInfo(l, "Assigning Storage Blob Data Owner role to user with object ID: %s", userObjectID)
	}

	return isServicePrincipal
}

// logPrincipalAssignment logs the assignment action based on principal type.
func (c *StorageAccountClient) logPrincipalAssignment(l log.Logger, isServicePrincipal bool, userObjectID string) {
	if isServicePrincipal {
		logInfo(l, "Assigning Storage Blob Data Owner role to service principal %s for storage account %s", userObjectID, c.storageAccountName)
	} else {
		logInfo(l, "Assigning Storage Blob Data Owner role to user %s for storage account %s", userObjectID, c.storageAccountName)
	}
}

// createRoleAssignmentParams creates the parameters for the role assignment.
func (c *StorageAccountClient) createRoleAssignmentParams(roleDefinitionID, userObjectID string) armauthorization.RoleAssignmentCreateParameters {
	return armauthorization.RoleAssignmentCreateParameters{
		Properties: &armauthorization.RoleAssignmentProperties{
			RoleDefinitionID: to.Ptr(roleDefinitionID),
			PrincipalID:      to.Ptr(userObjectID),
		},
	}
}

// logRoleAssignmentDebug logs debug information for the role assignment.
func (c *StorageAccountClient) logRoleAssignmentDebug(l log.Logger, roleAssignmentID, roleDefinitionID, storageAccountResourceID string) {
	logDebug(l, "Creating role assignment with ID: %s", roleAssignmentID)
	logDebug(l, "Role definition ID: %s", roleDefinitionID)
	logDebug(l, "Storage account resource ID: %s", storageAccountResourceID)
}

// createRoleAssignmentWithRetry handles the creation of the role assignment with retry logic for known errors.
func (c *StorageAccountClient) createRoleAssignmentWithRetry(
	ctx context.Context,
	l log.Logger,
	storageAccountResourceID, roleAssignmentID string,
	roleAssignment armauthorization.RoleAssignmentCreateParameters,
	userObjectID string,
	isServicePrincipal bool,
) error {
	_, err := c.roleAssignmentClient.Create(ctx, storageAccountResourceID, roleAssignmentID, roleAssignment, nil)
	if err == nil {
		return nil
	}

	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) && respErr.StatusCode == httpStatusConflict {
		if isServicePrincipal {
			logInfo(l, "Storage Blob Data Owner role already assigned to service principal %s", userObjectID)
		} else {
			logInfo(l, "Storage Blob Data Owner role already assigned to user %s", userObjectID)
		}

		return nil
	}

	if errors.As(err, &respErr) && (respErr.StatusCode == httpStatusForbidden || respErr.StatusCode == httpStatusUnauthorized) {
		logWarn(l, "Permission denied when assigning Storage Blob Data Owner role. Principal %s doesn't have sufficient permissions.", userObjectID)
		l.Info("To assign Storage Blob Data Owner role manually, use: az role assignment create --role 'Storage Blob Data Owner' --assignee <principal-id> --scope /subscriptions/<sub-id>/resourceGroups/<rg>/providers/Microsoft.Storage/storageAccounts/<sa-name>")

		return nil
	}

	if errors.As(err, &respErr) && respErr.ErrorCode == "InvalidRoleAssignmentId" {
		logWarn(l, "Invalid role assignment ID format. Status: %d, Error code: %s", respErr.StatusCode, respErr.ErrorCode)
		logDebug(l, "Full error: %+v", respErr)
		// Try with a different format for the role assignment ID
		roleAssignmentID := fmt.Sprintf("%s-%s-4000-8000-%s",
			util.GenerateUUID()[0:8],
			util.GenerateUUID()[0:4],
			util.GenerateUUID()[0:12])
		logInfo(l, "Retrying with alternative role assignment ID format: %s", roleAssignmentID)

		_, retryErr := c.roleAssignmentClient.Create(ctx, storageAccountResourceID, roleAssignmentID, roleAssignment, nil)
		if retryErr == nil {
			l.Info("Successfully created role assignment with alternative ID format")

			return nil
		}

		logWarn(l, "Retry also failed. Consider creating the role assignment manually: az role assignment create --role 'Storage Blob Data Owner' --assignee %s --scope %s",
			userObjectID, storageAccountResourceID)

		return nil
	}

	return err
}

// RBACTestResult represents the result of an RBAC permission test
type RBACTestResult struct {
	ManagementError   error
	DataPlaneError    error
	ManagementPlaneOK bool
	DataPlaneOK       bool
}

// testManagementPlanePermissions tests if management plane RBAC permissions are available
// by checking storage account properties via Azure Resource Manager API.
func (c *StorageAccountClient) testManagementPlanePermissions(ctx context.Context) error {
	_, err := c.client.GetProperties(ctx, c.resourceGroupName, c.storageAccountName, nil)
	return err
}

// getOrCreateDataPlaneBlobClient lazily creates the data plane blob client if needed
func (c *StorageAccountClient) getOrCreateDataPlaneBlobClient() (*azblob.Client, error) {
	if c.dataPlaneBlobClient != nil {
		return c.dataPlaneBlobClient, nil
	}

	if c.credential == nil {
		return nil, errors.Errorf("no credential available for data plane client creation")
	}

	// Get endpoint suffix from config or use default
	endpointSuffix := "blob.core.windows.net"
	if suffix, ok := c.config["endpoint_suffix"].(string); ok && suffix != "" {
		endpointSuffix = "blob." + suffix
	} else if cloudEnv, ok := c.config["cloud_environment"].(string); ok && cloudEnv != "" {
		suffix := azureauth.GetEndpointSuffix(cloudEnv)
		if suffix != "" {
			endpointSuffix = "blob." + suffix
		}
	}

	blobURL := fmt.Sprintf("https://%s.%s", c.storageAccountName, endpointSuffix)

	client, err := azblob.NewClient(blobURL, c.credential, nil)
	if err != nil {
		return nil, errors.Errorf("error creating data plane blob client: %w", err)
	}

	c.dataPlaneBlobClient = client

	return client, nil
}

// testDataPlanePermissions tests if data plane RBAC permissions are available
// by attempting to list containers in the storage account.
// This validates the "Storage Blob Data Owner" role assignment has propagated.
func (c *StorageAccountClient) testDataPlanePermissions(ctx context.Context) error {
	client, err := c.getOrCreateDataPlaneBlobClient()
	if err != nil {
		return err
	}

	// Try to list containers - this requires data plane permissions
	pager := client.NewListContainersPager(&azblob.ListContainersOptions{
		MaxResults: to.Ptr(int32(1)), // We only need to verify access, not list everything
	})

	// Try to get the first page
	_, err = pager.NextPage(ctx)

	return err
}

// testRBACPermissions tests both management and data plane permissions
func (c *StorageAccountClient) testRBACPermissions(ctx context.Context, _ log.Logger) *RBACTestResult {
	result := &RBACTestResult{}

	// Test management plane (Azure Resource Manager API)
	result.ManagementError = c.testManagementPlanePermissions(ctx)
	result.ManagementPlaneOK = result.ManagementError == nil

	// Test data plane (Blob Storage API)
	result.DataPlaneError = c.testDataPlanePermissions(ctx)
	result.DataPlaneOK = result.DataPlaneError == nil

	return result
}

// logRBACTestResult logs the result of an RBAC permission test
func (c *StorageAccountClient) logRBACTestResult(l log.Logger, result *RBACTestResult, isDebug bool) {
	logFn := logInfo
	if isDebug {
		logFn = logDebug
	}

	if result.ManagementPlaneOK {
		logFn(l, "  ✓ Management plane access: OK")
	} else {
		logFn(l, "  ✗ Management plane access: %v", result.ManagementError)
	}

	if result.DataPlaneOK {
		logFn(l, "  ✓ Data plane access: OK")
	} else {
		logFn(l, "  ✗ Data plane access: %v", result.DataPlaneError)
	}
}

// analyzeRBACErrors logs warnings for non-permission errors
func (c *StorageAccountClient) analyzeRBACErrors(l log.Logger, result *RBACTestResult) {
	if result.ManagementError != nil && !isPermissionError(result.ManagementError) {
		logWarn(l, "Management plane error is not permission-related: %v", result.ManagementError)
	}

	if result.DataPlaneError != nil && !isPermissionError(result.DataPlaneError) {
		// Data plane errors during RBAC propagation are often permission errors
		// even if they don't look like it (e.g., "ResourceNotFound" can occur)
		logDebug(l, "Data plane error (may still be RBAC propagation): %v", result.DataPlaneError)
	}
}

// waitForRetryDelay waits for the retry delay or context cancellation
func (c *StorageAccountClient) waitForRetryDelay(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(RbacRetryDelay):
		return nil
	}
}

// waitForRBACPermissions waits for RBAC permissions to propagate by testing both
// management plane and data plane access. This ensures the "Storage Blob Data Owner"
// role has fully propagated before returning.
func (c *StorageAccountClient) waitForRBACPermissions(ctx context.Context, l log.Logger) error {
	maxDuration := RbacPropagationTimeout
	startTime := time.Now()

	logInfo(l, "Waiting for RBAC permissions to propagate (up to %v)", maxDuration)
	logInfo(l, "Testing both management plane (ARM API) and data plane (Blob Storage API) access...")

	for attempt := 1; ; attempt++ {
		elapsed := time.Since(startTime)

		if elapsed >= maxDuration {
			return fmt.Errorf("RBAC permissions did not propagate within %v", maxDuration)
		}

		logDebug(l, "RBAC permission test attempt %d (elapsed: %v)", attempt, elapsed.Round(time.Second))

		result := c.testRBACPermissions(ctx, l)
		c.logRBACTestResult(l, result, true) // Log as debug during retries

		// Both must succeed
		if result.ManagementPlaneOK && result.DataPlaneOK {
			logInfo(l, "RBAC permissions verified successfully after %v (attempt %d)", elapsed.Round(time.Second), attempt)
			c.logRBACTestResult(l, result, false) // Log as info on success

			return nil
		}

		c.analyzeRBACErrors(l, result)

		if err := c.waitForRetryDelay(ctx); err != nil {
			return err
		}
	}
}

// isPermissionError checks if an error is related to insufficient permissions
func isPermissionError(err error) bool {
	if err == nil {
		return false
	}

	// Check for Azure ResponseError first
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		// Check status codes
		if respErr.StatusCode == httpStatusUnauthorized || respErr.StatusCode == httpStatusForbidden {
			return true
		}

		// Check specific Azure error codes that indicate permission issues
		permissionErrorCodes := []string{
			"AuthorizationFailed",
			"Forbidden",
			"Unauthorized",
			"InsufficientAccountPermissions",
			"AccountIsAccessDenied",
			"InsufficientPermissions",
			"AccessDenied",
			"PermissionDenied",
		}

		for _, code := range permissionErrorCodes {
			if respErr.ErrorCode == code {
				return true
			}
		}
	}

	// Fallback to string-based detection for other error types
	errStr := strings.ToLower(err.Error())

	// Check for common permission-related error messages
	permissionKeywords := []string{
		"forbidden",
		"unauthorized",
		"access denied",
		"insufficient permissions",
		"permission denied",
		"not authorized",
		"authorization failed",
		"role assignment",
		"storage blob data owner",
	}

	for _, keyword := range permissionKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}

	return false
}

// logRoleAssignmentSuccess logs a success message after role assignment.
func (c *StorageAccountClient) logRoleAssignmentSuccess(l log.Logger, isServicePrincipal bool, userObjectID string) {
	if isServicePrincipal {
		logInfo(l, "Successfully assigned Storage Blob Data Owner role to service principal %s", userObjectID)
	} else {
		logInfo(l, "Successfully assigned Storage Blob Data Owner role to user %s", userObjectID)
	}
}

// GetAzureCredentials checks for Azure environment variables and returns appropriate credentials.
// If no environment variables are set, it attempts to use default authentication methods.
// This function is now implemented using the centralized authentication package.
// Note: This function exists for backward compatibility.
func GetAzureCredentials(ctx context.Context, l log.Logger) (*azidentity.DefaultAzureCredential, string, error) {
	// Create an empty config and let the azureauth package handle finding credentials
	config := make(map[string]interface{})

	// Use centralized authentication logic to determine auth method
	authConfig, err := azureauth.GetAuthConfig(ctx, l, config)
	if err != nil {
		return nil, "", fmt.Errorf("error getting azure auth config: %w", err)
	}

	// For backward compatibility, always create a DefaultAzureCredential
	// This ensures existing code that depends on this type still works
	defaultCred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("error creating azure default credential: %w", err)
	}

	// Log the authentication method being used
	logInfo(l, "Using authentication method: %s", authConfig.Method)

	return defaultCred, authConfig.SubscriptionID, nil
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

// IsPermissionError checks if an error is related to insufficient permissions (public method for testing)
func (c *StorageAccountClient) IsPermissionError(err error) bool {
	return isPermissionError(err)
}

// GetStorageAccountKeys retrieves the access keys for the storage account
func (c *StorageAccountClient) GetStorageAccountKeys(ctx context.Context) ([]string, error) {
	resp, err := c.client.ListKeys(ctx, c.resourceGroupName, c.storageAccountName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list storage account keys: %w", err)
	}

	var keys []string

	if resp.Keys != nil {
		for _, key := range resp.Keys {
			if key.Value != nil {
				keys = append(keys, *key.Value)
			}
		}
	}

	return keys, nil
}

// GetStorageAccountSAS generates a SAS token for the storage account
func (c *StorageAccountClient) GetStorageAccountSAS(ctx context.Context, permissions string, expiry *time.Time) (string, error) {
	// Set default expiry if not provided
	if expiry == nil {
		defaultExpiry := time.Now().Add(time.Duration(defaultSASExpiryHours) * time.Hour)
		expiry = &defaultExpiry
	}

	// Set default permissions if not provided
	if permissions == "" {
		permissions = "rwdlacup" // Read, Write, Delete, List, Add, Create, Update, Process
	}

	// Create SAS parameters
	sasParams := armstorage.AccountSasParameters{
		Services:               to.Ptr(armstorage.ServicesB),                                                                                // Blob service
		ResourceTypes:          to.Ptr(armstorage.SignedResourceTypesC + armstorage.SignedResourceTypesO + armstorage.SignedResourceTypesS), // Container + Object + Service
		Permissions:            to.Ptr(armstorage.Permissions(permissions)),
		SharedAccessExpiryTime: expiry,
		Protocols:              to.Ptr(armstorage.HTTPProtocolHTTPS),
	}

	resp, err := c.client.ListAccountSAS(ctx, c.resourceGroupName, c.storageAccountName, sasParams, nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate SAS token: %w", err)
	}

	if resp.AccountSasToken == nil {
		return "", errors.New("SAS token is nil in response")
	}

	return *resp.AccountSasToken, nil
}

// GetStorageAccountProperties retrieves the properties of the storage account
func (c *StorageAccountClient) GetStorageAccountProperties(ctx context.Context) (*armstorage.AccountProperties, error) {
	resp, err := c.client.GetProperties(ctx, c.resourceGroupName, c.storageAccountName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage account properties: %w", err)
	}

	return resp.Account.Properties, nil
}

// GetResourceGroupName returns the resource group name configured for this client
func (c *StorageAccountClient) GetResourceGroupName() string {
	return c.resourceGroupName
}

// GetStorageAccountName returns the storage account name configured for this client
func (c *StorageAccountClient) GetStorageAccountName() string {
	return c.storageAccountName
}
