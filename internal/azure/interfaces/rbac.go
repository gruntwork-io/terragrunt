// Package interfaces provides interface definitions for Azure RBAC services used by Terragrunt
package interfaces

import (
	"context"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// RBAC role definition names
const (
	RoleOwner                 = "Owner"
	RoleContributor           = "Contributor"
	RoleReader                = "Reader"
	RoleStorageBlobDataOwner  = "Storage Blob Data Owner"
	RoleStorageBlobDataReader = "Storage Blob Data Reader"
	RoleStorageBlobDataWriter = "Storage Blob Data Contributor"
)

// Principal represents an Azure AD principal (user, service principal, or group)
type Principal struct {
	ID          string
	Type        string
	DisplayName string
}

// RoleAssignment represents an Azure RBAC role assignment
type RoleAssignment struct {
	RoleName    string
	PrincipalID string
	Scope       string
	Description string
}

// RoleDefinition represents an Azure RBAC role definition
type RoleDefinition struct {
	ID          *string
	Name        *string
	Type        *string
	RoleName    *string
	Description *string
	Permissions []Permission
}

// Permission represents an Azure RBAC permission
type Permission struct {
	Actions        []string
	NotActions     []string
	DataActions    []string
	NotDataActions []string
}

// RBACService defines the interface for Azure RBAC operations.
// This interface abstracts Azure RBAC operations to improve testability and decouple
// from the Azure SDK. It provides methods for role assignments, role definitions,
// and principal management.
//
// Usage examples:
//
//	// Assign Storage Blob Data Owner role to current principal
//	err := rbacService.AssignStorageBlobDataOwnerRole(ctx, logger, storageAccountScope)
//
//	// Check if principal has specific role assignment
//	hasRole, err := rbacService.HasRoleAssignment(ctx, principalID, roleDefID, scope)
//
//	// Get current principal ID for role assignments
//	principalID, err := rbacService.GetPrincipalID(ctx)
type RBACService interface {
	// Role Management
	AssignRole(ctx context.Context, l log.Logger, roleName, principalID, scope string) error
	RemoveRole(ctx context.Context, l log.Logger, roleName, principalID, scope string) error
	AssignStorageBlobDataOwnerRole(ctx context.Context, l log.Logger, scope string) error

	// Principal Management
	GetCurrentPrincipal(ctx context.Context) (*Principal, error)
	GetPrincipal(ctx context.Context, principalID string) (*Principal, error)
	GetPrincipalID(ctx context.Context) (string, error)

	// Utility
	IsPermissionError(err error) bool
}

// RBACConfig represents configuration for RBAC operations.
// This configuration controls retry behavior, timeouts, and other RBAC-specific settings.
type RBACConfig struct {
	// MaxRetries specifies the maximum number of retry attempts for RBAC operations.
	// RBAC operations can be slow due to propagation delays.
	// Default: 5
	MaxRetries int

	// RetryDelay specifies the delay between retry attempts.
	// RBAC operations often need longer delays due to Azure AD propagation.
	// Default: 3 seconds
	RetryDelay int

	// PropagationTimeout specifies how long to wait for role assignments to propagate.
	// Role assignments can take time to become effective across Azure services.
	// Default: 60 seconds
	PropagationTimeout int

	// EnableRetry indicates whether to enable retry logic for RBAC operations.
	// Default: true
	EnableRetry bool
}

const (
	defaultRBACMaxRetries         = 5
	defaultRBACRetryDelaySeconds  = 3
	defaultRBACPropagationTimeout = 60
)

// DefaultRBACConfig returns the default configuration for RBAC operations.
func DefaultRBACConfig() RBACConfig {
	return RBACConfig{
		MaxRetries:         defaultRBACMaxRetries,
		RetryDelay:         defaultRBACRetryDelaySeconds,
		PropagationTimeout: defaultRBACPropagationTimeout,
		EnableRetry:        true,
	}
}
