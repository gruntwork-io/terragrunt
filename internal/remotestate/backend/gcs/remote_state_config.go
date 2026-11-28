package gcs

import (
	"github.com/gruntwork-io/terragrunt/internal/gcphelper"
)

// These are settings that can appear in the remote_state config that are ONLY used by Terragrunt and NOT forwarded
// to the underlying Terraform backend configuration.
var terragruntOnlyConfigs = []string{
	"project",
	"location",
	"gcs_bucket_labels",
	"skip_bucket_versioning",
	"skip_bucket_creation",
	"enable_bucket_policy_only",
}

// ExtendedRemoteStateConfigGCS holds the GCS specific configuration options.
//
// It keeps the config key 'gcs_bucket_labels' apart from the others, as it is specific to the gcs backend and
// only used by terragrunt to label the gcs bucket in case it has to create it.
type ExtendedRemoteStateConfigGCS struct {
	GCSBucketLabels        map[string]string    `mapstructure:"gcs_bucket_labels"`
	Project                string               `mapstructure:"project"`
	Location               string               `mapstructure:"location"`
	RemoteStateConfigGCS   RemoteStateConfigGCS `mapstructure:",squash"`
	SkipBucketVersioning   bool                 `mapstructure:"skip_bucket_versioning"`
	SkipBucketCreation     bool                 `mapstructure:"skip_bucket_creation"`
	EnableBucketPolicyOnly bool                 `mapstructure:"enable_bucket_policy_only"`
}

// Validate validates the configuration for GCS remote state.
func (cfg *ExtendedRemoteStateConfigGCS) Validate() error {
	var bucketName = cfg.RemoteStateConfigGCS.Bucket

	// Bucket is always a required configuration parameter when not skipping bucket creation
	// so we check it here to make sure we have handle to the bucket
	// before we start validating the rest of the configuration.
	if bucketName == "" {
		return MissingRequiredGCSRemoteStateConfig("bucket")
	}

	return nil
}

// RemoteStateConfigGCS is a representation of the configuration
// options available for GCS remote state.
type RemoteStateConfigGCS struct {
	Bucket        string `mapstructure:"bucket"`
	Credentials   string `mapstructure:"credentials"`
	AccessToken   string `mapstructure:"access_token"`
	Prefix        string `mapstructure:"prefix"`
	Path          string `mapstructure:"path"`
	EncryptionKey string `mapstructure:"encryption_key"`

	ImpersonateServiceAccount          string   `mapstructure:"impersonate_service_account"`
	ImpersonateServiceAccountDelegates []string `mapstructure:"impersonate_service_account_delegates"`
}

// CacheKey returns a unique key for the given GCS config that can be used to cache the initialization.
func (cfg *RemoteStateConfigGCS) CacheKey() string {
	return cfg.Bucket
}

// GetGCPSessionConfig returns a GcpSessionConfig from the ExtendedRemoteStateConfigGCS configuration.
func (cfg *ExtendedRemoteStateConfigGCS) GetGCPSessionConfig() *gcphelper.GCPSessionConfig {
	return &gcphelper.GCPSessionConfig{
		Credentials:                        cfg.RemoteStateConfigGCS.Credentials,
		AccessToken:                        cfg.RemoteStateConfigGCS.AccessToken,
		ImpersonateServiceAccount:          cfg.RemoteStateConfigGCS.ImpersonateServiceAccount,
		ImpersonateServiceAccountDelegates: cfg.RemoteStateConfigGCS.ImpersonateServiceAccountDelegates,
	}
}
