// Role-assignment helpers.
//
// RBACClient wraps Azure's armauthorization RoleAssignmentsClient and
// exposes the small surface needed to bootstrap data-plane access for
// the remote-state backend (assign idempotently, list, and remove).

package azurehelper

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/google/uuid"

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

// roleDefinitionsPath is the provider path segment that, appended to a scope
// and a role definition GUID, forms a full role definition ID.
const roleDefinitionsPath = "/providers/Microsoft.Authorization/roleDefinitions/"

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
		return nil, ErrAzureConfigRequired
	}

	if cfg.SubscriptionID == "" {
		return nil, ErrSubscriptionIDRequired
	}

	if cfg.Credential == nil {
		return nil, &UnsupportedAuthForOpError{Method: cfg.Method, Operation: "RBAC operations"}
	}

	clientFactory, err := armauthorization.NewClientFactory(cfg.SubscriptionID, cfg.Credential, &arm.ClientOptions{
		ClientOptions: cfg.ClientOptions,
	})
	if err != nil {
		return nil, fmt.Errorf("creating armauthorization client factory: %w", err)
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
	// RoleDefinitionID is the GUID portion of a built-in or custom role
	// definition. The full definition ID is composed for you.
	RoleDefinitionID string
}

// AssignRole creates a role assignment. If an assignment satisfying the same
// (Scope, PrincipalID, RoleDefinitionID) tuple already exists, this returns
// nil — the operation is idempotent.
func (c *RBACClient) AssignRole(ctx context.Context, l log.Logger, in AssignRoleInput) error {
	if in.Scope == "" || in.PrincipalID == "" || in.RoleDefinitionID == "" {
		return ErrScopePrincipalRoleArgs
	}

	if _, err := uuid.Parse(in.PrincipalID); err != nil {
		return &InvalidPrincipalIDError{PrincipalID: in.PrincipalID}
	}

	if _, err := uuid.Parse(in.RoleDefinitionID); err != nil {
		return &InvalidRoleDefinitionIDError{RoleDefinitionID: in.RoleDefinitionID}
	}

	principalType := in.PrincipalType
	if principalType == "" {
		principalType = "ServicePrincipal"
	}

	assignmentName := uuid.NewString()
	roleDefID := "/subscriptions/" + c.subscriptionID +
		roleDefinitionsPath + in.RoleDefinitionID

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

	return fmt.Errorf("creating role assignment: %w", err)
}

// HasRoleAssignment reports whether principalID already holds the role
// identified by roleDefinitionID at scope.
func (c *RBACClient) HasRoleAssignment(ctx context.Context, scope, principalID, roleDefinitionID string) (bool, error) {
	if scope == "" || principalID == "" || roleDefinitionID == "" {
		return false, ErrScopePrincipalRoleArgs
	}

	if _, err := uuid.Parse(principalID); err != nil {
		return false, &InvalidPrincipalIDError{PrincipalID: principalID}
	}

	if _, err := uuid.Parse(roleDefinitionID); err != nil {
		return false, &InvalidRoleDefinitionIDError{RoleDefinitionID: roleDefinitionID}
	}

	roleDefSuffix := roleDefinitionsPath + roleDefinitionID

	// Azure's roleAssignments List for Scope API only supports the
	// `principalId eq '<id>'` filter at subscription scope. At resource
	// group or resource scope it returns 400 UnsupportedQuery and only
	// accepts `atScope()` or `assignedTo('<id>')`. assignedTo() works at
	// every scope (and additionally surfaces assignments granted via
	// group membership) so use it unconditionally.
	pager := c.client.NewListForScopePager(scope, &armauthorization.RoleAssignmentsClientListForScopeOptions{
		Filter: to.Ptr("assignedTo('" + principalID + "')"),
	})
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return false, fmt.Errorf("listing role assignments: %w", err)
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

// AssignRoleIfMissing is the idempotent convenience wrapper most callers
// want: it first checks whether (scope, principalID, roleDefinitionID) is
// already assigned and only issues AssignRole when it is not. This avoids
// the cost (and audit-log noise) of redundant Create calls on bootstrap
// reruns.
func (c *RBACClient) AssignRoleIfMissing(ctx context.Context, l log.Logger, in AssignRoleInput) error {
	if in.Scope == "" || in.PrincipalID == "" || in.RoleDefinitionID == "" {
		return ErrScopePrincipalRoleArgs
	}

	if _, err := uuid.Parse(in.PrincipalID); err != nil {
		return &InvalidPrincipalIDError{PrincipalID: in.PrincipalID}
	}

	if _, err := uuid.Parse(in.RoleDefinitionID); err != nil {
		return &InvalidRoleDefinitionIDError{RoleDefinitionID: in.RoleDefinitionID}
	}

	has, err := c.HasRoleAssignment(ctx, in.Scope, in.PrincipalID, in.RoleDefinitionID)
	if err != nil {
		return err
	}

	if has {
		l.Debugf("azurehelper: role %s already assigned to %s on %s; skipping create",
			in.RoleDefinitionID, in.PrincipalID, in.Scope)

		return nil
	}

	return c.AssignRole(ctx, l, in)
}

// RemoveRole removes the assignment of roleDefinitionID for principalID at
// scope. If no such assignment exists this is a no-op (returns nil).
//
// When multiple matching assignments exist (uncommon but possible), all of
// them are deleted; any delete errors are aggregated via errors.Join.
func (c *RBACClient) RemoveRole(ctx context.Context, l log.Logger, scope, principalID, roleDefinitionID string) error {
	if scope == "" || principalID == "" || roleDefinitionID == "" {
		return ErrScopePrincipalRoleArgs
	}

	if _, err := uuid.Parse(principalID); err != nil {
		return &InvalidPrincipalIDError{PrincipalID: principalID}
	}

	if _, err := uuid.Parse(roleDefinitionID); err != nil {
		return &InvalidRoleDefinitionIDError{RoleDefinitionID: roleDefinitionID}
	}

	roleDefSuffix := roleDefinitionsPath + roleDefinitionID

	// See HasRoleAssignment for why assignedTo() is used instead of
	// `principalId eq '<id>'`: the eq filter is only accepted at
	// subscription scope.
	pager := c.client.NewListForScopePager(scope, &armauthorization.RoleAssignmentsClientListForScopeOptions{
		Filter: to.Ptr("assignedTo('" + principalID + "')"),
	})

	var errs []error

	removed := 0

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("listing role assignments for removal: %w", err)
		}

		n, pageErrs := c.deleteMatchingAssignments(ctx, page.Value, roleDefSuffix)
		errs = append(errs, pageErrs...)
		removed += n
	}

	if err := errors.Join(errs...); err != nil {
		return err
	}

	l.Debugf("azurehelper: removed %d role assignment(s) for principal %s on %s", removed, principalID, scope)

	return nil
}

// deleteMatchingAssignments deletes every assignment in ras whose role
// definition ID ends with roleDefSuffix, returning how many were removed and
// any delete errors. Assignments removed concurrently (404) count as success.
func (c *RBACClient) deleteMatchingAssignments(ctx context.Context, ras []*armauthorization.RoleAssignment, roleDefSuffix string) (int, []error) {
	var errs []error

	removed := 0

	for _, ra := range ras {
		if ra == nil || ra.Properties == nil || ra.Properties.RoleDefinitionID == nil || ra.ID == nil {
			continue
		}

		if !strings.HasSuffix(*ra.Properties.RoleDefinitionID, roleDefSuffix) {
			continue
		}

		if _, err := c.client.DeleteByID(ctx, *ra.ID, nil); err != nil {
			// Concurrent removal raced us to the same assignment - treat as success.
			if IsNotFound(err) {
				continue
			}

			errs = append(errs, fmt.Errorf("deleting role assignment %s: %w", *ra.ID, err))

			continue
		}

		removed++
	}

	return removed, errs
}

// isAlreadyAssigned returns true for the Azure "RoleAssignmentExists"
// error, which is a 409 with that error code. Matches by typed error code
// rather than substring on err.Error() so wrapping or SDK message changes
// do not break the check.
func isAlreadyAssigned(err error) bool {
	if err == nil {
		return false
	}

	respErr, ok := errors.AsType[*azcore.ResponseError](err)
	if !ok {
		return false
	}

	return strings.EqualFold(respErr.ErrorCode, "RoleAssignmentExists")
}
