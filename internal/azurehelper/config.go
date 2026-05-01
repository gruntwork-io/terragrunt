// Package azurehelper provides helpers for interacting with Azure services
// (storage accounts, blob containers, RBAC). It mirrors the layout of
// internal/awshelper and internal/gcphelper: a flat package, no interfaces
// or factory abstractions, builder pattern for credential/client construction.
package azurehelper

import (
	"context"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"

	"github.com/gruntwork-io/terragrunt/internal/errors"
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
	AuthMethodDefault          AuthMethod = "default"
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
//  4. OIDC / workload identity federation (UseOIDC)
//  5. Managed Service Identity (UseMSI)
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

	cloudCfg := cloudConfigForEnvironment(resolved.CloudEnvironment)
	clientOpts := azcore.ClientOptions{Cloud: cloudCfg}

	out := &AzureConfig{
		SubscriptionID: resolved.SubscriptionID,
		TenantID:       resolved.TenantID,
		AccountName:    resolved.StorageAccountName,
		ResourceGroup:  resolved.ResourceGroupName,
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
			return nil, errors.Errorf("creating service principal credential: %w", err)
		}

		out.Method = AuthMethodServicePrincipal
		out.Credential = cred

		l.Debugf("azurehelper: using service principal authentication")

		return out, validate(out, &resolved)

	case resolved.UseOIDC:
		cred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{
			ClientOptions: clientOpts,
		})
		if err != nil {
			return nil, errors.Errorf("creating OIDC credential: %w", err)
		}

		out.Method = AuthMethodOIDC
		out.Credential = cred

		l.Debugf("azurehelper: using OIDC / workload identity authentication")

		return out, validate(out, &resolved)

	case resolved.UseMSI:
		opts := &azidentity.ManagedIdentityCredentialOptions{ClientOptions: clientOpts}
		if resolved.MSIResourceID != "" {
			opts.ID = azidentity.ResourceID(resolved.MSIResourceID)
		}

		cred, err := azidentity.NewManagedIdentityCredential(opts)
		if err != nil {
			return nil, errors.Errorf("creating managed identity credential: %w", err)
		}

		out.Method = AuthMethodMSI
		out.Credential = cred

		l.Debugf("azurehelper: using managed identity authentication")

		return out, validate(out, &resolved)

	default:
		cred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{
			ClientOptions: clientOpts,
			TenantID:      resolved.TenantID,
		})
		if err != nil {
			return nil, errors.Errorf("creating default Azure credential: %w", err)
		}

		out.Method = AuthMethodAzureAD
		out.Credential = cred

		l.Debugf("azurehelper: using Azure AD default credential chain")

		return out, validate(out, &resolved)
	}
}

// applyEnvFallbacks fills empty fields on cfg from environment variables
// (the builder's env map first, then the process environment). Mirrors the
// ARM_* and AZURE_* names used by Terraform's azurerm backend.
func (b *AzureConfigBuilder) applyEnvFallbacks(cfg *AzureSessionConfig) {
	if cfg.SubscriptionID == "" {
		cfg.SubscriptionID = b.firstEnv("ARM_SUBSCRIPTION_ID", "AZURE_SUBSCRIPTION_ID")
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

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes":
		return true
	}

	return false
}

// cloudConfigForEnvironment maps a cloud environment alias to an Azure SDK
// cloud.Configuration.
func cloudConfigForEnvironment(name string) cloud.Configuration {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "government", "usgovernment", "usgov", "azureusgovernment", "azureusgovernmentcloud":
		return cloud.AzureGovernment
	case "china", "azurechina", "azurechinacloud":
		return cloud.AzureChina
	default:
		return cloud.AzurePublic
	}
}

// validate returns an error if required fields are missing for the chosen method.
func validate(out *AzureConfig, cfg *AzureSessionConfig) error {
	// SAS token is data-plane only; subscription not required.
	if out.Method == AuthMethodSasToken {
		if cfg.StorageAccountName == "" {
			return errors.Errorf("storage_account_name is required for SAS token authentication")
		}

		return nil
	}

	if out.SubscriptionID == "" {
		return errors.Errorf("subscription_id is required (set via config, ARM_SUBSCRIPTION_ID, or AZURE_SUBSCRIPTION_ID)")
	}

	if out.Method == AuthMethodServicePrincipal {
		if cfg.TenantID == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
			return errors.Errorf("service principal authentication requires tenant_id, client_id, and client_secret")
		}
	}

	return nil
}
