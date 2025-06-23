package azurerm

import (
	"errors"
	"fmt"
	"slices"

	"github.com/mitchellh/mapstructure"
)

// BackendName is the name of the Azure RM backend.

// StorageAccountBootstrapConfig represents the configuration for Azure Storage account bootstrapping
type StorageAccountBootstrapConfig struct {
	// Maps first (larger alignment requirements)
	StorageAccountTags map[string]string `mapstructure:"storage_account_tags"`
	// Then string fields
	Location          string `mapstructure:"location"`
	ResourceGroupName string `mapstructure:"resource_group_name"`
	AccountKind       string `mapstructure:"account_kind"`
	AccountTier       string `mapstructure:"account_tier"`
	AccessTier        string `mapstructure:"access_tier"`
	ReplicationType   string `mapstructure:"replication_type"`
	// Group boolean fields together at the end
	EnableVersioning                bool `mapstructure:"enable_versioning"`
	AllowBlobPublicAccess           bool `mapstructure:"allow_blob_public_access"`
	EnableHierarchicalNS            bool `mapstructure:"enable_hierarchical_namespace"`
	CreateStorageAccountIfNotExists bool `mapstructure:"create_storage_account_if_not_exists"`
	SkipStorageAccountUpdate        bool `mapstructure:"skip_storage_account_update"`
}

// RemoteStateConfigAzurerm represents the configuration for Azure Storage backend.
type RemoteStateConfigAzurerm struct {
	// Group all string fields together for optimal alignment (16 bytes each)
	StorageAccountName string `mapstructure:"storage_account_name"`
	ContainerName      string `mapstructure:"container_name"`
	ResourceGroupName  string `mapstructure:"resource_group_name"`
	SubscriptionID     string `mapstructure:"subscription_id"`
	TenantID           string `mapstructure:"tenant_id"`
	ClientID           string `mapstructure:"client_id"`
	ClientSecret       string `mapstructure:"client_secret"`
	Environment        string `mapstructure:"environment"`
	EndpointURL        string `mapstructure:"endpoint"`
	Key                string `mapstructure:"key"`
	SasToken           string `mapstructure:"sas_token"`

	// Add padding to optimize struct size
	_ struct{}

	// Group bool fields together at the end (1 byte each)
	UseMsi         bool `mapstructure:"use_msi"`
	UseAzureADAuth bool `mapstructure:"use_azuread_auth"` // Default is now true
}

// ExtendedRemoteStateConfigAzurerm provides extended configuration for the Azure RM backend.
type ExtendedRemoteStateConfigAzurerm struct {
	// Put larger structs first
	StorageAccountConfig     StorageAccountBootstrapConfig `mapstructure:",squash"` // storage account bootstrap config
	RemoteStateConfigAzurerm RemoteStateConfigAzurerm      `mapstructure:",squash"` // large struct
	// Put smaller fields at the end
	DisableBlobPublicAccess bool     `mapstructure:"disable_blob_public_access"` // 1 byte at end
	_                       struct{} // padding for optimal alignment
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
				return errors.New("subscription_id is required when using service principal authentication")
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

			return fmt.Errorf("incomplete service principal configuration: missing required fields: %v", missing)
		}
	}

	if hasAzureAD {
		authCount++
	}

	if hasMSI {
		authCount++
	}

	if authCount > 1 {
		return errors.New("cannot specify multiple authentication methods: choose one of storage account key, SAS token, service principal, Azure AD auth, or MSI")
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
	"enable_hierarchical_namespace",
	"storage_account_tags",
}
