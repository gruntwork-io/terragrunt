package azurehelper

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// AzureSessionConfig holds the per-backend configuration needed to authenticate
// against Azure and identify the storage account being managed. Fields populated
// from a remote_state backend block, with environment variable fallbacks applied
// during Build.
type AzureSessionConfig struct {
	// SubscriptionID is the Azure subscription hosting the storage account.
	SubscriptionID string
	// TenantID is the Azure AD tenant used by token-based auth methods.
	TenantID string
	// ClientID is the service principal or workload identity client id.
	ClientID string
	// ClientSecret is the service principal secret.
	ClientSecret string
	// StorageAccountName is the storage account holding the state.
	StorageAccountName string
	// ResourceGroupName is the resource group containing the storage account.
	ResourceGroupName string
	// MSIResourceID selects a user-assigned managed identity for UseMSI.
	MSIResourceID string
	// SasToken authenticates data-plane calls without a token credential.
	SasToken string
	// AccessKey is the storage account shared key.
	AccessKey string
	// OIDCTokenFilePath points at a federated identity token file for UseOIDC.
	OIDCTokenFilePath string
	// CloudEnvironment selects the cloud: "" / "public", "usgovernment", "china".
	CloudEnvironment string
	// UseAzureADAuth selects the Azure AD default credential chain.
	UseAzureADAuth bool
	// UseMSI selects managed identity authentication.
	UseMSI bool
	// UseOIDC selects OIDC / workload identity authentication.
	UseOIDC bool
}

// AzureConfigBuilder builds an AzureConfig using the builder pattern.
// Use NewAzureConfigBuilder to create, chain With* methods, then call Build().
type AzureConfigBuilder struct {
	sessionConfig *AzureSessionConfig
	venv          venv.Venv
}

// NewAzureConfigBuilder creates a new builder for AzureConfig.
func NewAzureConfigBuilder() *AzureConfigBuilder {
	return &AzureConfigBuilder{
		sessionConfig: &AzureSessionConfig{},
		venv:          venv.Venv{}.WithEnv(nil),
	}
}

// WithSessionConfig sets the Azure session configuration; nil is ignored.
func (b *AzureConfigBuilder) WithSessionConfig(cfg *AzureSessionConfig) *AzureConfigBuilder {
	if cfg == nil {
		return b
	}

	b.sessionConfig = cfg

	return b
}

// WithVenv sets the virtualized environment whose Env map feeds ARM_* /
// AZURE_* fallback resolution; the builder never reads the process
// environment itself.
func (b *AzureConfigBuilder) WithVenv(v venv.Venv) *AzureConfigBuilder {
	b.venv = v.WithEnv(v.Env)
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
func (b *AzureConfigBuilder) Build(l log.Logger) (*AzureConfig, error) {
	resolved := *b.sessionConfig
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

	case resolved.UseAzureADAuth:
		return buildAzureADConfig(out, &resolved, &clientOpts, l, "use_azuread_auth")

	default:
		return buildAzureADConfig(out, &resolved, &clientOpts, l, "default credential chain")
	}
}

// buildAzureADConfig finishes an AzureConfig using the Azure AD credential
// chain. Shared by the explicit use_azuread_auth tier and the default fallback
// so the use_azuread_auth field is honored rather than silently dead.
//
// The chain tries an explicit AzureCLICredential first, then
// DefaultAzureCredential. As of azidentity v1.13, DefaultAzureCredential
// excludes the Azure CLI unless AZURE_TOKEN_CREDENTIALS opts in, so a plain
// `az login` would otherwise be ignored. AzureCLICredential reports
// "unavailable" when az is not installed or not logged in, which lets the chain
// fall through to DefaultAzureCredential (environment / workload-identity /
// managed-identity) in CI. The CLI must come first because
// DefaultAzureCredential can return a hard (non-"unavailable") error (e.g. an
// IMDS probe that gets an unexpected response) which would halt the chain
// before the CLI is reached. This matches OpenTofu azurerm's `use_cli` default.
func buildAzureADConfig(
	out *AzureConfig,
	resolved *AzureSessionConfig,
	clientOpts *azcore.ClientOptions,
	l log.Logger,
	reason string,
) (*AzureConfig, error) {
	defaultCred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{
		ClientOptions: *clientOpts,
		TenantID:      resolved.TenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("creating default Azure credential: %w", err)
	}

	out.Method = AuthMethodAzureAD
	out.Credential = chainedCredential(defaultCred, resolved.TenantID, l)

	l.Debugf("azurehelper: using Azure AD authentication (%s)", reason)

	return out, validate(out, resolved)
}

// chainedCredential prepends an Azure CLI credential to defaultCred when one
// can be constructed; on any construction error it logs and keeps defaultCred
// alone, so an operator expecting `az login` precedence is not left guessing.
func chainedCredential(defaultCred azcore.TokenCredential, tenantID string, l log.Logger) azcore.TokenCredential {
	cliCred, err := azidentity.NewAzureCLICredential(&azidentity.AzureCLICredentialOptions{
		TenantID: tenantID,
	})
	if err != nil {
		l.Debugf("azurehelper: Azure CLI credential unavailable, using DefaultAzureCredential only: %v", err)

		return defaultCred
	}

	chain, err := azidentity.NewChainedTokenCredential([]azcore.TokenCredential{cliCred, defaultCred}, nil)
	if err != nil {
		l.Debugf("azurehelper: failed to build CLI credential chain, using DefaultAzureCredential only: %v", err)

		return defaultCred
	}

	return chain
}

// BuildBlobClient is a convenience that calls Build and then constructs a
// BlobClient from the resulting AzureConfig.
func (b *AzureConfigBuilder) BuildBlobClient(l log.Logger) (*BlobClient, error) {
	cfg, err := b.Build(l)
	if err != nil {
		return nil, err
	}

	return NewBlobClient(cfg)
}

// BuildStorageAccountClient is a convenience that calls Build and then
// constructs a StorageAccountClient. Returns an error if the resolved
// config lacks the ARM-plane fields (subscription, resource group, account
// name, token credential) required for storage account management.
//
// SAS-token and access-key auth methods are rejected up-front because they
// cannot reach the ARM control plane; the rejection happens before Build
// emits any auth-resolution debug logs, keeping the failure mode obvious.
func (b *AzureConfigBuilder) BuildStorageAccountClient(l log.Logger) (*StorageAccountClient, error) {
	// Pre-flight against env-resolved values as well, not just the explicitly
	// supplied sessionConfig: ARM_SAS_TOKEN / ARM_ACCESS_KEY would otherwise
	// reach Build and fail with a less obvious error from the ARM client.
	preflight := *b.sessionConfig
	b.applyEnvFallbacks(&preflight)

	switch {
	case preflight.SasToken != "":
		return nil, &UnsupportedAuthForOpError{Method: AuthMethodSasToken, Operation: "storage account operations"}
	case preflight.AccessKey != "":
		return nil, &UnsupportedAuthForOpError{Method: AuthMethodAccessKey, Operation: "storage account operations"}
	}

	cfg, err := b.Build(l)
	if err != nil {
		return nil, err
	}

	return NewStorageAccountClient(cfg)
}

// applyEnvFallbacks fills empty fields on cfg from the builder's env map.
// Mirrors the ARM_* and AZURE_* names used by the OpenTofu azurerm backend.
func (b *AzureConfigBuilder) applyEnvFallbacks(cfg *AzureSessionConfig) {
	fallbacks := []struct {
		field *string
		keys  []string
	}{
		{&cfg.SubscriptionID, []string{"ARM_SUBSCRIPTION_ID", "AZURE_SUBSCRIPTION_ID"}},
		{&cfg.ResourceGroupName, []string{"ARM_RESOURCE_GROUP_NAME", "AZURE_RESOURCE_GROUP_NAME"}},
		{&cfg.StorageAccountName, []string{"ARM_STORAGE_ACCOUNT_NAME", "AZURE_STORAGE_ACCOUNT"}},
		{&cfg.TenantID, []string{"ARM_TENANT_ID", "AZURE_TENANT_ID"}},
		{&cfg.ClientID, []string{"ARM_CLIENT_ID", "AZURE_CLIENT_ID"}},
		{&cfg.ClientSecret, []string{"ARM_CLIENT_SECRET", "AZURE_CLIENT_SECRET"}},
		{&cfg.SasToken, []string{"ARM_SAS_TOKEN", "AZURE_STORAGE_SAS_TOKEN"}},
		{&cfg.AccessKey, []string{"ARM_ACCESS_KEY", "AZURE_STORAGE_KEY"}},
		{&cfg.MSIResourceID, []string{"ARM_MSI_RESOURCE_ID", "AZURE_MSI_RESOURCE_ID"}},
		{&cfg.OIDCTokenFilePath, []string{"ARM_OIDC_TOKEN_FILE_PATH", "AZURE_FEDERATED_TOKEN_FILE"}},
		{&cfg.CloudEnvironment, []string{"ARM_ENVIRONMENT", "AZURE_ENVIRONMENT"}},
	}

	// Trim first so a stray space or newline in a config value (common when CI
	// injects secrets via naive shell redirection) does not suppress an
	// otherwise-valid env fallback, then trim again after resolution so an
	// env-sourced value with trailing whitespace does not fail opaquely.
	for _, fb := range fallbacks {
		*fb.field = strings.TrimSpace(*fb.field)

		if *fb.field == "" {
			*fb.field = strings.TrimSpace(b.firstEnv(fb.keys...))
		}
	}

	if !cfg.UseMSI && parseBool(b.firstEnv("ARM_USE_MSI")) {
		cfg.UseMSI = true
	}

	if !cfg.UseOIDC && parseBool(b.firstEnv("ARM_USE_OIDC")) {
		cfg.UseOIDC = true
	}

	if !cfg.UseAzureADAuth && parseBool(b.firstEnv("ARM_USE_AZUREAD", "ARM_USE_AZUREAD_AUTH")) {
		cfg.UseAzureADAuth = true
	}

	// Presence of a federated token file implies OIDC / workload-identity auth.
	// The standard AKS workload-identity webhook injects AZURE_FEDERATED_TOKEN_FILE
	// without ARM_USE_OIDC, so without this the explicit OIDC tier is unreachable.
	// Defer to an explicit higher-tier choice (MSI / Azure AD) so a stray token
	// file in the environment does not override what the user asked for.
	if !cfg.UseOIDC && !cfg.UseMSI && !cfg.UseAzureADAuth && cfg.OIDCTokenFilePath != "" {
		cfg.UseOIDC = true
	}
}

// firstEnv returns the first non-empty value found for keys in the builder's
// venv environment; the process environment is never consulted, so resolution
// stays hermetic and fully caller-controlled.
func (b *AzureConfigBuilder) firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := b.venv.Env[k]; v != "" {
			return v
		}
	}

	return ""
}

// parseBool returns the boolean value of an env-var-style string using
// strconv.ParseBool semantics ("1", "t", "T", "TRUE", "true", "True", and
// their negations). Surrounding whitespace is tolerated. Any unrecognised
// value yields false, matching the convention used by the OpenTofu azurerm
// backend for ARM_USE_MSI / ARM_USE_OIDC.
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
	// SAS token and access key are data-plane only and bound to a specific
	// account; subscription not required.
	if out.Method == AuthMethodSasToken || out.Method == AuthMethodAccessKey {
		if cfg.StorageAccountName == "" {
			return fmt.Errorf("%w for %s authentication", ErrStorageAccountRequired, out.Method)
		}

		return nil
	}

	if out.SubscriptionID == "" {
		return fmt.Errorf("%w (set via config, ARM_SUBSCRIPTION_ID, or AZURE_SUBSCRIPTION_ID)", ErrSubscriptionIDRequired)
	}

	if out.Method == AuthMethodServicePrincipal {
		if cfg.TenantID == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
			return errors.New("service principal authentication requires tenant_id, client_id, and client_secret")
		}
	}

	return nil
}
