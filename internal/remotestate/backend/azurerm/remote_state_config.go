package azurerm

import (
	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
)

// terragruntOnlyConfigs lists configuration keys that are consumed by
// Terragrunt during bootstrap/cleanup but must NOT be forwarded to the
// underlying terraform azurerm backend block (terraform would reject
// them as unknown).
var terragruntOnlyConfigs = []string{
	"location",
	"skip_resource_group_creation",
	"skip_storage_account_creation",
	"skip_container_creation",
	"skip_versioning",
	"enable_soft_delete",
	"soft_delete_retention_days",
	"account_tier",
	"account_replication_type",
	"account_kind",
	"access_tier",
	"tags",
	"assign_storage_blob_data_owner",
	"principal_id",
}

// RemoteStateConfigAzureRM mirrors the keys that the terraform azurerm
// backend itself accepts. These are passed through to terraform init.
type RemoteStateConfigAzureRM struct {
	StorageAccountName string `mapstructure:"storage_account_name"`
	ContainerName      string `mapstructure:"container_name"`
	Key                string `mapstructure:"key"`
	ResourceGroupName  string `mapstructure:"resource_group_name"`

	SubscriptionID string `mapstructure:"subscription_id"`
	TenantID       string `mapstructure:"tenant_id"`
	ClientID       string `mapstructure:"client_id"`
	ClientSecret   string `mapstructure:"client_secret"`

	AccessKey string `mapstructure:"access_key"`
	SasToken  string `mapstructure:"sas_token"`

	Endpoint       string `mapstructure:"endpoint"`
	EndpointSuffix string `mapstructure:"endpoint_suffix"`
	Environment    string `mapstructure:"environment"`
	MSIEndpoint    string `mapstructure:"msi_endpoint"`
	OIDCToken      string `mapstructure:"oidc_token"`
	OIDCTokenPath  string `mapstructure:"oidc_token_file_path"`

	Snapshot       bool `mapstructure:"snapshot"`
	UseMSI         bool `mapstructure:"use_msi"`
	UseOIDC        bool `mapstructure:"use_oidc"`
	UseAzureADAuth bool `mapstructure:"use_azuread_auth"`
}

// CacheKey returns a unique key for the given config used to cache
// per-account bootstrap state.
func (cfg *RemoteStateConfigAzureRM) CacheKey() string {
	return cfg.StorageAccountName + "/" + cfg.ContainerName
}

// ExtendedRemoteStateConfigAzureRM is the full config map: terraform
// passthrough fields plus terragrunt-only keys (resource group / storage
// account creation policy, soft-delete, RBAC bootstrap etc.).
type ExtendedRemoteStateConfigAzureRM struct {
	Tags                   map[string]string `mapstructure:"tags"`
	AccountTier            string            `mapstructure:"account_tier"`
	AccountReplicationType string            `mapstructure:"account_replication_type"`
	AccountKind            string            `mapstructure:"account_kind"`
	AccessTier             string            `mapstructure:"access_tier"`
	Location               string            `mapstructure:"location"`
	// PrincipalID is the AAD object ID that should receive the Storage
	// Blob Data Owner assignment when AssignBlobDataOwner is true. Required
	// in that case — Terragrunt does not infer the principal from the
	// active credential.
	PrincipalID                string                   `mapstructure:"principal_id"`
	RemoteStateConfigAzureRM   RemoteStateConfigAzureRM `mapstructure:",squash"`
	SoftDeleteRetentionDays    int                      `mapstructure:"soft_delete_retention_days"`
	SkipResourceGroupCreation  bool                     `mapstructure:"skip_resource_group_creation"`
	SkipStorageAccountCreation bool                     `mapstructure:"skip_storage_account_creation"`
	SkipContainerCreation      bool                     `mapstructure:"skip_container_creation"`
	SkipVersioning             bool                     `mapstructure:"skip_versioning"`
	EnableSoftDelete           bool                     `mapstructure:"enable_soft_delete"`
	AssignBlobDataOwner        bool                     `mapstructure:"assign_storage_blob_data_owner"`
}

// Validate validates the configuration for AzureRM remote state. Required
// fields (storage_account_name / container_name / key) are always checked.
// resource_group_name is required for any control-plane operation, but its
// absence is tolerated when both skip_resource_group_creation and
// skip_storage_account_creation are true (i.e. user is opting out of all
// ARM-side bootstrap and only doing data-plane state IO).
func (cfg *ExtendedRemoteStateConfigAzureRM) Validate() error {
	rsc := &cfg.RemoteStateConfigAzureRM

	if rsc.StorageAccountName == "" {
		return MissingRequiredAzureRMRemoteStateConfig("storage_account_name")
	}

	if rsc.ContainerName == "" {
		return MissingRequiredAzureRMRemoteStateConfig("container_name")
	}

	if rsc.Key == "" {
		return MissingRequiredAzureRMRemoteStateConfig("key")
	}

	armOpsRequired := !cfg.SkipResourceGroupCreation || !cfg.SkipStorageAccountCreation
	if armOpsRequired && rsc.ResourceGroupName == "" {
		return MissingRequiredAzureRMRemoteStateConfig("resource_group_name")
	}

	if cfg.AssignBlobDataOwner && cfg.PrincipalID == "" {
		return MissingRequiredAzureRMRemoteStateConfig("principal_id")
	}

	return nil
}

// GetAzureSessionConfig converts the parsed config into an
// azurehelper.AzureSessionConfig the builder can consume.
// Endpoint / endpoint_suffix / oidc_token are terraform-only — terraform
// init receives them via GetTFInitArgs but they have no representation in
// azurehelper, so terragrunt's own bootstrap does not consume them.
func (cfg *ExtendedRemoteStateConfigAzureRM) GetAzureSessionConfig() *azurehelper.AzureSessionConfig {
	rsc := &cfg.RemoteStateConfigAzureRM

	return &azurehelper.AzureSessionConfig{
		StorageAccountName: rsc.StorageAccountName,
		ContainerName:      rsc.ContainerName,
		ResourceGroupName:  rsc.ResourceGroupName,
		Location:           cfg.Location,
		SubscriptionID:     rsc.SubscriptionID,
		TenantID:           rsc.TenantID,
		ClientID:           rsc.ClientID,
		ClientSecret:       rsc.ClientSecret,
		AccessKey:          rsc.AccessKey,
		SasToken:           rsc.SasToken,
		CloudEnvironment:   rsc.Environment,
		OIDCTokenFilePath:  rsc.OIDCTokenPath,
		UseMSI:             rsc.UseMSI,
		UseOIDC:            rsc.UseOIDC,
		UseAzureADAuth:     rsc.UseAzureADAuth,
	}
}
