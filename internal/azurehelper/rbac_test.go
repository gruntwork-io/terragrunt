//go:build azure

package azurehelper_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func TestNewRBACClient_NilConfig(t *testing.T) {
	t.Parallel()

	if _, err := azurehelper.NewRBACClient(nil); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestNewRBACClient_MissingSubscription(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewRBACClient(&azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodAzureAD,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	if err == nil {
		t.Fatal("expected error for missing subscription_id")
	}
}

func TestNewRBACClient_MissingCredential(t *testing.T) {
	t.Parallel()
	// SAS token has no token credential — RBAC needs one.
	_, err := azurehelper.NewRBACClient(&azurehelper.AzureConfig{
		Method:         azurehelper.AuthMethodSasToken,
		SubscriptionID: testSub,
		SasToken:       testSASToken,
		ClientOptions:  azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	if err == nil {
		t.Fatal("expected error when SAS-token config used for RBAC")
	}
}

func newTestRBACClient(t *testing.T) *azurehelper.RBACClient {
	t.Helper()

	c, err := azurehelper.NewRBACClient(&azurehelper.AzureConfig{
		Method:         azurehelper.AuthMethodAzureAD,
		SubscriptionID: testSub,
		Credential:     fakeCredential{},
		ClientOptions:  azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	return c
}

func TestRBAC_AssignRole_RequiresArgs(t *testing.T) {
	t.Parallel()

	c := newTestRBACClient(t)

	cases := []azurehelper.AssignRoleInput{
		{Scope: "", PrincipalID: "p", RoleDefinitionID: "r"},
		{Scope: "s", PrincipalID: "", RoleDefinitionID: "r"},
		{Scope: "s", PrincipalID: "p", RoleDefinitionID: ""},
	}

	for _, in := range cases {
		if err := c.AssignRole(context.Background(), log.New(), in); err == nil {
			t.Errorf("AssignRole(%+v) should error", in)
		}
	}
}

func TestRBAC_AssignRoleIfMissing_RequiresArgs(t *testing.T) {
	t.Parallel()

	c := newTestRBACClient(t)

	if err := c.AssignRoleIfMissing(context.Background(), log.New(), azurehelper.AssignRoleInput{}); err == nil {
		t.Error("AssignRoleIfMissing with empty input should error")
	}
}

func TestRBAC_HasRoleAssignment_RequiresArgs(t *testing.T) {
	t.Parallel()

	c := newTestRBACClient(t)

	if _, err := c.HasRoleAssignment(context.Background(), "", "p", "r"); err == nil {
		t.Error("HasRoleAssignment with empty scope should error")
	}

	if _, err := c.HasRoleAssignment(context.Background(), "s", "", "r"); err == nil {
		t.Error("HasRoleAssignment with empty principal should error")
	}

	if _, err := c.HasRoleAssignment(context.Background(), "s", "p", ""); err == nil {
		t.Error("HasRoleAssignment with empty role should error")
	}
}

func TestRBAC_RemoveRole_RequiresArgs(t *testing.T) {
	t.Parallel()

	c := newTestRBACClient(t)

	if err := c.RemoveRole(context.Background(), log.New(), "", "p", "r"); err == nil {
		t.Error("RemoveRole with empty scope should error")
	}

	if err := c.RemoveRole(context.Background(), log.New(), "s", "", "r"); err == nil {
		t.Error("RemoveRole with empty principal should error")
	}

	if err := c.RemoveRole(context.Background(), log.New(), "s", "p", ""); err == nil {
		t.Error("RemoveRole with empty role should error")
	}
}

func TestRBAC_AssignRole_RejectsNonUUIDPrincipal(t *testing.T) {
	t.Parallel()

	c := newTestRBACClient(t)

	err := c.AssignRole(context.Background(), log.New(), azurehelper.AssignRoleInput{
		Scope:            "/subscriptions/x",
		PrincipalID:      "not-a-uuid",
		RoleDefinitionID: "ba92f5b4-2d11-453d-a403-e96b0029c9fe",
	})
	if err == nil {
		t.Fatal("expected error for non-UUID principal_id")
	}
}

func TestRBAC_RemoveRole_RejectsNonUUIDPrincipal(t *testing.T) {
	t.Parallel()

	c := newTestRBACClient(t)

	if err := c.RemoveRole(context.Background(), log.New(), "/subscriptions/x", "not-a-uuid", "rdid"); err == nil {
		t.Fatal("expected error for non-UUID principal_id")
	}
}

func TestRBAC_RemoveRole_NoMatchIsNoop(t *testing.T) {
	t.Parallel()
	// Empty list → no role assignments to delete.
	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"value": []any{},
	})}

	c, err := azurehelper.NewRBACClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := c.RemoveRole(context.Background(), log.New(),
		"/subscriptions/sub",
		"11111111-1111-1111-1111-111111111111",
		"ba92f5b4-2d11-453d-a403-e96b0029c9fe",
	); err != nil {
		t.Errorf("RemoveRole on empty list: %v", err)
	}
}
