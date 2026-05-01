package azurehelper

import (
	"context"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/google/uuid"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Built-in Azure role definition IDs (GUIDs). These are stable across all
// subscriptions. The full role definition ID is
// /subscriptions/{sub}/providers/Microsoft.Authorization/roleDefinitions/{guid}.
//
// See: https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles
const (
	RoleStorageBlobDataOwner       = "b7e6dc6d-f1e8-4753-8033-0f276bb0955b"
	RoleStorageBlobDataContributor = "ba92f5b4-2d11-453d-a403-e96b0029c9fe"
	RoleStorageBlobDataReader      = "2a2b9908-6ea1-4ae2-8e65-a410df84e7d1"
)

// RBAC propagation timing. Azure can take several minutes to propagate role
// assignments to data-plane services; callers needing immediate access should
// poll with these values.
const (
	RBACRetryDelay         = 10 * time.Second
	RBACMaxRetries         = 30
	RBACPropagationTimeout = 5 * time.Minute
)

// RBACClient wraps the Azure role-assignment management API.
type RBACClient struct {
	client         *armauthorization.RoleAssignmentsClient
	subscriptionID string
}

// NewRBACClient creates a role-assignment client scoped to cfg.SubscriptionID.
// cfg must carry a token credential (SAS-token and access-key configs are
// data-plane only and cannot manage RBAC).
func NewRBACClient(cfg *AzureConfig) (*RBACClient, error) {
	if cfg == nil {
		return nil, errors.Errorf("azure config is required")
	}

	if cfg.SubscriptionID == "" {
		return nil, errors.Errorf("subscription_id is required for RBAC operations")
	}

	if cfg.Credential == nil {
		return nil, errors.Errorf("RBAC operations require a token credential (auth method %q is not supported)", cfg.Method)
	}

	clientFactory, err := armauthorization.NewClientFactory(cfg.SubscriptionID, cfg.Credential, &arm.ClientOptions{
		ClientOptions: cfg.ClientOptions,
	})
	if err != nil {
		return nil, errors.Errorf("creating armauthorization client factory: %w", err)
	}

	return &RBACClient{
		client:         clientFactory.NewRoleAssignmentsClient(),
		subscriptionID: cfg.SubscriptionID,
	}, nil
}

// AssignRoleInput parameters for AssignRole.
type AssignRoleInput struct {
	// Scope is the resource ID the assignment applies to (e.g. a storage
	// account ID, resource group, or subscription).
	Scope string
	// PrincipalID is the AAD object ID of the user, group, or service principal.
	PrincipalID string
	// PrincipalType: "User", "Group", "ServicePrincipal". Defaults to ServicePrincipal.
	PrincipalType string
	// RoleDefinitionID is the GUID portion of a role definition (e.g.
	// RoleStorageBlobDataOwner). The full definition ID is composed for you.
	RoleDefinitionID string
}

// AssignRole creates a role assignment. If an assignment satisfying the same
// (Scope, PrincipalID, RoleDefinitionID) tuple already exists, this returns
// nil — the operation is idempotent.
func (c *RBACClient) AssignRole(ctx context.Context, l log.Logger, in AssignRoleInput) error {
	if in.Scope == "" || in.PrincipalID == "" || in.RoleDefinitionID == "" {
		return errors.Errorf("scope, principal_id, and role_definition_id are required")
	}

	principalType := in.PrincipalType
	if principalType == "" {
		principalType = "ServicePrincipal"
	}

	assignmentName := uuid.NewString()
	roleDefID := "/subscriptions/" + c.subscriptionID +
		"/providers/Microsoft.Authorization/roleDefinitions/" + in.RoleDefinitionID

	params := armauthorization.RoleAssignmentCreateParameters{
		Properties: &armauthorization.RoleAssignmentProperties{
			PrincipalID:      to.Ptr(in.PrincipalID),
			PrincipalType:    to.Ptr(armauthorization.PrincipalType(principalType)),
			RoleDefinitionID: to.Ptr(roleDefID),
		},
	}

	_, err := c.client.Create(ctx, in.Scope, assignmentName, params, nil)
	if err == nil {
		l.Debugf("azurehelper: assigned role %s to %s on %s", in.RoleDefinitionID, in.PrincipalID, in.Scope)
		return nil
	}

	// Azure returns 409 RoleAssignmentExists when the same role is already
	// assigned to the same principal at the same scope.
	if isAlreadyAssigned(err) {
		l.Debugf("azurehelper: role %s already assigned to %s on %s", in.RoleDefinitionID, in.PrincipalID, in.Scope)
		return nil
	}

	return WrapError(err, "creating role assignment")
}

// HasRoleAssignment reports whether principalID already holds the role
// identified by roleDefinitionID at scope.
func (c *RBACClient) HasRoleAssignment(ctx context.Context, scope, principalID, roleDefinitionID string) (bool, error) {
	if scope == "" || principalID == "" || roleDefinitionID == "" {
		return false, errors.Errorf("scope, principal_id, and role_definition_id are required")
	}

	roleDefSuffix := "/providers/Microsoft.Authorization/roleDefinitions/" + roleDefinitionID

	pager := c.client.NewListForScopePager(scope, &armauthorization.RoleAssignmentsClientListForScopeOptions{
		Filter: to.Ptr("principalId eq '" + principalID + "'"),
	})
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return false, WrapError(err, "listing role assignments")
		}

		for _, ra := range page.Value {
			if ra == nil || ra.Properties == nil || ra.Properties.RoleDefinitionID == nil {
				continue
			}

			if strings.HasSuffix(*ra.Properties.RoleDefinitionID, roleDefSuffix) {
				return true, nil
			}
		}
	}

	return false, nil
}

// isAlreadyAssigned returns true for the Azure "RoleAssignmentExists" error,
// which is a 409 with that error code.
func isAlreadyAssigned(err error) bool {
	if err == nil {
		return false
	}
	// Match by error code (most reliable across SDK versions).
	return strings.Contains(err.Error(), "RoleAssignmentExists")
}
