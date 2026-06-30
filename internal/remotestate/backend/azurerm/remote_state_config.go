package azurerm

import (
	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
)

// terragruntOnlyConfigs lists the keys that may appear in the remote_state
// config but are used ONLY by Terragrunt to bootstrap the storage account and
// container. They are NOT forwarded to the underlying `azurerm` Terraform
// backend configuration (which would reject unknown keys).
var terragruntOnlyConfigs = []string{
	"location",
	"account_tier",
	"account_replication_type",
	"account_kind",
	"access_tier",
	"tags",
	"skip_resource_group_creation",
	"skip_storage_account_creation",
	"skip_container_creation",
	"skip_versioning",
	"enable_soft_delete",
	"soft_delete_retention_days",
	"allow_blob_public_access",
	// msi_resource_id is used by Terragrunt to select a user-assigned managed
	// identity during bootstrap, but it is NOT a valid azurerm backend argument
	// (the backend uses msi_endpoint), so it must not reach `tofu init`.
	"msi_resource_id",
}

// ExtendedRemoteStateConfigAzurerm holds the azurerm backend configuration plus
// the Terragrunt-only bootstrap options (location, SKU, skip_* toggles) that
// are stripped before the config is handed to `tofu init -backend-config`.
type ExtendedRemoteStateConfigAzurerm struct {
	Tags                     map[string]string        `mapstructure:"tags"`
	Location                 string                   `mapstructure:"location"`
	AccountTier              string                   `mapstructure:"account_tier"`
	AccountReplicationType   string                   `mapstructure:"account_replication_type"`
	AccountKind              string                   `mapstructure:"account_kind"`
	AccessTier               string                   `mapstructure:"access_tier"`
	RemoteStateConfigAzurerm RemoteStateConfigAzurerm `mapstructure:",squash"`
	SoftDeleteRetentionDays  int                      `mapstructure:"soft_delete_retention_days"`

	SkipResourceGroupCreation  bool `mapstructure:"skip_resource_group_creation"`
	SkipStorageAccountCreation bool `mapstructure:"skip_storage_account_creation"`
	SkipContainerCreation      bool `mapstructure:"skip_container_creation"`
	SkipVersioning             bool `mapstructure:"skip_versioning"`
	EnableSoftDelete           bool `mapstructure:"enable_soft_delete"`
	AllowBlobPublicAccess      bool `mapstructure:"allow_blob_public_access"`
}

// RemoteStateConfigAzurerm mirrors the configuration keys accepted by the
// `azurerm` Terraform/OpenTofu backend. These are forwarded verbatim to
// `tofu init -backend-config`.
type RemoteStateConfigAzurerm struct {
	StorageAccountName string `mapstructure:"storage_account_name"`
	ContainerName      string `mapstructure:"container_name"`
	Key                string `mapstructure:"key"`
	ResourceGroupName  string `mapstructure:"resource_group_name"`
	SubscriptionID     string `mapstructure:"subscription_id"`
	TenantID           string `mapstructure:"tenant_id"`
	ClientID           string `mapstructure:"client_id"`
	ClientSecret       string `mapstructure:"client_secret"`
	SasToken           string `mapstructure:"sas_token"`
	AccessKey          string `mapstructure:"access_key"`
	Environment        string `mapstructure:"environment"`
	MSIResourceID      string `mapstructure:"msi_resource_id"`
	OIDCTokenFilePath  string `mapstructure:"oidc_token_file_path"`
	UseAzureADAuth     bool   `mapstructure:"use_azuread_auth"`
	UseMSI             bool   `mapstructure:"use_msi"`
	UseOIDC            bool   `mapstructure:"use_oidc"`
	// Snapshot is a valid azurerm backend boolean argument. It is declared here
	// (even though Terragrunt does not act on it) so NormalizeBoolValues coerces
	// a string "true"/"false" into a real bool before forwarding to `tofu init`.
	Snapshot bool `mapstructure:"snapshot"`
}

// CacheKey returns a unique key identifying the bootstrapped container so that
// repeated Bootstrap calls for the same account/container short-circuit.
func (cfg *RemoteStateConfigAzurerm) CacheKey() string {
	return cfg.StorageAccountName + "/" + cfg.ContainerName
}

// GetAzureSessionConfig maps the parsed backend config to the session config
// consumed by internal/azurehelper.
func (cfg *ExtendedRemoteStateConfigAzurerm) GetAzureSessionConfig() *azurehelper.AzureSessionConfig {
	rs := cfg.RemoteStateConfigAzurerm

	return &azurehelper.AzureSessionConfig{
		SubscriptionID:     rs.SubscriptionID,
		TenantID:           rs.TenantID,
		ClientID:           rs.ClientID,
		ClientSecret:       rs.ClientSecret,
		StorageAccountName: rs.StorageAccountName,
		ResourceGroupName:  rs.ResourceGroupName,
		ContainerName:      rs.ContainerName,
		Location:           cfg.Location,
		MSIResourceID:      rs.MSIResourceID,
		SasToken:           rs.SasToken,
		AccessKey:          rs.AccessKey,
		OIDCTokenFilePath:  rs.OIDCTokenFilePath,
		CloudEnvironment:   rs.Environment,
		UseAzureADAuth:     rs.UseAzureADAuth,
		UseMSI:             rs.UseMSI,
		UseOIDC:            rs.UseOIDC,
	}
}

// StorageAccountConfig maps the parsed backend config to the storage-account
// creation config consumed by internal/azurehelper during Bootstrap.
func (cfg *ExtendedRemoteStateConfigAzurerm) StorageAccountConfig() *azurehelper.StorageAccountConfig {
	rs := cfg.RemoteStateConfigAzurerm

	return &azurehelper.StorageAccountConfig{
		Name:              rs.StorageAccountName,
		ResourceGroupName: rs.ResourceGroupName,
		Location:          cfg.Location,
		AccountKind:       cfg.AccountKind,
		AccountTier:       cfg.AccountTier,
		ReplicationType:   cfg.AccountReplicationType,
		AccessTier:        cfg.AccessTier,
		Tags:              cfg.Tags,
		// EnableVersioning is intentionally left false here: bootstrapAccount
		// converges versioning (and soft delete) on both new and pre-existing
		// accounts, so setting it on Create too would be a redundant ARM call.
		AllowBlobPublicAccess: cfg.AllowBlobPublicAccess,
	}
}

// Validate checks that the required azurerm remote-state keys are present.
func (cfg *ExtendedRemoteStateConfigAzurerm) Validate() error {
	rs := cfg.RemoteStateConfigAzurerm

	if rs.StorageAccountName == "" {
		return MissingRequiredAzurermRemoteStateConfigError("storage_account_name")
	}

	if rs.ContainerName == "" {
		return MissingRequiredAzurermRemoteStateConfigError("container_name")
	}

	if rs.Key == "" {
		return MissingRequiredAzurermRemoteStateConfigError("key")
	}

	// resource_group_name is required only for ARM control-plane operations
	// (creating / inspecting the storage account). Validation cannot see the
	// resolved auth method, so it is enforced at the ARM call sites
	// (NewStorageAccountClient returns ErrResourceGroupNameRequired) rather than
	// here, which would wrongly reject data-plane-only (SAS / access-key) configs.

	return nil
}
