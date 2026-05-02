//go:build azure

package azurehelper_test

import (
	"context"
	"io"
	"net/http"
	"strings"
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

// methodCountingTransport returns one body for GET and a different body
// for PUT, while counting calls per method. Used to assert that an
// idempotent helper short-circuits before issuing a write.
type methodCountingTransport struct {
	getBody []byte
	putBody []byte
	gets    int
	puts    int
}

func (m *methodCountingTransport) Do(req *http.Request) (*http.Response, error) {
	body := m.getBody
	if req.Method == http.MethodPut {
		m.puts++
		body = m.putBody
	} else {
		m.gets++
	}

	return &http.Response{
		Request:    req,
		StatusCode: http.StatusOK,
		Status:     http.StatusText(http.StatusOK),
		Body:       io.NopCloser(strings.NewReader(string(body))),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func TestRBAC_AssignRoleIfMissing_AlreadyPresentNoOp(t *testing.T) {
	t.Parallel()

	const (
		principal = "11111111-1111-1111-1111-111111111111"
		roleGUID  = "ba92f5b4-2d11-453d-a403-e96b0029c9fe"
	)

	scope := "/subscriptions/" + testSub
	roleDefID := scope + "/providers/Microsoft.Authorization/roleDefinitions/" + roleGUID

	tr := &methodCountingTransport{
		getBody: jsonBody(map[string]any{
			"value": []any{
				map[string]any{
					"id":   scope + "/providers/Microsoft.Authorization/roleAssignments/existing",
					"name": "existing",
					"properties": map[string]any{
						"principalId":      principal,
						"roleDefinitionId": roleDefID,
					},
				},
			},
		}),
		// PUT body should never be consumed; provide something parseable
		// so a regression that does call Create fails the count assertion
		// rather than the JSON decoder.
		putBody: jsonBody(map[string]any{}),
	}

	c, err := azurehelper.NewRBACClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	err = c.AssignRoleIfMissing(context.Background(), log.New(), azurehelper.AssignRoleInput{
		Scope:            scope,
		PrincipalID:      principal,
		RoleDefinitionID: roleGUID,
	})
	if err != nil {
		t.Fatalf("AssignRoleIfMissing: %v", err)
	}

	if tr.puts != 0 {
		t.Errorf("expected 0 PUT calls (idempotent skip), got %d", tr.puts)
	}

	if tr.gets == 0 {
		t.Error("expected at least one GET to list existing assignments")
	}
}
