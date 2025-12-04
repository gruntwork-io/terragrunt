// Package azureauth provides centralized authentication logic for Azure services.
// This package handles all Azure authentication methods (Azure AD, MSI, Service Principal)
// with a consistent interface and supports different Azure services (storage, resource management, RBAC).
package azureauth

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// AuthMethod represents the Azure authentication method to use.
// The authentication method determines how Terragrunt will authenticate with Azure services.
type AuthMethod string

const (
	// AuthMethodAzureAD uses Azure Active Directory authentication.
	// This is the recommended authentication method for most scenarios.
	AuthMethodAzureAD AuthMethod = "azuread"

	// AuthMethodMSI uses Managed Service Identity authentication.
	// This is ideal for services running in Azure that have managed identities assigned.
	AuthMethodMSI AuthMethod = "msi"

	// AuthMethodServicePrincipal uses Service Principal authentication.
	// Requires client_id, client_secret, and tenant_id to be provided.
	AuthMethodServicePrincipal AuthMethod = "service-principal"

	// AuthMethodEnvironment uses environment variables for authentication.
	// Looks for standard Azure environment variables like AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, etc.
	AuthMethodEnvironment AuthMethod = "environment"

	// AuthMethodCLI uses Azure CLI for authentication.
	// Uses the currently logged in CLI session for authentication.
	AuthMethodCLI AuthMethod = "cli"

	// AuthMethodSasToken uses SAS token authentication.
	// For storage-specific operations that support SAS tokens.
	AuthMethodSasToken AuthMethod = "sas-token"
)

// AuthConfig represents the configuration for Azure authentication.
// This struct contains all possible authentication parameters for connecting to Azure services.
// Only a subset of fields will be used depending on the selected authentication method.
// The configuration supports multiple authentication methods with automatic fallback and
// environment variable integration.
// Authentication Methods:
//   - Azure AD (Default): Uses Azure Active Directory with automatic credential discovery
//   - Service Principal: Uses explicit client credentials (client_id, client_secret, tenant_id)
//   - Managed Service Identity (MSI): Uses Azure MSI for resources running in Azure
//   - CLI: Uses the currently logged-in Azure CLI session
//   - SAS Token: Uses Storage Account SAS tokens for storage-specific operations
//
// Required Fields by Method:
//
//	Azure AD: SubscriptionID (others auto-discovered)
//	Service Principal: ClientID, ClientSecret, TenantID, SubscriptionID
//	MSI: SubscriptionID (UseMSI=true)
//	CLI: SubscriptionID (uses current CLI session)
//	SAS Token: StorageAccountName, SasToken, SubscriptionID
//
// Environment Variable Support:
//
//	The following environment variables are automatically detected:
//	- AZURE_SUBSCRIPTION_ID, ARM_SUBSCRIPTION_ID: Sets SubscriptionID
//	- AZURE_CLIENT_ID, ARM_CLIENT_ID: Sets ClientID
//	- AZURE_CLIENT_SECRET, ARM_CLIENT_SECRET: Sets ClientSecret
//	- AZURE_TENANT_ID, ARM_TENANT_ID: Sets TenantID
//	- AZURE_ENVIRONMENT, ARM_ENVIRONMENT: Sets CloudEnvironment
//	- MSI_ENDPOINT: Sets MSIEndpoint
//
// Cloud Environment Support:
//   - "public" or "AzurePublicCloud": Azure Public Cloud (default)
//   - "government" or "AzureUSGovernmentCloud": Azure US Government Cloud
//   - "china" or "AzureChinaCloud": Azure China Cloud
//   - "german" or "AzureGermanCloud": Azure German Cloud (deprecated)
//
// Examples:
//
//	// Azure AD Authentication (Recommended)
//	config := AuthConfig{
//	    Method:         AuthMethodAzureAD,
//	    SubscriptionID: "12345678-1234-1234-1234-123456789abc",
//	    UseAzureAD:     true,
//	}
//
//	// Service Principal Authentication
//	config := AuthConfig{
//	    Method:         AuthMethodServicePrincipal,
//	    ClientID:       "app-client-id",
//	    ClientSecret:   "app-client-secret",
//	    TenantID:       "tenant-id",
//	    SubscriptionID: "12345678-1234-1234-1234-123456789abc",
//	}
//
//	// Managed Service Identity (for Azure VMs/App Service)
//	config := AuthConfig{
//	    Method:         AuthMethodMSI,
//	    SubscriptionID: "12345678-1234-1234-1234-123456789abc",
//	    UseMSI:         true,
//	}
//
//	// Custom MSI Configuration
//	config := AuthConfig{
//	    Method:         AuthMethodMSI,
//	    SubscriptionID: "12345678-1234-1234-1234-123456789abc",
//	    UseMSI:         true,
//	    MSIEndpoint:    "http://custom-msi-endpoint:50342/oauth2/token",
//	    MSIResourceID:  "/subscriptions/sub-id/resourcegroups/rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/my-identity",
//	}
//
//	// Azure CLI Authentication
//	config := AuthConfig{
//	    Method:         AuthMethodCLI,
//	    SubscriptionID: "12345678-1234-1234-1234-123456789abc",
//	}
//
//	// SAS Token Authentication (Storage-only)
//	config := AuthConfig{
//	    Method:             AuthMethodSasToken,
//	    StorageAccountName: "mystorageaccount",
//	    SasToken:           "?sv=2021-06-08&ss=b&srt=sco&sp=rwdlacx&se=...",
//	    SubscriptionID:     "12345678-1234-1234-1234-123456789abc",
//	}
//
//	// Environment-based Configuration
//	config := AuthConfig{
//	    Method:         AuthMethodServicePrincipal,
//	    UseEnvironment: true, // Loads from environment variables
//	}
//
//	// Azure Government Cloud
//	config := AuthConfig{
//	    Method:           AuthMethodAzureAD,
//	    SubscriptionID:   "12345678-1234-1234-1234-123456789abc",
//	    CloudEnvironment: "government",
//	}
type AuthConfig struct {
	// Method specifies the authentication method to use.
	// Determines which other fields in this struct are required.
	// Valid values: AuthMethodAzureAD (default), AuthMethodServicePrincipal,
	// AuthMethodMSI, AuthMethodCLI, AuthMethodSasToken.
	// Default: AuthMethodAzureAD
	Method AuthMethod

	// ClientID specifies the Application (client) ID for Service Principal authentication.
	// Also called "Application ID" in Azure portal.
	// Required for: AuthMethodServicePrincipal
	// Environment variables: AZURE_CLIENT_ID, ARM_CLIENT_ID
	// Format: UUID (e.g., "12345678-1234-1234-1234-123456789abc")
	// Optional for other authentication methods.
	ClientID string

	// ClientSecret specifies the client secret for Service Principal authentication.
	// This is a sensitive value that should be stored securely.
	// Required for: AuthMethodServicePrincipal
	// Environment variables: AZURE_CLIENT_SECRET, ARM_CLIENT_SECRET
	// Should be treated as a password and never logged or exposed.
	// Optional for other authentication methods.
	ClientSecret string

	// TenantID specifies the Azure Active Directory tenant ID.
	// Also called "Directory ID" in Azure portal.
	// Required for: AuthMethodServicePrincipal
	// Environment variables: AZURE_TENANT_ID, ARM_TENANT_ID
	// Format: UUID (e.g., "12345678-1234-1234-1234-123456789abc")
	// Optional for other authentication methods (auto-discovered for Azure AD).
	TenantID string

	// SubscriptionID specifies the Azure subscription ID.
	// Required for all authentication methods.
	// Environment variables: AZURE_SUBSCRIPTION_ID, ARM_SUBSCRIPTION_ID
	// Format: UUID (e.g., "12345678-1234-1234-1234-123456789abc")
	// Determines which subscription resources are created in.
	SubscriptionID string

	// StorageAccountName specifies the Azure Storage Account name for storage operations.
	// Required for: AuthMethodSasToken
	// Must be 3-24 characters, lowercase letters and numbers only.
	// Used in combination with SasToken for storage-specific authentication.
	// Optional for other authentication methods.
	StorageAccountName string

	// SasToken specifies the Shared Access Signature token for storage operations.
	// Required for: AuthMethodSasToken
	// Must start with "?" and contain valid SAS token parameters.
	// Example: "?sv=2021-06-08&ss=b&srt=sco&sp=rwdlacx&se=2023-12-31T23:59:59Z&sig=..."
	// This is a sensitive value that should be stored securely.
	// Time-limited and scope-limited access to storage resources.
	// Optional for other authentication methods.
	SasToken string

	// MSIEndpoint specifies a custom MSI endpoint URL.
	// Optional for: AuthMethodMSI
	// When empty, uses the default Azure Instance Metadata Service endpoint.
	// Format: Full URL (e.g., "http://custom-endpoint:50342/oauth2/token")
	// Used for custom MSI configurations or testing scenarios.
	MSIEndpoint string

	// MSIResourceID specifies the resource ID of a user-assigned managed identity.
	// Optional for: AuthMethodMSI
	// When empty, uses the system-assigned managed identity.
	// Format: Azure resource ID (e.g., "/subscriptions/.../providers/Microsoft.ManagedIdentity/userAssignedIdentities/my-identity")
	// Required when using user-assigned managed identities.
	MSIResourceID string

	// CloudEnvironment specifies the Azure cloud environment to use.
	// Valid values:
	// - "public" or "AzurePublicCloud": Azure Public Cloud (default)
	// - "government" or "AzureUSGovernmentCloud": Azure US Government Cloud
	// - "china" or "AzureChinaCloud": Azure China Cloud
	// - "german" or "AzureGermanCloud": Azure German Cloud (deprecated)
	// Environment variables: AZURE_ENVIRONMENT, ARM_ENVIRONMENT
	// Default: "public" (Azure Public Cloud)
	CloudEnvironment string

	// UseMSI indicates whether to use Managed Service Identity authentication.
	// When true, the code will attempt to authenticate using Azure MSI.
	// Required for: AuthMethodMSI (set to true)
	// Only works when running on Azure resources (VMs, App Service, Function Apps, etc.)
	// Automatically uses the system-assigned or user-assigned managed identity.
	// Default: false
	UseMSI bool

	// UseAzureAD indicates whether to use Azure Active Directory authentication.
	// When true, uses Azure AD with automatic credential discovery.
	// Default: true (Azure AD is now the default authentication method)
	// Set to false only when using non-Azure AD authentication methods.
	UseAzureAD bool

	// UseEnvironment indicates whether the configuration was loaded from environment variables.
	// This is informational and helps with debugging authentication issues.
	// When true, indicates that configuration came from environment variables.
	// Automatically set by the authentication system during configuration loading.
	// Default: false
	UseEnvironment bool
}

// AuthResult contains the credential and any associated metadata.
// This struct is returned by authentication operations and provides
// access to the appropriate credentials based on the authentication method used.
// It encapsulates all the information needed to authenticate with Azure services.
//
// The AuthResult is designed to be used with various Azure SDK clients and
// provides a unified interface regardless of the underlying authentication method.
//
// Usage examples:
//
//	// Using the credential with Azure Storage SDK
//	authResult, err := azureauth.GetAuthConfig(ctx, logger, config)
//	if err != nil {
//	    return fmt.Errorf("failed to get auth config: %w", err)
//	}
//
//	// Create a storage client using the credential
//	storageClient, err := azstorage.NewClient(storageURL, authResult.Credential, nil)
//	if err != nil {
//	    return fmt.Errorf("failed to create storage client: %w", err)
//	}
//
//	// Using with Resource Manager SDK
//	resourcesClient, err := armresources.NewClient(authResult.SubscriptionID, authResult.Credential, nil)
//	if err != nil {
//	    return fmt.Errorf("failed to create resources client: %w", err)
//	}
//
//	// Checking authentication method
//	if authResult.Method == AuthMethodSasToken {
//	    // Handle SAS token specific logic
//	    sasToken := authResult.SasToken
//	    // Use SAS token for direct API calls
//	}
type AuthResult struct {
	// Credential is the Azure token credential that can be used with Azure SDK clients.
	// This credential implements the azcore.TokenCredential interface and can be used
	// with any Azure SDK client that requires authentication.
	// The credential handles token acquisition, refresh, and expiration automatically.
	// This field is populated for all authentication methods except SAS token.
	Credential azcore.TokenCredential

	// SasToken is populated only when using SAS token authentication.
	// Contains the raw SAS token string that can be used for direct Azure Storage API calls.
	// The token includes the "?" prefix and all required parameters.
	// Example: "?sv=2021-06-08&ss=b&srt=sco&sp=rwdlacx&se=2023-12-31T23:59:59Z&sig=..."
	// This field is empty for all other authentication methods.
	SasToken string

	// Config is the configuration used to create this credential.
	// Contains the original AuthConfig that was processed to create the credential.
	// Useful for debugging, logging, or understanding which authentication method was used.
	// This is a copy of the original configuration and can be safely accessed.
	Config *AuthConfig

	// Method indicates which authentication method was used to create the credential.
	// Possible values: AuthMethodAzureAD, AuthMethodServicePrincipal, AuthMethodMSI,
	// AuthMethodCLI, AuthMethodSasToken.
	// This information is useful for logging, debugging, or implementing method-specific logic.
	Method AuthMethod

	// SubscriptionID is the Azure subscription ID associated with the authentication.
	// This is extracted from the configuration or discovered during authentication.
	// Used when creating Azure SDK clients that require a subscription ID.
	// Format: UUID (e.g., "12345678-1234-1234-1234-123456789abc")
	SubscriptionID string
}

// GetAuthConfig constructs an AuthConfig from options and config.
// The config map is typically derived from the backend configuration in Terragrunt.
// This function follows this precedence order:
// 1. Explicit configuration in the provided config map
// 2. Environment variables (AZURE_* or ARM_* prefixes)
// 3. Default to Azure AD authentication if no other method is specified
//
// Supported config keys include:
// - subscription_id: Azure Subscription ID
// - client_id: Service Principal Client ID
// - client_secret: Service Principal Client Secret
// - tenant_id: Azure AD Tenant ID
// - use_azuread_auth: Whether to use Azure AD authentication (bool)
// - use_msi: Whether to use Managed Service Identity authentication (bool)
// - storage_account_name: Azure Storage Account Name
// - sas_token: SAS Token for storage operations
//
// Context is used for potential future authentication operations that may require it.
func GetAuthConfig(
	ctx context.Context,
	l log.Logger,
	config map[string]interface{},
) (*AuthConfig, error) {
	authConfig := &AuthConfig{
		// Extract values from config with type assertion
		SubscriptionID: getStringValue(config, "subscription_id"),
		ClientID:       getStringValue(config, "client_id"),
		ClientSecret:   getStringValue(config, "client_secret"),
		TenantID:       getStringValue(config, "tenant_id"),
		SasToken:       getStringValue(config, "sas_token"),

		// Extract boolean values
		UseAzureAD: getBoolValue(config, "use_azuread_auth", false),
		UseMSI:     getBoolValue(config, "use_msi", false),

		// Extract storage account name if present
		StorageAccountName: getStringValue(config, "storage_account_name"),
	}

	// Try to detect auth method based on configuration
	switch {
	case authConfig.UseAzureAD:
		authConfig.Method = AuthMethodAzureAD

		l.Debugf("Using Azure AD authentication based on configuration")
	case authConfig.UseMSI:
		authConfig.Method = AuthMethodMSI

		l.Debugf("Using MSI authentication based on configuration")
	case authConfig.SasToken != "":
		authConfig.Method = AuthMethodSasToken

		l.Debugf("Using SAS token authentication based on configuration")
	case authConfig.ClientID != "" && authConfig.ClientSecret != "" && authConfig.TenantID != "":
		authConfig.Method = AuthMethodServicePrincipal

		l.Debugf("Using Service Principal authentication based on configuration")
	default:
		// Check environment variables if no explicit credentials in config
		envClientID := getFirstEnvValue("AZURE_CLIENT_ID", "ARM_CLIENT_ID", "")
		envClientSecret := getFirstEnvValue("AZURE_CLIENT_SECRET", "ARM_CLIENT_SECRET", "")
		envTenantID := getFirstEnvValue("AZURE_TENANT_ID", "ARM_TENANT_ID", "")
		envSubID := getFirstEnvValue("AZURE_SUBSCRIPTION_ID", "ARM_SUBSCRIPTION_ID", "")
		envSas := getFirstEnvValue("AZURE_STORAGE_SAS_TOKEN", "ARM_SAS_TOKEN", "")

		switch {
		case envClientID != "" && envClientSecret != "" && envTenantID != "":
			authConfig.Method = AuthMethodServicePrincipal
			authConfig.ClientID = envClientID
			authConfig.ClientSecret = envClientSecret
			authConfig.TenantID = envTenantID
			authConfig.UseEnvironment = true

			l.Debugf("Using Service Principal authentication from environment variables")

			if envSubID != "" {
				authConfig.SubscriptionID = envSubID
			}
		case envSas != "":
			authConfig.Method = AuthMethodSasToken
			authConfig.SasToken = envSas
			authConfig.UseEnvironment = true

			l.Debugf("Using SAS token authentication from environment variables")
		default:
			// Default to Azure AD authentication if nothing else specified
			authConfig.Method = AuthMethodAzureAD
			authConfig.UseAzureAD = true

			l.Debugf("No explicit authentication method configured, defaulting to Azure AD authentication")
		}
	}

	// If no subscription ID was provided in config, try to get it from environment
	if authConfig.SubscriptionID == "" {
		authConfig.SubscriptionID = getFirstEnvValue("AZURE_SUBSCRIPTION_ID", "ARM_SUBSCRIPTION_ID", "")
	}

	return authConfig, nil
}

// GetTokenCredential creates an Azure token credential based on the provided config.
// This function handles all supported authentication methods and returns an AuthResult
// containing the appropriate credential based on the configured authentication method.
//
// For token-based authentication methods (Azure AD, MSI, Service Principal), it returns
// an azcore.TokenCredential that can be used with Azure SDK clients.
//
// For SAS token authentication, the returned AuthResult will have a nil Credential
// but will include the SAS token.
//
// The context is used for authentication operations that may have timeouts or cancellation.
func GetTokenCredential(
	ctx context.Context,
	l log.Logger,
	config *AuthConfig,
) (*AuthResult, error) {
	var (
		credential azcore.TokenCredential
		err        error
	)

	switch config.Method {
	case AuthMethodAzureAD:
		l.Debugf("Creating Azure AD credential")

		credential, err = azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{})

	case AuthMethodMSI:
		l.Debugf("Creating MSI credential")

		opts := &azidentity.ManagedIdentityCredentialOptions{}

		if config.MSIResourceID != "" {
			opts.ID = azidentity.ResourceID(config.MSIResourceID)
		}

		credential, err = azidentity.NewManagedIdentityCredential(opts)

	case AuthMethodServicePrincipal:
		l.Debugf("Creating Service Principal credential")

		credential, err = azidentity.NewClientSecretCredential(
			config.TenantID,
			config.ClientID,
			config.ClientSecret,
			&azidentity.ClientSecretCredentialOptions{},
		)

	case AuthMethodEnvironment:
		l.Debugf("Creating credential from environment variables")

		credential, err = azidentity.NewEnvironmentCredential(nil)

	case AuthMethodCLI:
		l.Debugf("Creating Azure CLI credential")

		credential, err = azidentity.NewAzureCLICredential(nil)

	case AuthMethodSasToken:
		// For SAS token, we return no credential but include the token in the result
		l.Debugf("Using SAS token authentication")

		return &AuthResult{
			Credential:     nil,
			SasToken:       config.SasToken,
			Config:         config,
			Method:         config.Method,
			SubscriptionID: config.SubscriptionID,
		}, nil

	default:
		return nil, errors.Errorf("unknown authentication method: %s", config.Method)
	}

	if err != nil {
		return nil, errors.Errorf("failed to create token credential: %w", err)
	}

	return &AuthResult{
		Credential:     credential,
		SasToken:       config.SasToken, // May be empty for most auth methods
		Config:         config,
		Method:         config.Method,
		SubscriptionID: config.SubscriptionID,
	}, nil
}

// ValidateAuthConfig validates the auth config and returns any errors
func ValidateAuthConfig(config *AuthConfig) error {
	var errs []error

	switch config.Method {
	case AuthMethodServicePrincipal:
		if config.TenantID == "" {
			errs = append(errs, errors.Errorf("tenant_id is required for service principal authentication"))
		}

		if config.ClientID == "" {
			errs = append(errs, errors.Errorf("client_id is required for service principal authentication"))
		}

		if config.ClientSecret == "" {
			errs = append(errs, errors.Errorf("client_secret is required for service principal authentication"))
		}
	case AuthMethodSasToken:
		if config.SasToken == "" {
			errs = append(errs, errors.Errorf("sas_token is required for SAS token authentication"))
		}

		if config.StorageAccountName == "" {
			errs = append(errs, errors.Errorf("storage_account_name is required for SAS token authentication"))
		}
	// No additional validation required for these authentication methods.
	case AuthMethodAzureAD, AuthMethodMSI, AuthMethodEnvironment, AuthMethodCLI:
	}

	if len(errs) > 0 {
		// Combine all errors into a single error message
		var builder strings.Builder

		builder.WriteString("Azure authentication config validation failed:")

		for _, err := range errs {
			builder.WriteString("\n- ")
			builder.WriteString(err.Error())
		}

		return errors.Errorf("%s", builder.String())
	}

	return nil
}

// CreateStorageClientOptions creates Azure Storage client options from the auth result
func (r *AuthResult) CreateStorageClientOptions() *azcore.ClientOptions {
	return &azcore.ClientOptions{}
}

// GetEndpointSuffix returns the Azure storage endpoint suffix based on the cloud environment
func GetEndpointSuffix(cloudEnv string) string {
	switch strings.ToLower(cloudEnv) {
	case "usgovernment", "usgov":
		return "core.usgovcloudapi.net"
	case "china":
		return "core.chinacloudapi.cn"
	case "german", "germany":
		return "core.cloudapi.de"
	default:
		return "core.windows.net" // Default to public cloud
	}
}

// GetArmEndpoint returns the Azure Resource Manager endpoint based on the cloud environment
func GetArmEndpoint(cloudEnv string) string {
	switch strings.ToLower(cloudEnv) {
	case "usgovernment", "usgov":
		return "https://management.usgovcloudapi.net"
	case "china":
		return "https://management.chinacloudapi.cn"
	case "german", "germany":
		return "https://management.microsoftazure.de"
	default:
		return "https://management.azure.com" // Default to public cloud
	}
}

// Helper function to get string value from map with default empty string
func getStringValue(config map[string]interface{}, key string) string {
	if val, ok := config[key].(string); ok {
		return val
	}

	return ""
}

// Helper function to get bool value from map with default
func getBoolValue(config map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := config[key].(bool); ok {
		return val
	}

	return defaultValue
}

// Helper function to get the first non-empty value from environment variables
func getFirstEnvValue(keys ...string) string {
	for _, key := range keys[:len(keys)-1] { // Last element is default value
		if val := os.Getenv(key); val != "" {
			return val
		}
	}

	return keys[len(keys)-1] // Default value
}

// GetAzureStorageURL generates the storage account URL based on the configuration
func GetAzureStorageURL(storageAccountName string, endpointSuffix string) string {
	if endpointSuffix == "" {
		endpointSuffix = "core.windows.net"
	}

	return fmt.Sprintf("https://%s.blob.%s", storageAccountName, endpointSuffix)
}

// IsAzureError checks if an error is an Azure API error with a specific error code
func IsAzureError(err error, errorCode string) bool {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		return strings.EqualFold(respErr.ErrorCode, errorCode)
	}

	return false
}
