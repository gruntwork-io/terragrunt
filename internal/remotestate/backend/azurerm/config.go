package azurerm

import (
	"fmt"
	"slices"

	"github.com/hashicorp/consul/template/mapstructure"
	"github.com/pkg/errors"
)

const BackendName = "azurerm"

// These are settings that appear in the remote_state config that are only used by
// Terragrunt and not passed on to the Azure backend configuration
var terragruntOnlyConfigs = []string{
	"skip_blob_versioning",
	"disable_blob_public_access",
	"container_tags",
}

// These are settings that can appear in the remote_state config that are ONLY used by Terragrunt
// and NOT forwarded to the underlying Terraform backend configuration
var terragruntOnlyConfigs = []string{
	"skip_blob_versioning",
	"disable_blob_public_access",
	"container_tags",
}

type ExtendedRemoteStateConfigAzurerm struct {
	ContainerTags             map[string]string    `mapstructure:"container_tags"`
	SkipBlobVersioning        bool                 `mapstructure:"skip_blob_versioning"`
	DisableBlobPublicAccess   bool                 `mapstructure:"disable_blob_public_access"`
	RemoteStateConfigAzurerm  RemoteStateConfigAzurerm `mapstructure:",squash"`
}

// RemoteStateConfigAzurerm represents the configuration for Azure Storage backend
type RemoteStateConfigAzurerm struct {
	StorageAccountName   string `mapstructure:"storage_account_name"`
	ContainerName        string `mapstructure:"container_name"`
	Key                  string `mapstructure:"key"`
	ResourceGroupName    string `mapstructure:"resource_group_name"`
	ConnectionString     string `mapstructure:"connection_string"`
	SasToken            string `mapstructure:"sas_token"`
	SubscriptionID      string `mapstructure:"subscription_id"`
	TenantID            string `mapstructure:"tenant_id"`
	ClientID            string `mapstructure:"client_id"`
	ClientSecret        string `mapstructure:"client_secret"`
	Environment         string `mapstructure:"environment"`
	EndpointUrl         string `mapstructure:"endpoint"`
	UseMsi             bool   `mapstructure:"use_msi"`
}

// Config represents the configuration for Azure Storage backend
type Config map[string]interface{}

// FilterOutTerragruntKeys returns a new map with all Terragrunt-only keys removed
func (cfg Config) FilterOutTerragruntKeys() Config {
	filtered := make(Config)
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
		return nil, errors.New(err)
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

// Validate checks if all required fields are set
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

	return nil
}

// CacheKey returns a unique key for the Azure config
func (cfg *RemoteStateConfigAzurerm) CacheKey() string {
	return fmt.Sprintf("%s/%s/%s", cfg.StorageAccountName, cfg.ContainerName, cfg.Key)
}
