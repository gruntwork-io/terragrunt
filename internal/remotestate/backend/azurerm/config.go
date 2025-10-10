package azurerm

import (
	"fmt"
	"slices"

	"github.com/mitchellh/mapstructure"
)

// BackendName is the name of the Azure RM backend.

// StorageAccountBootstrapConfig represents the configuration for Azure Storage account bootstrapping.
// This configuration is used when Terragrunt automatically creates or updates Azure Storage accounts
// for remote state storage. It controls various storage account properties and creation behaviors.
//
// Usage example:
//
//	config := StorageAccountBootstrapConfig{
//	    Location:                        "eastus",
//	    ResourceGroupName:               "terraform-state-rg",
//	    AccountKind:                     "StorageV2",
//	    AccountTier:                     "Standard",
//	    AccessTier:                      "Hot",
//	    ReplicationType:                 "LRS",
//	    EnableVersioning:                true,
//	    AllowBlobPublicAccess:           false,
//	    CreateStorageAccountIfNotExists: true,
//	    SkipStorageAccountUpdate:        false,
//	    StorageAccountTags: map[string]string{
//	        "Environment": "production",
//	        "Owner":       "platform-team",
//	    },
//	}
type StorageAccountBootstrapConfig struct {
	// StorageAccountTags specifies custom metadata tags to apply to the storage account.
	// Azure allows up to 50 tags per storage account.
	// Tag names and values are case-sensitive and cannot exceed 512 characters each.
	// Common tags include Environment, Owner, CostCenter, Project.
	// Default: empty map (no custom tags)
	StorageAccountTags map[string]string `mapstructure:"storage_account_tags"`

	// Location specifies the Azure region where the storage account will be created.
	// Must be a valid Azure region name (e.g., "eastus", "westeurope", "southeastasia").
	// This cannot be changed after the storage account is created.
	// Consider data residency, compliance requirements, and latency when choosing.
	// Required field when CreateStorageAccountIfNotExists is true.
	Location string `mapstructure:"location"`

	// ResourceGroupName specifies the name of the Azure resource group for the storage account.
	// Must be 1-90 characters long and can contain alphanumeric characters, periods,
	// underscores, hyphens, and parentheses. Cannot end with a period.
	// The resource group must exist before creating the storage account.
	// Required field when CreateStorageAccountIfNotExists is true.
	ResourceGroupName string `mapstructure:"resource_group_name"`

	// AccountKind specifies the type of storage account to create.
	// Valid values:
	// - "StorageV2": General Purpose v2 (recommended, supports all features)
	// - "Storage": General Purpose v1 (legacy, limited features)
	// - "BlobStorage": Blob-only storage (legacy, use StorageV2 instead)
	// - "FileStorage": Premium file shares only
	// - "BlockBlobStorage": Premium block blobs and append blobs only
	// Default: "StorageV2"
	AccountKind string `mapstructure:"account_kind"`

	// AccountTier specifies the performance tier of the storage account.
	// Valid values:
	// - "Standard": Lower cost, higher latency, supports all replication types
	// - "Premium": Higher cost, lower latency, SSD-based, limited replication (LRS/ZRS only)
	// Premium tier has region limitations and significantly higher costs.
	// Default: "Standard"
	AccountTier string `mapstructure:"account_tier"`

	// AccessTier specifies the default access tier for blob storage data.
	// Valid values:
	// - "Hot": Optimized for frequent access, higher storage cost, lower access cost
	// - "Cool": Optimized for infrequent access, lower storage cost, higher access cost
	// Archive tier is set per-blob, not per-account.
	// Only applies to StorageV2 and BlobStorage account kinds.
	// Default: "Hot"
	AccessTier string `mapstructure:"access_tier"`

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
	ReplicationType string `mapstructure:"replication_type"`

	// EnableVersioning controls whether blob versioning is enabled for the storage account.
	// When enabled, Azure automatically creates a version when a blob is modified.
	// Provides protection against accidental deletion or modification of Terraform state files.
	// Has cost implications as old versions consume storage space.
	// Should be combined with lifecycle management policies for cost optimization.
	// Default: true (versioning enabled for state file protection)
	EnableVersioning bool `mapstructure:"enable_versioning"`

	// AllowBlobPublicAccess controls whether public access to blobs is allowed.
	// When false (recommended), all blob access requires authentication.
	// When true, individual containers can be configured for public access.
	// For security reasons, public access should be disabled for state storage.
	// Default: false (public access disabled for security)
	AllowBlobPublicAccess bool `mapstructure:"allow_blob_public_access"`

	// CreateStorageAccountIfNotExists controls whether to create the storage account if it doesn't exist.
	// When true, Terragrunt will create the storage account with the specified configuration.
	// When false, Terragrunt expects the storage account to already exist.
	// Requires appropriate Azure permissions to create storage accounts.
	// Default: false (do not create storage account automatically)
	CreateStorageAccountIfNotExists bool `mapstructure:"create_storage_account_if_not_exists"`

	// SkipStorageAccountUpdate controls whether to skip updating existing storage account properties.
	// When true, Terragrunt will not modify existing storage account settings.
	// When false, Terragrunt will update the storage account to match the configuration.
	// Useful when storage account is managed by other tools or has specific configurations.
	// Default: false (update storage account to match configuration)
	SkipStorageAccountUpdate bool `mapstructure:"skip_storage_account_update"`
}

// RemoteStateConfigAzurerm represents the configuration for Azure Storage backend.
// This configuration defines how Terragrunt connects to and uses Azure Storage
// for storing Terraform remote state files. It supports multiple authentication methods
// and cloud environments.
//
// Authentication Methods:
// 1. Azure AD (default): Uses Azure Active Directory with automatic credential discovery
// 2. Service Principal: Uses explicit client ID, secret, and tenant ID
// 3. Managed Service Identity (MSI): Uses Azure MSI for authentication
// 4. SAS Token: Uses Shared Access Signature for storage-specific authentication
//
// Usage examples:
//
//	// Basic configuration with Azure AD
//	config := RemoteStateConfigAzurerm{
//	    StorageAccountName: "mystorageaccount",
//	    ContainerName:      "terraform-state",
//	    ResourceGroupName:  "terraform-rg",
//	    SubscriptionID:     "12345678-1234-1234-1234-123456789abc",
//	    Key:                "prod/terraform.tfstate",
//	    UseAzureADAuth:     true,
//	}
//
//	// Service Principal authentication
//	config := RemoteStateConfigAzurerm{
//	    StorageAccountName: "mystorageaccount",
//	    ContainerName:      "terraform-state",
//	    ResourceGroupName:  "terraform-rg",
//	    SubscriptionID:     "12345678-1234-1234-1234-123456789abc",
//	    TenantID:           "87654321-4321-4321-4321-210987654321",
//	    ClientID:           "11111111-1111-1111-1111-111111111111",
//	    ClientSecret:       "client-secret-value",
//	    Key:                "prod/terraform.tfstate",
//	    UseAzureADAuth:     false,
//	}
//
//	// MSI authentication
//	config := RemoteStateConfigAzurerm{
//	    StorageAccountName: "mystorageaccount",
//	    ContainerName:      "terraform-state",
//	    ResourceGroupName:  "terraform-rg",
//	    SubscriptionID:     "12345678-1234-1234-1234-123456789abc",
//	    Key:                "prod/terraform.tfstate",
//	    UseMsi:             true,
//	}
type RemoteStateConfigAzurerm struct {
	// StorageAccountName specifies the name of the Azure Storage account for state storage.
	// Must be 3-24 characters long, contain only lowercase letters and numbers.
	// Must be globally unique across all Azure storage accounts.
	// Required field for all authentication methods.
	StorageAccountName string `mapstructure:"storage_account_name"`

	// ContainerName specifies the name of the blob container within the storage account.
	// Container names must be 3-63 characters long, contain only lowercase letters,
	// numbers, and hyphens. Cannot start or end with hyphens.
	// The container will be created if it doesn't exist (with appropriate permissions).
	// Required field for all authentication methods.
	ContainerName string `mapstructure:"container_name"`

	// ResourceGroupName specifies the name of the Azure resource group containing the storage account.
	// Must be 1-90 characters long and can contain alphanumeric characters, periods,
	// underscores, hyphens, and parentheses. Cannot end with a period.
	// Required field for all authentication methods.
	ResourceGroupName string `mapstructure:"resource_group_name"`

	// SubscriptionID specifies the Azure subscription ID where the storage account exists.
	// Must be a valid UUID format (e.g., "12345678-1234-1234-1234-123456789abc").
	// Required field for all authentication methods.
	// Environment variables: AZURE_SUBSCRIPTION_ID, ARM_SUBSCRIPTION_ID
	SubscriptionID string `mapstructure:"subscription_id"`

	// TenantID specifies the Azure Active Directory tenant ID.
	// Also called "Directory ID" in Azure portal.
	// Required for Service Principal authentication.
	// Format: UUID (e.g., "12345678-1234-1234-1234-123456789abc")
	// Environment variables: AZURE_TENANT_ID, ARM_TENANT_ID
	// Optional for Azure AD authentication (auto-discovered).
	TenantID string `mapstructure:"tenant_id"`

	// Location specifies the Azure region where the resource group and storage account should be created.
	// Required only when creating a new resource group.
	// Example: "eastus", "westeurope"
	Location string `mapstructure:"location"`

	// ClientID specifies the Azure Active Directory application client ID.
	// Also called "Application ID" in Azure portal.
	// Required for Service Principal authentication.
	// Format: UUID (e.g., "12345678-1234-1234-1234-123456789abc")
	// Environment variables: AZURE_CLIENT_ID, ARM_CLIENT_ID
	// Optional for other authentication methods.
	ClientID string `mapstructure:"client_id"`

	// ClientSecret specifies the client secret for Service Principal authentication.
	// This is a sensitive value that should be stored securely.
	// Required for Service Principal authentication.
	// Environment variables: AZURE_CLIENT_SECRET, ARM_CLIENT_SECRET
	// Should be treated as a password and never logged or exposed.
	// Optional for other authentication methods.
	ClientSecret string `mapstructure:"client_secret"`

	// Environment specifies the Azure cloud environment to use.
	// Valid values:
	// - "public" or "AzurePublicCloud": Azure Public Cloud (default)
	// - "government" or "AzureUSGovernmentCloud": Azure US Government Cloud
	// - "china" or "AzureChinaCloud": Azure China Cloud
	// - "german" or "AzureGermanCloud": Azure German Cloud (deprecated)
	// Environment variables: AZURE_ENVIRONMENT, ARM_ENVIRONMENT
	// Default: "public" (Azure Public Cloud)
	Environment string `mapstructure:"environment"`

	// EndpointURL specifies a custom endpoint URL for the Azure Storage service.
	// When specified, overrides the default storage endpoint for the selected environment.
	// Format: Full URL (e.g., "https://mystorageaccount.blob.core.windows.net/")
	// Used for custom endpoints, private endpoints, or testing scenarios.
	// Optional for all authentication methods.
	EndpointURL string `mapstructure:"endpoint"`

	// Key specifies the path/key for the state file within the container.
	// This is equivalent to the "key" parameter in other Terraform backends.
	// Should include the filename and any directory structure.
	// Example: "prod/terraform.tfstate" or "environments/staging/app.tfstate"
	// Required field for all authentication methods.
	Key string `mapstructure:"key"`

	// SasToken specifies the Shared Access Signature token for storage operations.
	// Must start with "?" and contain valid SAS token parameters.
	// Example: "?sv=2021-06-08&ss=b&srt=sco&sp=rwdlacx&se=2023-12-31T23:59:59Z&sig=..."
	// This is a sensitive value that should be stored securely.
	// Time-limited and scope-limited access to storage resources.
	// Optional for other authentication methods.
	SasToken string `mapstructure:"sas_token"`

	// Add padding to optimize struct size (align to 8 bytes for bool fields)
	_ [6]byte // 6 bytes padding to align the following bools to 8-byte boundary

	// UseMsi indicates whether to use Managed Service Identity authentication.
	// When true, the code will attempt to authenticate using Azure MSI.
	// Only works when running on Azure resources (VMs, App Service, Function Apps, etc.)
	// Automatically uses the system-assigned or user-assigned managed identity.
	// Mutually exclusive with UseAzureADAuth and Service Principal authentication.
	// Default: false
	UseMsi bool `mapstructure:"use_msi"`

	// UseAzureADAuth indicates whether to use Azure Active Directory authentication.
	// When true, uses Azure AD with automatic credential discovery.
	// This is now the default and recommended authentication method.
	// Supports various credential sources: CLI, environment variables, managed identity, etc.
	// Mutually exclusive with UseMsi and Service Principal authentication.
	// Default: true (Azure AD is the default authentication method)
	UseAzureADAuth bool `mapstructure:"use_azuread_auth"`
}

// ExtendedRemoteStateConfigAzurerm provides extended configuration for the Azure RM backend.
// This configuration combines the core remote state configuration with additional
// storage account bootstrapping options. It allows Terragrunt to automatically
// create and configure Azure Storage accounts for remote state storage.
//
// This configuration is used when you want Terragrunt to manage the storage account
// lifecycle (creation, configuration, updates) in addition to storing state files.
// It embeds both RemoteStateConfigAzurerm and StorageAccountBootstrapConfig.
//
// Usage examples:
//
//	// Basic extended configuration with automatic storage account creation
//	config := ExtendedRemoteStateConfigAzurerm{
//	    RemoteStateConfigAzurerm: RemoteStateConfigAzurerm{
//	        StorageAccountName: "mystorageaccount",
//	        ContainerName:      "terraform-state",
//	        ResourceGroupName:  "terraform-rg",
//	        SubscriptionID:     "12345678-1234-1234-1234-123456789abc",
//	        Key:                "prod/terraform.tfstate",
//	        UseAzureADAuth:     true,
//	    },
//	    StorageAccountConfig: StorageAccountBootstrapConfig{
//	        Location:                        "eastus",
//	        ResourceGroupName:               "terraform-rg",
//	        AccountKind:                     "StorageV2",
//	        AccountTier:                     "Standard",
//	        AccessTier:                      "Hot",
//	        ReplicationType:                 "LRS",
//	        EnableVersioning:                true,
//	        AllowBlobPublicAccess:           false,
//	        CreateStorageAccountIfNotExists: true,
//	        SkipStorageAccountUpdate:        false,
//	    },
//	    DisableBlobPublicAccess: true,
//	}
//
//	// Production configuration with geo-redundant storage
//	config := ExtendedRemoteStateConfigAzurerm{
//	    RemoteStateConfigAzurerm: RemoteStateConfigAzurerm{
//	        StorageAccountName: "prodstorageaccount",
//	        ContainerName:      "terraform-state",
//	        ResourceGroupName:  "production-rg",
//	        SubscriptionID:     "12345678-1234-1234-1234-123456789abc",
//	        Key:                "prod/terraform.tfstate",
//	        UseAzureADAuth:     true,
//	    },
//	    StorageAccountConfig: StorageAccountBootstrapConfig{
//	        Location:                        "eastus",
//	        ResourceGroupName:               "production-rg",
//	        AccountKind:                     "StorageV2",
//	        AccountTier:                     "Standard",
//	        AccessTier:                      "Hot",
//	        ReplicationType:                 "GRS", // Geo-redundant for production
//	        EnableVersioning:                true,
//	        AllowBlobPublicAccess:           false,
//	        CreateStorageAccountIfNotExists: true,
//	        SkipStorageAccountUpdate:        false,
//	        StorageAccountTags: map[string]string{
//	            "Environment": "production",
//	            "Team":        "platform",
//	            "CostCenter":  "engineering",
//	        },
//	    },
//	    DisableBlobPublicAccess: true,
//	}
type ExtendedRemoteStateConfigAzurerm struct {
	// StorageAccountConfig embeds the storage account bootstrap configuration.
	// This configuration controls how the storage account is created and configured.
	// The ",squash" mapstructure tag flattens this struct's fields into the parent.
	// All fields from StorageAccountBootstrapConfig are available at the top level.
	StorageAccountConfig StorageAccountBootstrapConfig `mapstructure:",squash"`

	// RemoteStateConfigAzurerm embeds the core remote state configuration.
	// This configuration defines how to connect to and use the Azure Storage backend.
	// The ",squash" mapstructure tag flattens this struct's fields into the parent.
	// All fields from RemoteStateConfigAzurerm are available at the top level.
	RemoteStateConfigAzurerm RemoteStateConfigAzurerm `mapstructure:",squash"`

	// DisableBlobPublicAccess provides an additional control to disable public access to blobs.
	// When true, overrides the AllowBlobPublicAccess setting to ensure public access is disabled.
	// This provides an extra layer of security configuration.
	// When false, the AllowBlobPublicAccess setting from StorageAccountConfig is used.
	// This field exists for backward compatibility and explicit security control.
	// Default: false (use AllowBlobPublicAccess setting from StorageAccountConfig)
	DisableBlobPublicAccess bool `mapstructure:"disable_blob_public_access"`

	// 7 bytes padding to align struct size for optimal memory layout
	_ [7]byte
}

// Config represents the configuration for Azure Storage backend.
type Config map[string]interface{}

// FilterOutTerragruntKeys returns a new map without Terragrunt-specific keys.
func (cfg Config) FilterOutTerragruntKeys() map[string]interface{} {
	filtered := make(map[string]interface{})

	for key, val := range cfg {
		if slices.Contains(terragruntOnlyConfigs, key) {
			continue
		}

		filtered[key] = val
	}

	return filtered
}

// ParseExtendedAzureConfig parses the config into an ExtendedRemoteStateConfigAzurerm.
func (cfg Config) ParseExtendedAzureConfig() (*ExtendedRemoteStateConfigAzurerm, error) {
	var extConfig ExtendedRemoteStateConfigAzurerm

	// Set default values before decoding
	extConfig.StorageAccountConfig.CreateStorageAccountIfNotExists = false
	extConfig.StorageAccountConfig.EnableVersioning = true
	extConfig.StorageAccountConfig.AllowBlobPublicAccess = false // Default to secure option

	// Check if use_msi is explicitly set before defaulting to Azure AD auth
	useMsi, hasMsi := cfg["use_msi"]
	if !hasMsi || useMsi != true {
		extConfig.RemoteStateConfigAzurerm.UseAzureADAuth = true // Default to Azure AD auth only if not using MSI
	}

	if err := mapstructure.Decode(cfg, &extConfig); err != nil {
		return nil, fmt.Errorf("failed to decode Azure config: %w", err)
	}

	return &extConfig, nil
}

// ExtendedAzureConfig parses and validates the config.
func (cfg Config) ExtendedAzureConfig() (*ExtendedRemoteStateConfigAzurerm, error) {
	extConfig, parseErr := cfg.ParseExtendedAzureConfig()
	if parseErr != nil {
		return nil, parseErr
	}

	if err := extConfig.Validate(); err != nil {
		return nil, err
	}

	return extConfig, nil
}

// Validate checks if all required fields are set and validates auth methods.
func (cfg *ExtendedRemoteStateConfigAzurerm) Validate() error {
	if cfg.RemoteStateConfigAzurerm.StorageAccountName == "" {
		return MissingRequiredAzureRemoteStateConfig("storage_account_name")
	}

	if cfg.RemoteStateConfigAzurerm.ContainerName == "" {
		return MissingRequiredAzureRemoteStateConfig("container_name")
	}

	if cfg.RemoteStateConfigAzurerm.Key == "" {
		return MissingRequiredAzureRemoteStateConfig("key")
	}

	// Validate auth method combinations
	hasSasToken := cfg.RemoteStateConfigAzurerm.SasToken != ""

	// Service principal requires all three values to be set
	hasServicePrincipal := cfg.RemoteStateConfigAzurerm.ClientID != "" ||
		cfg.RemoteStateConfigAzurerm.ClientSecret != "" ||
		cfg.RemoteStateConfigAzurerm.TenantID != ""

	hasAzureAD := cfg.RemoteStateConfigAzurerm.UseAzureADAuth
	hasMSI := cfg.RemoteStateConfigAzurerm.UseMsi

	// Check for multiple auth methods
	var authCount int

	if hasSasToken {
		authCount++
	}

	if hasServicePrincipal {
		// Check if all required fields are present for service principal auth
		if cfg.RemoteStateConfigAzurerm.ClientID != "" &&
			cfg.RemoteStateConfigAzurerm.ClientSecret != "" &&
			cfg.RemoteStateConfigAzurerm.TenantID != "" {
			// If service principal seems to be the intended auth method
			authCount++

			// Validate required fields for service principal auth
			if cfg.RemoteStateConfigAzurerm.SubscriptionID == "" {
				return NewServicePrincipalMissingSubscriptionIDError()
			}
		} else if cfg.RemoteStateConfigAzurerm.ClientID != "" ||
			cfg.RemoteStateConfigAzurerm.ClientSecret != "" ||
			cfg.RemoteStateConfigAzurerm.TenantID != "" {
			// If only some service principal fields are provided, it's incomplete
			missing := []string{}

			if cfg.RemoteStateConfigAzurerm.ClientID == "" {
				missing = append(missing, "client_id")
			}

			if cfg.RemoteStateConfigAzurerm.ClientSecret == "" {
				missing = append(missing, "client_secret")
			}

			if cfg.RemoteStateConfigAzurerm.TenantID == "" {
				missing = append(missing, "tenant_id")
			}

			return WrapIncompleteServicePrincipalError(missing)
		}
	}

	if hasAzureAD {
		authCount++
	}

	if hasMSI {
		authCount++
	}

	if authCount > 1 {
		return NewMultipleAuthMethodsSpecifiedError()
	}

	return nil
}

// CacheKey returns a key that uniquely identifies this config.
func (cfg *ExtendedRemoteStateConfigAzurerm) CacheKey() string {
	return fmt.Sprintf("%s-%s-%s", cfg.RemoteStateConfigAzurerm.StorageAccountName, cfg.RemoteStateConfigAzurerm.ContainerName, cfg.RemoteStateConfigAzurerm.Key)
}

// terragruntOnlyConfigs are settings that appear in the remote_state config that are only used by
// Terragrunt and not passed on to the Azure backend configuration.
var terragruntOnlyConfigs = []string{
	// Storage account bootstrap configuration
	"create_storage_account_if_not_exists",
	"skip_storage_account_update",
	"resource_group_name", // Resource group is only used during bootstrap

	// Storage account creation parameters (used only during bootstrap)
	"location",
	"account_kind",
	"account_tier",
	"account_replication_type",
	"replication_type", // Alternative name for account_replication_type
	"access_tier",
	"enable_versioning",
	"allow_blob_public_access",
	"disable_blob_public_access", // Legacy name for allow_blob_public_access
	"storage_account_tags",
}
