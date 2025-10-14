// Package interfaces provides interface definitions for Azure authentication services used by Terragrunt
package interfaces

import (
	"context"
	"errors"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

// Common errors for authentication operations
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrNotImplemented     = errors.New("not implemented")
)

const (
	defaultAuthMethod                  = "azuread"
	defaultCloudEnvironment            = "public"
	defaultTokenRefreshThresholdSecond = 300
	defaultAuthMaxRetries              = 3
	defaultAuthRetryDelaySecond        = 1
	defaultAuthTimeoutSecond           = 30
)

// PrincipalInfo represents information about an Azure AD principal
type PrincipalInfo struct {
	ID   string
	Name string
	Type string // "User", "ServicePrincipal", "Group", "ManagedIdentity"
}

// AuthenticationService defines the interface for Azure authentication operations.
// This interface abstracts Azure authentication to improve testability and decouple
// from specific Azure SDK authentication implementations. It provides methods for
// credential management, token operations, and authentication information retrieval.
//
// Usage examples:
//
//	// Get credentials for Azure service authentication
//	credential, err := authService.GetCredential(ctx, config)
//
//	// Validate current credentials
//	err := authService.ValidateCredentials(ctx)
//
//	// Get current subscription ID
//	subscriptionID, err := authService.GetSubscriptionID(ctx)
//
//	// Get access token for specific scopes
//	token, err := authService.GetAccessToken(ctx, []string{"https://storage.azure.com/.default"})
type AuthenticationService interface {
	// Credential Management

	// GetCredential creates and returns an Azure credential based on the provided configuration.
	// The configuration map should contain authentication parameters such as:
	// - client_id, client_secret, tenant_id for Service Principal auth
	// - use_msi for Managed Service Identity auth
	// - use_azuread_auth for Azure AD auth
	// Returns an azcore.TokenCredential that can be used with Azure SDK clients.
	GetCredential(ctx context.Context, config map[string]interface{}) (azcore.TokenCredential, error)

	// ValidateCredentials validates that the current credentials are valid and can authenticate.
	// This method performs a lightweight operation to verify credential validity.
	ValidateCredentials(ctx context.Context) error

	// RefreshCredentials refreshes the current credentials if they support refresh.
	// This is useful for long-running operations that might exceed token lifetimes.
	RefreshCredentials(ctx context.Context) error

	// Authentication Information

	// GetCurrentPrincipal retrieves information about the currently authenticated principal.
	// Returns details about the user, service principal, or managed identity.
	GetCurrentPrincipal(ctx context.Context) (interface{}, error)

	// GetSubscriptionID retrieves the Azure subscription ID from the current authentication context.
	// This may come from configuration, environment variables, or the authenticated context.
	GetSubscriptionID(ctx context.Context) (string, error)

	// GetTenantID retrieves the Azure AD tenant ID from the current authentication context.
	// This may come from configuration, environment variables, or the authenticated context.
	GetTenantID(ctx context.Context) (string, error)

	// GetClientID retrieves the client ID from the current authentication context.
	// This is applicable for Service Principal and some other authentication methods.
	GetClientID(ctx context.Context) (string, error)

	// Token Operations

	// GetAccessToken retrieves an access token for the specified scopes.
	// scopes: The OAuth 2.0 scopes for which to request the token
	// Returns a token that can be used for direct API calls.
	GetAccessToken(ctx context.Context, scopes []string) (string, error)

	// RefreshToken refreshes the current access token if it's expired or near expiry.
	// This is handled automatically by most Azure SDK clients but can be useful for direct API calls.
	RefreshToken(ctx context.Context) error

	// Authentication Method Detection

	// GetAuthenticationMethod returns the authentication method being used.
	// Possible values: "azuread", "service_principal", "msi", "cli", "sas_token"
	GetAuthenticationMethod(ctx context.Context) (string, error)

	// IsServicePrincipal returns true if authenticating with a Service Principal.
	IsServicePrincipal(ctx context.Context) (bool, error)

	// IsManagedIdentity returns true if authenticating with Managed Service Identity.
	IsManagedIdentity(ctx context.Context) (bool, error)

	// IsAzureAD returns true if authenticating with Azure AD (default method).
	IsAzureAD(ctx context.Context) (bool, error)

	// Cloud Environment Support

	// GetCloudEnvironment returns the Azure cloud environment being used.
	// Possible values: "public", "government", "china", "german"
	GetCloudEnvironment(ctx context.Context) (string, error)

	// SetCloudEnvironment sets the Azure cloud environment for authentication.
	// environment: The cloud environment name ("public", "government", "china", "german")
	SetCloudEnvironment(ctx context.Context, environment string) error

	// Configuration Management

	// GetConfiguration returns the current authentication configuration.
	// This includes sanitized configuration (secrets are masked).
	GetConfiguration(ctx context.Context) (map[string]interface{}, error)

	// UpdateConfiguration updates the authentication configuration.
	// config: New configuration parameters to apply
	UpdateConfiguration(ctx context.Context, config map[string]interface{}) error

	// Error Handling and Utilities

	// IsAuthenticationError checks if an error is related to authentication.
	// This is useful for error classification and retry logic.
	IsAuthenticationError(err error) bool

	// IsTokenExpiredError checks if an error indicates an expired token.
	// This can trigger automatic token refresh in retry logic.
	IsTokenExpiredError(err error) bool

	// IsPermissionError checks if an error is due to insufficient permissions.
	// This helps distinguish between authentication and authorization errors.
	IsPermissionError(err error) bool
}

// AuthenticationConfig represents configuration for authentication operations.
// This configuration controls authentication behavior, retry logic, and token management.
type AuthenticationConfig struct {
	// Method specifies the authentication method to use.
	// Valid values: "azuread", "service_principal", "msi", "cli", "sas_token"
	// Default: "azuread"
	Method string

	// CloudEnvironment specifies the Azure cloud environment.
	// Valid values: "public", "government", "china", "german"
	// Default: "public"
	CloudEnvironment string

	// TokenCachePath specifies the path for token caching (optional).
	// When specified, tokens are cached to improve performance.
	TokenCachePath string

	// SubscriptionID is the Azure subscription ID
	SubscriptionID string

	// TenantID is the Azure AD tenant ID
	TenantID string

	// ClientID is the Azure AD application client ID
	ClientID string

	// ClientSecret is the Azure AD application client secret
	ClientSecret string

	// TokenRefreshThreshold specifies when to refresh tokens (in seconds before expiry).
	// Default: 300 (5 minutes)
	TokenRefreshThreshold int

	// MaxRetries specifies the maximum number of retry attempts for authentication operations.
	// Default: 3
	MaxRetries int

	// RetryDelay specifies the delay between retry attempts (in seconds).
	// Default: 1
	RetryDelay int

	// Timeout specifies the timeout for authentication operations (in seconds).
	// Default: 30
	Timeout int

	// EnableTokenCache indicates whether to enable token caching.
	// Default: true
	EnableTokenCache bool

	// UseManagedIdentity indicates whether to use managed identity authentication
	UseManagedIdentity bool
}

// DefaultAuthenticationConfig returns the default configuration for authentication operations.
func DefaultAuthenticationConfig() AuthenticationConfig {
	return AuthenticationConfig{
		Method:                defaultAuthMethod,
		CloudEnvironment:      defaultCloudEnvironment,
		TokenCachePath:        "",
		EnableTokenCache:      true,
		TokenRefreshThreshold: defaultTokenRefreshThresholdSecond,
		MaxRetries:            defaultAuthMaxRetries,
		RetryDelay:            defaultAuthRetryDelaySecond,
		Timeout:               defaultAuthTimeoutSecond,
	}
}
