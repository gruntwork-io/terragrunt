package azurerm

import (
	"fmt"
	"slices"

	"github.com/mitchellh/mapstructure"
)

// BackendName is the name of the Azure RM backend
const BackendName = "azurerm"

// terragruntOnlyConfigs are settings that appear in the remote_state config that are only used by
// Terragrunt and not passed on to the Azure backend configuration
var terragruntOnlyConfigs = []string{
	"container_tags",
	"disable_blob_public_access",
}

// ExtendedRemoteStateConfigAzurerm provides extended configuration for the Azure RM backend
type ExtendedRemoteStateConfigAzurerm struct {
	ContainerTags             map[string]string    `mapstructure:"container_tags"`
	DisableBlobPublicAccess   bool                 `mapstructure:"disable_blob_public_access"`
	RemoteStateConfigAzurerm  RemoteStateConfigAzurerm `mapstructure:",squash"`
}

// RemoteStateConfigAzurerm represents the configuration for Azure Storage backend
type RemoteStateConfigAzurerm struct {
	StorageAccountName   string `mapstructure:"storage_account_name"`
	// Deprecated: Use Azure AD authentication instead
	StorageAccountKey    string `mapstructure:"storage_account_key"`
	ContainerName        string `mapstructure:"container_name"`
	Key                  string `mapstructure:"key"`
	ResourceGroupName    string `mapstructure:"resource_group_name"`
	// Deprecated: Use Azure AD authentication instead
	ConnectionString     string `mapstructure:"connection_string"`
	SasToken            string `mapstructure:"sas_token"`
	SubscriptionID      string `mapstructure:"subscription_id"`
	TenantID            string `mapstructure:"tenant_id"`
	ClientID            string `mapstructure:"client_id"`
	ClientSecret        string `mapstructure:"client_secret"`
	Environment         string `mapstructure:"environment"`
	EndpointURL         string `mapstructure:"endpoint"`
	UseMsi             bool   `mapstructure:"use_msi"`
	UseAzureADAuth     bool   `mapstructure:"use_azuread_auth"`
}

// Config represents the configuration for Azure Storage backend
type Config map[string]interface{}

// FilterOutTerragruntKeys returns a new map without Terragrunt-specific keys
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

// ParseExtendedAzureConfig parses the config into an ExtendedRemoteStateConfigAzurerm
func (cfg Config) ParseExtendedAzureConfig() (*ExtendedRemoteStateConfigAzurerm, error) {
	var extConfig ExtendedRemoteStateConfigAzurerm

	if err := mapstructure.Decode(cfg, &extConfig); err != nil {
		return nil, fmt.Errorf("failed to decode Azure config: %w", err)
	}

	return &extConfig, nil
}

// ExtendedAzureConfig parses and validates the config
func (cfg Config) ExtendedAzureConfig() (*ExtendedRemoteStateConfigAzurerm, error) {
	extConfig, err := cfg.ParseExtendedAzureConfig()
	if err != nil {
		return nil, err
	}

	if err := extConfig.Validate(); err != nil {
		return nil, err
	}

	return extConfig, nil
}

// Validate checks if all required fields are set and validates auth methods
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
	hasKeyAuth := cfg.RemoteStateConfigAzurerm.ConnectionString != "" || cfg.RemoteStateConfigAzurerm.StorageAccountKey != ""
	hasSasToken := cfg.RemoteStateConfigAzurerm.SasToken != ""
	
	// Service principal requires all three values to be set
	hasServicePrincipal := false
	if cfg.RemoteStateConfigAzurerm.ClientID != "" || cfg.RemoteStateConfigAzurerm.ClientSecret != "" ||
		cfg.RemoteStateConfigAzurerm.TenantID != "" {
		hasServicePrincipal = true
	}

	hasAzureAD := cfg.RemoteStateConfigAzurerm.UseAzureADAuth
	hasMSI := cfg.RemoteStateConfigAzurerm.UseMsi

	// Check for multiple auth methods
	var authCount int
	if hasKeyAuth {
		authCount++
	}
	if hasSasToken {
		authCount++
	} 
	if hasServicePrincipal {
		// Only validate service principal fields if it's actually being used
		if !hasKeyAuth && !hasSasToken && !hasAzureAD && !hasMSI {
			if cfg.RemoteStateConfigAzurerm.ClientID == "" || cfg.RemoteStateConfigAzurerm.ClientSecret == "" ||
				cfg.RemoteStateConfigAzurerm.TenantID == "" || cfg.RemoteStateConfigAzurerm.SubscriptionID == "" {
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
				if cfg.RemoteStateConfigAzurerm.SubscriptionID == "" {
					missing = append(missing, "subscription_id")
				}
				return fmt.Errorf("incomplete service principal configuration: missing required fields: %v", missing)
			}
		}
		authCount++
	}
	if hasAzureAD {
		authCount++
	}
	if hasMSI {
		authCount++
	}

	if authCount > 1 {
		return fmt.Errorf("cannot specify multiple authentication methods: choose one of storage account key, SAS token, service principal, Azure AD auth, or MSI")
	}

	return nil
}

// CacheKey returns a key that uniquely identifies this config
func (cfg *ExtendedRemoteStateConfigAzurerm) CacheKey() string {
	return fmt.Sprintf("%s-%s-%s", cfg.RemoteStateConfigAzurerm.StorageAccountName, cfg.RemoteStateConfigAzurerm.ContainerName, cfg.RemoteStateConfigAzurerm.Key)
}


