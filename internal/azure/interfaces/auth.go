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
// from specific Azure SDK authentication implementations.
//
// The interface is intentionally small. Getters for subscription ID, tenant ID, etc.
// are available as exported methods on the concrete AuthenticationServiceImpl struct
// but are not part of the interface because no external caller uses them through it.
// Stateless error classifiers (IsAuthenticationError, IsTokenExpiredError, IsPermissionError)
// live in the errorutil package as standalone functions.
//
// Usage examples:
//
//	// Get credentials for Azure service authentication
//	credential, err := authService.GetCredential(ctx, config)
//
//	// Validate current credentials
//	err := authService.ValidateCredentials(ctx)
//
//	// Get current principal information
//	principal, err := authService.GetCurrentPrincipal(ctx)
type AuthenticationService interface {
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

	// GetCurrentPrincipal retrieves information about the currently authenticated principal.
	// Returns details about the user, service principal, or managed identity.
	GetCurrentPrincipal(ctx context.Context) (interface{}, error)

	// IsPermissionError checks if an error is due to insufficient permissions.
	// This helps distinguish between authentication and authorization errors.
	IsPermissionError(err error) bool
}

// AuthInfo holds authentication metadata that is known at credential creation time.
// These values were previously exposed as interface methods (GetSubscriptionID, GetTenantID, etc.)
// but are better represented as plain struct fields since they are set once and never change.
type AuthInfo struct {
	SubscriptionID       string // Azure subscription ID
	TenantID             string // Azure AD tenant ID
	ClientID             string // Azure AD application client ID
	AuthenticationMethod string // "azuread", "service_principal", "msi", "cli", "sas_token"
	CloudEnvironment     string // "public", "government", "china"
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
