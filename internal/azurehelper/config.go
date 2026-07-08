package azurehelper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// AzureSessionConfig holds the per-backend configuration needed to authenticate
// against Azure and identify the storage account being managed. Fields populated
// from a remote_state backend block, with environment variable fallbacks applied
// during Build.
type AzureSessionConfig struct {
	SubscriptionID     string
	TenantID           string
	ClientID           string
	ClientSecret       string
	StorageAccountName string
	ResourceGroupName  string
	ContainerName      string
	Location           string
	MSIResourceID      string
	SasToken           string
	AccessKey          string
	// OIDCTokenFilePath is the path to a federated identity token file used
	// for OIDC / workload identity authentication. Falls back to
	// ARM_OIDC_TOKEN_FILE_PATH or AZURE_FEDERATED_TOKEN_FILE.
	OIDCTokenFilePath string
	// CloudEnvironment selects a sovereign cloud. Accepted values:
	// "" / "public" (default), "government" / "usgovernment", "china".
	CloudEnvironment string
	UseAzureADAuth   bool
	UseMSI           bool
	UseOIDC          bool
}

// AzureConfigBuilder builds an AzureConfig using the builder pattern.
// Use NewAzureConfigBuilder to create, chain With* methods, then call Build().
type AzureConfigBuilder struct {
	sessionConfig *AzureSessionConfig
	env           map[string]string
}

// NewAzureConfigBuilder creates a new builder for AzureConfig.
func NewAzureConfigBuilder() *AzureConfigBuilder {
	return &AzureConfigBuilder{
		env: make(map[string]string),
	}
}

// WithSessionConfig sets the Azure session configuration.
func (b *AzureConfigBuilder) WithSessionConfig(cfg *AzureSessionConfig) *AzureConfigBuilder {
	b.sessionConfig = cfg
	return b
}

// WithEnv sets environment variables used for credential and subscription resolution.
// When non-nil, values from env take precedence over os.Getenv lookups.
func (b *AzureConfigBuilder) WithEnv(env map[string]string) *AzureConfigBuilder {
	if env != nil {
		b.env = env
	}

	return b
}

// AuthMethod identifies how the resolved AzureConfig authenticates with Azure.
type AuthMethod string

const (
	AuthMethodSasToken         AuthMethod = "sas-token"
	AuthMethodAccessKey        AuthMethod = "access-key"
	AuthMethodServicePrincipal AuthMethod = "service-principal"
	AuthMethodOIDC             AuthMethod = "oidc"
	AuthMethodMSI              AuthMethod = "msi"
	AuthMethodAzureAD          AuthMethod = "azuread"
)

// AzureConfig holds the resolved credential and session metadata used to
// construct Azure SDK clients. Analogous to aws.Config returned by
// awshelper.NewAWSConfigBuilder().Build.
//
// Exactly one of Credential, SasToken, or AccessKey is populated, depending
// on Method.
type AzureConfig struct {
	// Credential is the Azure SDK token credential. nil for SAS-token and
	// access-key based methods.
	Credential azcore.TokenCredential
	// SasToken is the SAS token string. Non-empty only when Method == AuthMethodSasToken.
	SasToken string
	// AccessKey is the storage account access key. Non-empty only when Method == AuthMethodAccessKey.
	AccessKey string
	// SubscriptionID is the Azure subscription scope.
	SubscriptionID string
	// TenantID is the Azure AD tenant scope. May be empty for non-AD methods.
	TenantID string
	// AccountName is the storage account name.
	AccountName string
	// ResourceGroup is the resource group containing the storage account.
	ResourceGroup string
	// Location is the Azure region the storage account lives in (e.g.
	// "westeurope"). Populated from the session config or from the
	// ARM_LOCATION / AZURE_LOCATION environment variables.
	Location string
	// CloudConfig is the Azure cloud (public, government, china).
	CloudConfig cloud.Configuration
	// Method records which authentication method was selected by Build.
	Method AuthMethod
	// ClientOptions is preconfigured for the resolved cloud and is suitable
	// for passing to Azure SDK client constructors as the embedded
	// azcore.ClientOptions value.
	ClientOptions azcore.ClientOptions
}

// Build resolves credentials from the session config and environment and
// returns an AzureConfig. Resolution order (first match wins):
//
//  1. SAS token (data-plane only, no credential needed)
//  2. Storage account access key
//  3. Service principal (client_id + client_secret + tenant_id)
//  4. Managed Service Identity (UseMSI)
//  5. OIDC / workload identity federation (UseOIDC)
//  6. Azure AD via DefaultAzureCredential (UseAzureADAuth or default fallback)
//
// Environment variable fallbacks (ARM_* / AZURE_*) are applied to subscription,
// tenant, client id, client secret, SAS token, and access key when the session
// config leaves them empty.
func (b *AzureConfigBuilder) Build(_ context.Context, l log.Logger) (*AzureConfig, error) {
	cfg := b.sessionConfig
	if cfg == nil {
		cfg = &AzureSessionConfig{}
	}

	resolved := *cfg
	b.applyEnvFallbacks(&resolved)

	cloudCfg, err := cloudConfigForEnvironment(resolved.CloudEnvironment)
	if err != nil {
		return nil, err
	}

	clientOpts := azcore.ClientOptions{Cloud: cloudCfg}

	out := &AzureConfig{
		SubscriptionID: resolved.SubscriptionID,
		TenantID:       resolved.TenantID,
		AccountName:    resolved.StorageAccountName,
		ResourceGroup:  resolved.ResourceGroupName,
		Location:       resolved.Location,
		CloudConfig:    cloudCfg,
		ClientOptions:  clientOpts,
	}

	switch {
	case resolved.SasToken != "":
		out.Method = AuthMethodSasToken
		out.SasToken = resolved.SasToken

		l.Debugf("azurehelper: using SAS token authentication")

		return out, validate(out, &resolved)

	case resolved.AccessKey != "":
		out.Method = AuthMethodAccessKey
		out.AccessKey = resolved.AccessKey

		l.Debugf("azurehelper: using storage account access key authentication")

		return out, validate(out, &resolved)

	case resolved.ClientID != "" && resolved.ClientSecret != "" && resolved.TenantID != "":
		cred, err := azidentity.NewClientSecretCredential(
			resolved.TenantID, resolved.ClientID, resolved.ClientSecret,
			&azidentity.ClientSecretCredentialOptions{ClientOptions: clientOpts},
		)
		if err != nil {
			return nil, fmt.Errorf("creating service principal credential: %w", err)
		}

		out.Method = AuthMethodServicePrincipal
		out.Credential = cred

		l.Debugf("azurehelper: using service principal authentication")

		return out, validate(out, &resolved)

	case resolved.UseMSI:
		opts := &azidentity.ManagedIdentityCredentialOptions{ClientOptions: clientOpts}
		if resolved.MSIResourceID != "" {
			opts.ID = azidentity.ResourceID(resolved.MSIResourceID)
		}

		cred, err := azidentity.NewManagedIdentityCredential(opts)
		if err != nil {
			return nil, fmt.Errorf("creating managed identity credential: %w", err)
		}

		out.Method = AuthMethodMSI
		out.Credential = cred

		l.Debugf("azurehelper: using managed identity authentication")

		return out, validate(out, &resolved)

	case resolved.UseOIDC:
		cred, err := azidentity.NewWorkloadIdentityCredential(&azidentity.WorkloadIdentityCredentialOptions{
			ClientOptions: clientOpts,
			TenantID:      resolved.TenantID,
			ClientID:      resolved.ClientID,
			TokenFilePath: resolved.OIDCTokenFilePath,
		})
		if err != nil {
			return nil, fmt.Errorf("creating OIDC workload identity credential: %w", err)
		}

		out.Method = AuthMethodOIDC
		out.Credential = cred

		l.Debugf("azurehelper: using OIDC / workload identity authentication")

		return out, validate(out, &resolved)

	default:
		cred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{
			ClientOptions: clientOpts,
			TenantID:      resolved.TenantID,
		})
		if err != nil {
			return nil, fmt.Errorf("creating default Azure credential: %w", err)
		}

		out.Method = AuthMethodAzureAD
		out.Credential = cred

		l.Debugf("azurehelper: using Azure AD default credential chain")

		return out, validate(out, &resolved)
	}
}

// BuildBlobClient is a convenience that calls Build and then constructs a
// BlobClient from the resulting AzureConfig. Most callers only need a
// BlobClient and benefit from a single entry point. If the session config
// carries a non-empty ContainerName, it is bound on the returned client so
// callers can immediately use GetObject without an extra BindContainer call.
func (b *AzureConfigBuilder) BuildBlobClient(ctx context.Context, l log.Logger) (*BlobClient, error) {
	cfg, err := b.Build(ctx, l)
	if err != nil {
		return nil, err
	}

	client, err := NewBlobClient(cfg)
	if err != nil {
		return nil, err
	}

	if b.sessionConfig != nil && b.sessionConfig.ContainerName != "" {
		client.BindContainer(b.sessionConfig.ContainerName)
	}

	return client, nil
}

// BuildStorageAccountClient is a convenience that calls Build and then
// constructs a StorageAccountClient. Returns an error if the resolved
// config lacks the ARM-plane fields (subscription, resource group, account
// name, token credential) required for storage account management.
//
// SAS-token and access-key auth methods are rejected up-front because they
// cannot reach the ARM control plane; the rejection happens before Build
// emits any auth-resolution debug logs, keeping the failure mode obvious.
func (b *AzureConfigBuilder) BuildStorageAccountClient(ctx context.Context, l log.Logger) (*StorageAccountClient, error) {
	// Pre-flight against env-resolved values as well, not just the explicitly
	// supplied sessionConfig: ARM_SAS_TOKEN / ARM_ACCESS_KEY would otherwise
	// reach Build and fail with a less obvious error from the ARM client.
	preflight := AzureSessionConfig{}
	if b.sessionConfig != nil {
		preflight = *b.sessionConfig
	}

	b.applyEnvFallbacks(&preflight)

	switch {
	case preflight.SasToken != "":
		return nil, errors.New("storage account management requires a token credential, not a SAS token")
	case preflight.AccessKey != "":
		return nil, errors.New("storage account management requires a token credential, not an access key")
	}

	cfg, err := b.Build(ctx, l)
	if err != nil {
		return nil, err
	}

	return NewStorageAccountClient(cfg)
}

// applyEnvFallbacks fills empty fields on cfg from environment variables
// (the builder's env map first, then the process environment). Mirrors the
// ARM_* and AZURE_* names used by Terraform's azurerm backend.
func (b *AzureConfigBuilder) applyEnvFallbacks(cfg *AzureSessionConfig) {
	if cfg.SubscriptionID == "" {
		cfg.SubscriptionID = b.firstEnv("ARM_SUBSCRIPTION_ID", "AZURE_SUBSCRIPTION_ID")
	}

	if cfg.ResourceGroupName == "" {
		cfg.ResourceGroupName = b.firstEnv("ARM_RESOURCE_GROUP_NAME", "AZURE_RESOURCE_GROUP_NAME")
	}

	if cfg.StorageAccountName == "" {
		cfg.StorageAccountName = b.firstEnv("ARM_STORAGE_ACCOUNT_NAME", "AZURE_STORAGE_ACCOUNT")
	}

	if cfg.Location == "" {
		cfg.Location = b.firstEnv("ARM_LOCATION", "AZURE_LOCATION")
	}

	if cfg.TenantID == "" {
		cfg.TenantID = b.firstEnv("ARM_TENANT_ID", "AZURE_TENANT_ID")
	}

	if cfg.ClientID == "" {
		cfg.ClientID = b.firstEnv("ARM_CLIENT_ID", "AZURE_CLIENT_ID")
	}

	if cfg.ClientSecret == "" {
		cfg.ClientSecret = b.firstEnv("ARM_CLIENT_SECRET", "AZURE_CLIENT_SECRET")
	}

	if cfg.SasToken == "" {
		cfg.SasToken = b.firstEnv("ARM_SAS_TOKEN", "AZURE_STORAGE_SAS_TOKEN")
	}

	if cfg.AccessKey == "" {
		cfg.AccessKey = b.firstEnv("ARM_ACCESS_KEY", "AZURE_STORAGE_KEY")
	}

	if cfg.OIDCTokenFilePath == "" {
		cfg.OIDCTokenFilePath = b.firstEnv("ARM_OIDC_TOKEN_FILE_PATH", "AZURE_FEDERATED_TOKEN_FILE")
	}

	if cfg.CloudEnvironment == "" {
		cfg.CloudEnvironment = b.firstEnv("ARM_ENVIRONMENT", "AZURE_ENVIRONMENT")
	}

	if !cfg.UseMSI && parseBool(b.firstEnv("ARM_USE_MSI")) {
		cfg.UseMSI = true
	}

	if !cfg.UseOIDC && parseBool(b.firstEnv("ARM_USE_OIDC")) {
		cfg.UseOIDC = true
	}
}

// firstEnv returns the first non-empty value found by looking up keys in the
// builder's env map and falling back to os.Getenv. If a key is present in the
// builder's env map (even with an empty value), that map value is returned
// without consulting os.Getenv — this lets tests shield resolution from the
// developer's shell environment by passing an explicit empty value.
func (b *AzureConfigBuilder) firstEnv(keys ...string) string {
	for _, k := range keys {
		if v, ok := b.env[k]; ok {
			if v != "" {
				return v
			}

			continue
		}

		if v := os.Getenv(k); v != "" {
			return v
		}
	}

	return ""
}

// parseBool returns the boolean value of an env-var-style string using
// strconv.ParseBool semantics ("1", "t", "T", "TRUE", "true", "True", and
// their negations). Surrounding whitespace is tolerated. Any unrecognised
// value yields false, matching the convention used by the Terraform azurerm
// provider for ARM_USE_MSI / ARM_USE_OIDC.
func parseBool(s string) bool {
	v, _ := strconv.ParseBool(strings.TrimSpace(s))
	return v
}

// cloudConfigForEnvironment maps a cloud environment alias to an Azure SDK
// cloud.Configuration. An empty name resolves to cloud.AzurePublic. Any
// non-empty value that does not match a known alias is rejected so that a
// typo (e.g. "governmnt") does not silently route a Government or China
// deployment at the public Azure endpoints.
func cloudConfigForEnvironment(name string) (cloud.Configuration, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "public", "azurepublic", "azurepubliccloud":
		return cloud.AzurePublic, nil
	case "government", "usgovernment", "usgov", "azureusgovernment", "azureusgovernmentcloud":
		return cloud.AzureGovernment, nil
	case "china", "azurechina", "azurechinacloud":
		return cloud.AzureChina, nil
	default:
		return cloud.Configuration{}, &UnknownCloudEnvironmentError{Name: name}
	}
}

// validate returns an error if required fields are missing for the chosen method.
func validate(out *AzureConfig, cfg *AzureSessionConfig) error {
	// SAS token is data-plane only; subscription not required.
	if out.Method == AuthMethodSasToken {
		if cfg.StorageAccountName == "" {
			return errors.New("storage_account_name is required for SAS token authentication")
		}

		return nil
	}

	// Access key is data-plane only and is bound to a specific account; subscription not required.
	if out.Method == AuthMethodAccessKey {
		if cfg.StorageAccountName == "" {
			return errors.New("storage_account_name is required for access key authentication")
		}

		return nil
	}

	if out.SubscriptionID == "" {
		return errors.New("subscription_id is required (set via config, ARM_SUBSCRIPTION_ID, or AZURE_SUBSCRIPTION_ID)")
	}

	if out.Method == AuthMethodServicePrincipal {
		if cfg.TenantID == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
			return errors.New("service principal authentication requires tenant_id, client_id, and client_secret")
		}
	}

	return nil
}
