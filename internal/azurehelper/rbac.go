// Role-assignment helpers: creating a storage account grants its creator no access to the blobs inside it.

package azurehelper

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/google/uuid"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Built-in Azure role definition ids, stable across every subscription.
const (
	RoleStorageBlobDataOwner       = "b7e6dc6d-f1e8-4753-8033-0f276bb0955b"
	RoleStorageBlobDataContributor = "ba92f5b4-2d11-453d-a403-e96b0029c9fe"
	RoleStorageBlobDataReader      = "2a2b9908-6ea1-4ae2-8e65-a410df84e7d1"
)

// armScope is the token audience whose access token carries the caller's object id.
const armScope = "https://management.azure.com/.default"

// Principal types accepted by the role-assignment API.
const (
	PrincipalTypeUser             = "User"
	PrincipalTypeServicePrincipal = "ServicePrincipal"
)

// jwtParts is the number of dot-separated segments in a JWT.
const jwtParts = 3

// maxRoleAssignmentPages bounds role-assignment list walks so a misbehaving
// service cannot hang bootstrap indefinitely.
const maxRoleAssignmentPages = 100

// roleDefinitionsPath joins a subscription and a role GUID into a full role definition id.
const roleDefinitionsPath = "/providers/Microsoft.Authorization/roleDefinitions/"

// RBACClient wraps the Azure role-assignment management API.
type RBACClient struct {
	client         *armauthorization.RoleAssignmentsClient
	subscriptionID string
}

// NewRBACClient creates a role-assignment client; SAS-token and access-key configs cannot manage RBAC.
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

// AssignRoleInput carries the parameters for a role assignment.
type AssignRoleInput struct {
	// Scope is the resource id the assignment applies to.
	Scope string
	// PrincipalID is the Microsoft Entra object id of a user, group, or service principal.
	PrincipalID string
	// PrincipalType is "User", "Group", or "ServicePrincipal"; empty lets Azure infer it.
	PrincipalType string
	// RoleDefinitionID is the GUID of a built-in or custom role definition.
	RoleDefinitionID string
}

// StorageAccountScope builds the resource id a data-plane role assignment applies to.
func StorageAccountScope(subscriptionID, resourceGroup, account string) string {
	return "/subscriptions/" + subscriptionID +
		"/resourceGroups/" + resourceGroup +
		"/providers/Microsoft.Storage/storageAccounts/" + account
}

// Principal identifies the caller for a role assignment.
type Principal struct {
	// ID is the Microsoft Entra object id.
	ID string
	// Type is "User" or "ServicePrincipal", derived because Azure rejects a mismatch.
	Type string
}

// ResolvePrincipal reads the caller from its own access token, avoiding a Graph call that directory policy often denies.
func ResolvePrincipal(ctx context.Context, cfg *AzureConfig) (Principal, error) {
	if cfg == nil {
		return Principal{}, ErrAzureConfigRequired
	}

	if cfg.Credential == nil {
		return Principal{}, &UnsupportedAuthForOpError{Method: cfg.Method, Operation: "resolving the caller principal"}
	}

	tok, err := cfg.Credential.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{armScope}})
	if err != nil {
		return Principal{}, fmt.Errorf("acquiring token to resolve principal id: %w", err)
	}

	return principalFromToken(tok.Token)
}

// AssignRole creates a role assignment, treating an already-existing one as success.
func (c *RBACClient) AssignRole(ctx context.Context, l log.Logger, in AssignRoleInput) error {
	if err := validateAssignRoleInput(in); err != nil {
		return err
	}

	props := &armauthorization.RoleAssignmentProperties{
		PrincipalID:      new(in.PrincipalID),
		RoleDefinitionID: new("/subscriptions/" + c.subscriptionID + roleDefinitionsPath + in.RoleDefinitionID),
	}

	// A wrongly declared type is rejected as UnmatchedPrincipalType, so an unknown one is omitted.
	if in.PrincipalType != "" {
		props.PrincipalType = new(armauthorization.PrincipalType(in.PrincipalType))
	}

	params := armauthorization.RoleAssignmentCreateParameters{Properties: props}

	// The assignment name is a caller-chosen GUID Azure uses as the resource name.
	_, err := c.client.Create(ctx, in.Scope, uuid.NewString(), params, nil)
	if err == nil {
		l.Debugf("azurehelper: assigned role %s to %s on %s", in.RoleDefinitionID, in.PrincipalID, in.Scope)

		return nil
	}

	if isAlreadyAssigned(err) {
		l.Debugf("azurehelper: role %s already assigned to %s on %s", in.RoleDefinitionID, in.PrincipalID, in.Scope)

		return nil
	}

	return fmt.Errorf("creating role assignment: %w", err)
}

// HasRoleAssignment reports whether principalID already holds roleDefinitionID at scope.
func (c *RBACClient) HasRoleAssignment(ctx context.Context, scope, principalID, roleDefinitionID string) (bool, error) {
	if err := validateAssignRoleInput(AssignRoleInput{Scope: scope, PrincipalID: principalID, RoleDefinitionID: roleDefinitionID}); err != nil {
		return false, err
	}

	roleDefSuffix := roleDefinitionsPath + roleDefinitionID

	// principalId eq is only accepted at subscription scope; assignedTo() works at every scope.
	pager := c.client.NewListForScopePager(scope, &armauthorization.RoleAssignmentsClientListForScopeOptions{
		Filter: new("assignedTo('" + principalID + "')"),
	})

	pages := 0

	for pager.More() {
		pages++
		if pages > maxRoleAssignmentPages {
			return false, &TooManyRoleAssignmentPagesError{Scope: scope, MaxPages: maxRoleAssignmentPages}
		}

		page, err := pager.NextPage(ctx)
		if err != nil {
			return false, fmt.Errorf("listing role assignments: %w", err)
		}

		for _, ra := range page.Value {
			if ra == nil || ra.Properties == nil || ra.Properties.RoleDefinitionID == nil {
				continue
			}

			if ra.Properties.PrincipalID == nil || !strings.EqualFold(*ra.Properties.PrincipalID, principalID) {
				continue
			}

			if strings.HasSuffix(*ra.Properties.RoleDefinitionID, roleDefSuffix) {
				return true, nil
			}
		}
	}

	return false, nil
}

// AssignRoleIfMissing assigns only when missing, so a rerun needs no write permission.
func (c *RBACClient) AssignRoleIfMissing(ctx context.Context, l log.Logger, in AssignRoleInput) error {
	if err := validateAssignRoleInput(in); err != nil {
		return err
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

// RemoveRole removes every matching assignment; removing an absent one is a no-op.
func (c *RBACClient) RemoveRole(ctx context.Context, l log.Logger, scope, principalID, roleDefinitionID string) error {
	if err := validateAssignRoleInput(AssignRoleInput{Scope: scope, PrincipalID: principalID, RoleDefinitionID: roleDefinitionID}); err != nil {
		return err
	}

	roleDefSuffix := roleDefinitionsPath + roleDefinitionID

	// See HasRoleAssignment for why assignedTo() is used here too.
	pager := c.client.NewListForScopePager(scope, &armauthorization.RoleAssignmentsClientListForScopeOptions{
		Filter: new("assignedTo('" + principalID + "')"),
	})

	var errs []error

	removed := 0
	pages := 0

	for pager.More() {
		pages++
		if pages > maxRoleAssignmentPages {
			return &TooManyRoleAssignmentPagesError{Scope: scope, MaxPages: maxRoleAssignmentPages}
		}

		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("listing role assignments for removal: %w", err)
		}

		n, pageErrs := c.deleteMatchingAssignments(ctx, page.Value, principalID, roleDefSuffix)
		errs = append(errs, pageErrs...)
		removed += n
	}

	if err := errors.Join(errs...); err != nil {
		return err
	}

	l.Debugf("azurehelper: removed %d role assignment(s) for principal %s on %s", removed, principalID, scope)

	return nil
}

// deleteMatchingAssignments deletes assignments owned by principalID that match roleDefSuffix.
// A concurrent removal is treated as success. assignedTo() can also surface group-inherited
// rows, so the principal id is checked before delete.
func (c *RBACClient) deleteMatchingAssignments(
	ctx context.Context,
	ras []*armauthorization.RoleAssignment,
	principalID, roleDefSuffix string,
) (int, []error) {
	var errs []error

	removed := 0

	for _, ra := range ras {
		if ra == nil || ra.Properties == nil || ra.Properties.RoleDefinitionID == nil || ra.ID == nil {
			continue
		}

		if ra.Properties.PrincipalID == nil || !strings.EqualFold(*ra.Properties.PrincipalID, principalID) {
			continue
		}

		if !strings.HasSuffix(*ra.Properties.RoleDefinitionID, roleDefSuffix) {
			continue
		}

		if _, err := c.client.DeleteByID(ctx, *ra.ID, nil); err != nil {
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

// validateAssignRoleInput surfaces bad input as a typed error instead of a service 400.
func validateAssignRoleInput(in AssignRoleInput) error {
	if in.Scope == "" || in.PrincipalID == "" || in.RoleDefinitionID == "" {
		return ErrScopePrincipalRoleArgs
	}

	if _, err := uuid.Parse(in.PrincipalID); err != nil {
		return &InvalidPrincipalIDError{PrincipalID: in.PrincipalID}
	}

	if _, err := uuid.Parse(in.RoleDefinitionID); err != nil {
		return &InvalidRoleDefinitionIDError{RoleDefinitionID: in.RoleDefinitionID}
	}

	return nil
}

// principalFromToken reads the oid and idtyp claims; Entra already verified the signature.
func principalFromToken(token string) (Principal, error) {
	segments := strings.Split(token, ".")
	if len(segments) != jwtParts {
		return Principal{}, ErrPrincipalIDUnresolved
	}

	payload, err := base64.RawURLEncoding.DecodeString(segments[1])
	if err != nil {
		return Principal{}, ErrPrincipalIDUnresolved
	}

	var claims struct {
		OID   string `json:"oid"`
		IDTyp string `json:"idtyp"`
	}

	if err := json.Unmarshal(payload, &claims); err != nil {
		return Principal{}, ErrPrincipalIDUnresolved
	}

	if claims.OID == "" {
		return Principal{}, ErrPrincipalIDUnresolved
	}

	// Entra sets idtyp to "app" on an app-only token.
	principalType := PrincipalTypeUser
	if strings.EqualFold(claims.IDTyp, "app") {
		principalType = PrincipalTypeServicePrincipal
	}

	return Principal{ID: claims.OID, Type: principalType}, nil
}

// isAlreadyAssigned matches by error code so wrapping or SDK message changes do not break it.
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
