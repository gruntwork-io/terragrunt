package azurehelper_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	testRoleDefinitions = "/subscriptions/x/providers/Microsoft.Authorization/roleDefinitions/"
	testPrincipalID     = "11111111-2222-3333-4444-555555555555"
	testScope           = "/subscriptions/" + testSub + "/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/" + testAccount
)

func TestNewRBACClient_Validation(t *testing.T) {
	t.Parallel()

	tt := []struct {
		checkAs func(t *testing.T, err error)
		cfg     *azurehelper.AzureConfig
		wantIs  error
		name    string
	}{
		{name: "nil config", cfg: nil, wantIs: azurehelper.ErrAzureConfigRequired},
		{name: "missing subscription", cfg: &azurehelper.AzureConfig{Credential: fakeCredential{}}, wantIs: azurehelper.ErrSubscriptionIDRequired},
		{
			name: "data-plane auth cannot manage rbac",
			cfg: &azurehelper.AzureConfig{
				SubscriptionID: testSub,
				Method:         azurehelper.AuthMethodAccessKey,
			},
			checkAs: func(t *testing.T, err error) {
				t.Helper()

				var unsupported *azurehelper.UnsupportedAuthForOpError
				require.ErrorAs(t, err, &unsupported)
				assert.Equal(t, azurehelper.AuthMethodAccessKey, unsupported.Method)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := azurehelper.NewRBACClient(tc.cfg)
			require.Error(t, err)

			if tc.wantIs != nil {
				require.ErrorIs(t, err, tc.wantIs)
			}

			if tc.checkAs != nil {
				tc.checkAs(t, err)
			}
		})
	}
}

func TestAssignRole_RejectsMalformedInput(t *testing.T) {
	t.Parallel()

	c, err := azurehelper.NewRBACClient(cfgWithTransport(&stubTransport{status: http.StatusOK, body: []byte(`{}`)}))
	require.NoError(t, err)

	tt := []struct {
		wantErr error
		name    string
		in      azurehelper.AssignRoleInput
	}{
		{
			name:    "empty scope",
			in:      azurehelper.AssignRoleInput{PrincipalID: testPrincipalID, RoleDefinitionID: azurehelper.RoleStorageBlobDataContributor},
			wantErr: azurehelper.ErrScopePrincipalRoleArgs,
		},
		{
			name:    "empty principal",
			in:      azurehelper.AssignRoleInput{Scope: testScope, RoleDefinitionID: azurehelper.RoleStorageBlobDataContributor},
			wantErr: azurehelper.ErrScopePrincipalRoleArgs,
		},
		{
			name:    "empty role definition",
			in:      azurehelper.AssignRoleInput{Scope: testScope, PrincipalID: testPrincipalID},
			wantErr: azurehelper.ErrScopePrincipalRoleArgs,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, c.AssignRole(t.Context(), log.New(), tc.in), tc.wantErr)
		})
	}
}

func TestAssignRole_RejectsNonUUIDs(t *testing.T) {
	t.Parallel()

	c, err := azurehelper.NewRBACClient(cfgWithTransport(&stubTransport{status: http.StatusOK, body: []byte(`{}`)}))
	require.NoError(t, err)

	err = c.AssignRole(t.Context(), log.New(), azurehelper.AssignRoleInput{
		Scope:            testScope,
		PrincipalID:      "not-a-uuid",
		RoleDefinitionID: azurehelper.RoleStorageBlobDataContributor,
	})

	var principalErr *azurehelper.InvalidPrincipalIDError
	require.ErrorAs(t, err, &principalErr)

	err = c.AssignRole(t.Context(), log.New(), azurehelper.AssignRoleInput{
		Scope:            testScope,
		PrincipalID:      testPrincipalID,
		RoleDefinitionID: "not-a-uuid",
	})

	var roleErr *azurehelper.InvalidRoleDefinitionIDError
	require.ErrorAs(t, err, &roleErr)
}

// TestAssignRole_ExistingAssignmentIsNotAnError covers bootstrap reruns: Azure
// answers 409 RoleAssignmentExists, which must not fail the bootstrap.
func TestAssignRole_ExistingAssignmentIsNotAnError(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{
		status: http.StatusConflict,
		body: jsonBody(map[string]any{
			"error": map[string]any{"code": "RoleAssignmentExists", "message": "already exists"},
		}),
	}

	c, err := azurehelper.NewRBACClient(cfgWithTransport(tr))
	require.NoError(t, err)

	require.NoError(t, c.AssignRole(t.Context(), log.New(), azurehelper.AssignRoleInput{
		Scope:            testScope,
		PrincipalID:      testPrincipalID,
		RoleDefinitionID: azurehelper.RoleStorageBlobDataContributor,
	}))
}

func TestAssignRole_SurfacesOtherFailures(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{
		status: http.StatusForbidden,
		body: jsonBody(map[string]any{
			"error": map[string]any{"code": "AuthorizationFailed", "message": "no permission"},
		}),
	}

	c, err := azurehelper.NewRBACClient(cfgWithTransport(tr))
	require.NoError(t, err)

	err = c.AssignRole(t.Context(), log.New(), azurehelper.AssignRoleInput{
		Scope:            testScope,
		PrincipalID:      testPrincipalID,
		RoleDefinitionID: azurehelper.RoleStorageBlobDataContributor,
	})
	require.Error(t, err)

	var respErr *azcore.ResponseError
	require.ErrorAs(t, err, &respErr)
	assert.Equal(t, "AuthorizationFailed", respErr.ErrorCode)
}

// TestHasRoleAssignment_UsesAssignedToFilter pins the filter form: Azure only
// accepts `principalId eq` at subscription scope and answers 400 at resource
// scope, so a regression here would break every non-subscription lookup.
func TestHasRoleAssignment_UsesAssignedToFilter(t *testing.T) {
	t.Parallel()

	tr := &recordingTransport{status: http.StatusOK, body: jsonBody(map[string]any{"value": []any{}})}

	c, err := azurehelper.NewRBACClient(cfgWithTransport(tr))
	require.NoError(t, err)

	has, err := c.HasRoleAssignment(t.Context(), testScope, testPrincipalID, azurehelper.RoleStorageBlobDataContributor)
	require.NoError(t, err)
	assert.False(t, has)

	require.NotEmpty(t, tr.urls)
	assert.Contains(t, tr.urls[0], "assignedTo('"+testPrincipalID+"')")
	assert.NotContains(t, tr.urls[0], "principalId")
}

func TestHasRoleAssignment_MatchesBySuffix(t *testing.T) {
	t.Parallel()

	tt := []struct {
		name     string
		roleDef  string
		expected bool
	}{
		{
			name:     "same role",
			roleDef:  "/subscriptions/x/providers/Microsoft.Authorization/roleDefinitions/" + azurehelper.RoleStorageBlobDataContributor,
			expected: true,
		},
		{
			name:     "different role",
			roleDef:  "/subscriptions/x/providers/Microsoft.Authorization/roleDefinitions/" + azurehelper.RoleStorageBlobDataReader,
			expected: false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
				"value": []any{
					map[string]any{
						"id": "/ra/1",
						"properties": map[string]any{
							"principalId":      testPrincipalID,
							"roleDefinitionId": tc.roleDef,
						},
					},
				},
			})}

			c, err := azurehelper.NewRBACClient(cfgWithTransport(tr))
			require.NoError(t, err)

			has, err := c.HasRoleAssignment(t.Context(), testScope, testPrincipalID, azurehelper.RoleStorageBlobDataContributor)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, has)
		})
	}
}

// TestAssignRoleIfMissing_SkipsCreateWhenPresent proves the pre-check short
// circuits, so a rerun needs only read permission on role assignments.
func TestAssignRoleIfMissing_SkipsCreateWhenPresent(t *testing.T) {
	t.Parallel()

	tr := &recordingTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"value": []any{
			map[string]any{
				"id": "/ra/1",
				"properties": map[string]any{
					"principalId":      testPrincipalID,
					"roleDefinitionId": "/subscriptions/x/providers/Microsoft.Authorization/roleDefinitions/" + azurehelper.RoleStorageBlobDataContributor,
				},
			},
		},
	})}

	c, err := azurehelper.NewRBACClient(cfgWithTransport(tr))
	require.NoError(t, err)

	require.NoError(t, c.AssignRoleIfMissing(t.Context(), log.New(), azurehelper.AssignRoleInput{
		Scope:            testScope,
		PrincipalID:      testPrincipalID,
		RoleDefinitionID: azurehelper.RoleStorageBlobDataContributor,
	}))

	for _, m := range tr.methods {
		assert.NotEqual(t, http.MethodPut, m, "an existing assignment must not be re-created")
	}
}

func TestRemoveRole_MissingAssignmentIsNoop(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{"value": []any{}})}

	c, err := azurehelper.NewRBACClient(cfgWithTransport(tr))
	require.NoError(t, err)

	require.NoError(t, c.RemoveRole(t.Context(), log.New(), testScope, testPrincipalID, azurehelper.RoleStorageBlobDataContributor))
}

// TestRemoveRole_IgnoresInheritedAssignments pins that assignedTo() rows for a
// different principal (for example a group membership expansion) are not deleted.
func TestRemoveRole_IgnoresInheritedAssignments(t *testing.T) {
	t.Parallel()

	otherPrincipal := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	tr := &recordingTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"value": []any{
			map[string]any{
				"id": "/ra/group",
				"properties": map[string]any{
					"principalId":      otherPrincipal,
					"roleDefinitionId": "/subscriptions/x/providers/Microsoft.Authorization/roleDefinitions/" + azurehelper.RoleStorageBlobDataContributor,
				},
			},
		},
	})}

	c, err := azurehelper.NewRBACClient(cfgWithTransport(tr))
	require.NoError(t, err)

	require.NoError(t, c.RemoveRole(t.Context(), log.New(), testScope, testPrincipalID, azurehelper.RoleStorageBlobDataContributor))

	for _, m := range tr.methods {
		assert.NotEqual(t, http.MethodDelete, m, "inherited assignments must not be deleted")
	}
}

func TestHasRoleAssignment_BoundsPages(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"value":    []any{},
		"nextLink": "https://management.azure.com/next",
	})}
	c, err := azurehelper.NewRBACClient(cfgWithTransport(tr))
	require.NoError(t, err)

	_, err = c.HasRoleAssignment(t.Context(), testScope, testPrincipalID, azurehelper.RoleStorageBlobDataContributor)

	var tooMany *azurehelper.TooManyRoleAssignmentPagesError
	require.ErrorAs(t, err, &tooMany)
	assert.Equal(t, testScope, tooMany.Scope)
	assert.Equal(t, 100, tooMany.MaxPages)
}

func TestStorageAccountScope(t *testing.T) {
	t.Parallel()

	assert.Equal(t, testScope, azurehelper.StorageAccountScope(testSub, "rg", testAccount))
}

// TestResolvePrincipal reads the caller from its own token rather than Microsoft Graph, which directory policy often denies.
func TestResolvePrincipal(t *testing.T) {
	t.Parallel()

	// Azure rejects an assignment whose declared type does not match the principal.
	tt := []struct {
		name     string
		idtyp    string
		wantType string
	}{
		{name: "app-only token is a service principal", idtyp: "app", wantType: azurehelper.PrincipalTypeServicePrincipal},
		{name: "signed-in human is a user", idtyp: "user", wantType: azurehelper.PrincipalTypeUser},
		{name: "absent idtyp defaults to user", idtyp: "", wantType: azurehelper.PrincipalTypeUser},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			claims := map[string]any{"oid": testPrincipalID}
			if tc.idtyp != "" {
				claims["idtyp"] = tc.idtyp
			}

			cfg := cfgWithTransport(&stubTransport{status: http.StatusOK, body: []byte(`{}`)})
			cfg.Credential = tokenCredential{token: jwtWithClaims(claims)}

			got, err := azurehelper.ResolvePrincipal(t.Context(), cfg)
			require.NoError(t, err)
			assert.Equal(t, testPrincipalID, got.ID)
			assert.Equal(t, tc.wantType, got.Type)
		})
	}
}

// TestResolvePrincipal_UsesCloudARMAudience pins that sovereign clouds request
// their own ARM token audience instead of the public management.azure.com scope.
func TestResolvePrincipal_UsesCloudARMAudience(t *testing.T) {
	t.Parallel()

	tt := []struct {
		name      string
		cloudCfg  cloud.Configuration
		wantScope string
	}{
		{
			name:      "public",
			cloudCfg:  cloud.AzurePublic,
			wantScope: strings.TrimSuffix(cloud.AzurePublic.Services[cloud.ResourceManager].Audience, "/") + "/.default",
		},
		{
			name:      "us government",
			cloudCfg:  cloud.AzureGovernment,
			wantScope: strings.TrimSuffix(cloud.AzureGovernment.Services[cloud.ResourceManager].Audience, "/") + "/.default",
		},
		{
			name:      "china",
			cloudCfg:  cloud.AzureChina,
			wantScope: strings.TrimSuffix(cloud.AzureChina.Services[cloud.ResourceManager].Audience, "/") + "/.default",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cred := &recordingCredential{token: jwtWithClaims(map[string]any{"oid": testPrincipalID})}
			cfg := cfgWithTransport(&stubTransport{status: http.StatusOK, body: []byte(`{}`)})
			cfg.CloudConfig = tc.cloudCfg
			cfg.ClientOptions.Cloud = tc.cloudCfg
			cfg.Credential = cred

			got, err := azurehelper.ResolvePrincipal(t.Context(), cfg)
			require.NoError(t, err)
			assert.Equal(t, testPrincipalID, got.ID)
			require.Equal(t, []string{tc.wantScope}, cred.scopes)
		})
	}
}

// TestAssignRole_OmitsUnknownPrincipalType pins that an unset type is left out so Azure infers it instead of answering UnmatchedPrincipalType.
func TestAssignRole_OmitsUnknownPrincipalType(t *testing.T) {
	t.Parallel()

	tr := &recordingTransport{status: http.StatusCreated, body: jsonBody(map[string]any{"id": "/ra/1"})}

	c, err := azurehelper.NewRBACClient(cfgWithTransport(tr))
	require.NoError(t, err)

	require.NoError(t, c.AssignRole(t.Context(), log.New(), azurehelper.AssignRoleInput{
		Scope:            testScope,
		PrincipalID:      testPrincipalID,
		RoleDefinitionID: azurehelper.RoleStorageBlobDataContributor,
	}))

	require.NotEmpty(t, tr.bodies)
	assert.NotContains(t, tr.bodies[0], "principalType")
}

func TestResolvePrincipal_Failures(t *testing.T) {
	t.Parallel()

	tt := []struct {
		name  string
		token string
	}{
		{name: "not a jwt", token: "opaque-token"},
		{name: "payload not base64", token: "a.!!!.c"},
		{name: "payload not json", token: "a." + base64.RawURLEncoding.EncodeToString([]byte("nope")) + ".c"},
		{name: "no oid claim", token: jwtWithClaims(map[string]any{"appid": "x"})},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := cfgWithTransport(&stubTransport{status: http.StatusOK, body: []byte(`{}`)})
			cfg.Credential = tokenCredential{token: tc.token}

			_, err := azurehelper.ResolvePrincipal(t.Context(), cfg)
			require.ErrorIs(t, err, azurehelper.ErrPrincipalIDUnresolved)
		})
	}
}

func TestResolvePrincipal_RequiresTokenCredential(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.ResolvePrincipal(t.Context(), &azurehelper.AzureConfig{Method: azurehelper.AuthMethodAccessKey})
	require.Error(t, err)

	var unsupported *azurehelper.UnsupportedAuthForOpError
	require.ErrorAs(t, err, &unsupported)
	assert.Equal(t, azurehelper.AuthMethodAccessKey, unsupported.Method)

	_, err = azurehelper.ResolvePrincipal(t.Context(), nil)
	require.ErrorIs(t, err, azurehelper.ErrAzureConfigRequired)
}

// TestResolvePrincipal_ARMScope pins where the token scope comes from: the
// client options cloud stands in for an empty CloudConfig, and a config with
// no Resource Manager audience at all is rejected before any token request.
func TestResolvePrincipal_ARMScope(t *testing.T) {
	t.Parallel()

	cred := &recordingCredential{token: jwtWithClaims(map[string]any{"oid": testPrincipalID})}

	got, err := azurehelper.ResolvePrincipal(t.Context(), &azurehelper.AzureConfig{
		Credential:    cred,
		ClientOptions: policy.ClientOptions{Cloud: cloud.AzureChina},
	})
	require.NoError(t, err)
	assert.Equal(t, testPrincipalID, got.ID)
	assert.Equal(t,
		[]string{strings.TrimSuffix(cloud.AzureChina.Services[cloud.ResourceManager].Audience, "/") + "/.default"},
		cred.scopes,
	)

	_, err = azurehelper.ResolvePrincipal(t.Context(), &azurehelper.AzureConfig{Credential: tokenCredential{}})
	require.ErrorIs(t, err, azurehelper.ErrARMAudienceRequired)
}

func TestResolvePrincipal_TokenFailure(t *testing.T) {
	t.Parallel()

	cfg := cfgWithTransport(&stubTransport{status: http.StatusOK, body: []byte(`{}`)})
	cfg.Credential = tokenCredential{err: errors.New("AADSTS70043: refresh token expired")}

	_, err := azurehelper.ResolvePrincipal(t.Context(), cfg)
	require.ErrorContains(t, err, "acquiring token to resolve principal id")
	require.ErrorContains(t, err, "AADSTS70043")
}

// TestAssignRole_SendsDeclaredPrincipalType pins that a known type reaches the
// request, since Azure needs it to assign a role to a just-created principal.
func TestAssignRole_SendsDeclaredPrincipalType(t *testing.T) {
	t.Parallel()

	tr := &recordingTransport{status: http.StatusCreated, body: jsonBody(map[string]any{"id": "/ra/1"})}

	c, err := azurehelper.NewRBACClient(cfgWithTransport(tr))
	require.NoError(t, err)

	require.NoError(t, c.AssignRole(t.Context(), log.New(), azurehelper.AssignRoleInput{
		Scope:            testScope,
		PrincipalID:      testPrincipalID,
		PrincipalType:    azurehelper.PrincipalTypeServicePrincipal,
		RoleDefinitionID: azurehelper.RoleStorageBlobDataContributor,
	}))

	require.NotEmpty(t, tr.bodies)
	assert.Contains(t, tr.bodies[0], `"principalType":"ServicePrincipal"`)
}

// TestRBACClient_RejectsInvalidInput verifies the read and remove paths
// validate their arguments the same way AssignRole does.
func TestRBACClient_RejectsInvalidInput(t *testing.T) {
	t.Parallel()

	c, err := azurehelper.NewRBACClient(cfgWithTransport(&stubTransport{status: http.StatusOK, body: []byte(`{}`)}))
	require.NoError(t, err)

	_, err = c.HasRoleAssignment(t.Context(), "", testPrincipalID, azurehelper.RoleStorageBlobDataContributor)
	require.ErrorIs(t, err, azurehelper.ErrScopePrincipalRoleArgs)

	err = c.AssignRoleIfMissing(t.Context(), log.New(), azurehelper.AssignRoleInput{Scope: testScope})
	require.ErrorIs(t, err, azurehelper.ErrScopePrincipalRoleArgs)

	err = c.RemoveRole(t.Context(), log.New(), testScope, "not-a-uuid", azurehelper.RoleStorageBlobDataContributor)

	var principalErr *azurehelper.InvalidPrincipalIDError
	require.ErrorAs(t, err, &principalErr)
}

// TestRBACClient_ListFailures verifies a failed role-assignment list is
// surfaced, and in AssignRoleIfMissing stops before any write is attempted.
func TestRBACClient_ListFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		call func(ctx context.Context, c *azurehelper.RBACClient) error
		name string
		want string
	}{
		{
			name: "has role assignment",
			call: func(ctx context.Context, c *azurehelper.RBACClient) error {
				_, err := c.HasRoleAssignment(ctx, testScope, testPrincipalID, azurehelper.RoleStorageBlobDataContributor)
				return err
			},
			want: "listing role assignments",
		},
		{
			name: "assign role if missing",
			call: func(ctx context.Context, c *azurehelper.RBACClient) error {
				return c.AssignRoleIfMissing(ctx, log.New(), azurehelper.AssignRoleInput{
					Scope:            testScope,
					PrincipalID:      testPrincipalID,
					RoleDefinitionID: azurehelper.RoleStorageBlobDataContributor,
				})
			},
			want: "listing role assignments",
		},
		{
			name: "remove role",
			call: func(ctx context.Context, c *azurehelper.RBACClient) error {
				return c.RemoveRole(ctx, log.New(), testScope, testPrincipalID, azurehelper.RoleStorageBlobDataContributor)
			},
			want: "listing role assignments for removal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tr := &recordingTransport{status: http.StatusForbidden, body: jsonBody(map[string]any{
				"error": map[string]any{"code": "AuthorizationFailed", "message": "no permission"},
			})}

			c, err := azurehelper.NewRBACClient(cfgWithTransport(tr))
			require.NoError(t, err)

			err = tc.call(t.Context(), c)
			require.ErrorContains(t, err, tc.want)

			var respErr *azcore.ResponseError
			require.ErrorAs(t, err, &respErr)
			assert.Equal(t, "AuthorizationFailed", respErr.ErrorCode)

			assert.NotContains(t, tr.methods, http.MethodPut, "a failed read must not be followed by a write")
		})
	}
}

// TestHasRoleAssignment_SkipsMalformedAndForeignRows verifies rows missing
// their properties, role, or principal, and rows for another principal, never
// count as a match.
func TestHasRoleAssignment_SkipsMalformedAndForeignRows(t *testing.T) {
	t.Parallel()

	contributor := testRoleDefinitions + azurehelper.RoleStorageBlobDataContributor

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"value": []any{
			nil,
			map[string]any{"id": "/ra/no-properties"},
			map[string]any{"id": "/ra/no-role", "properties": map[string]any{"principalId": testPrincipalID}},
			map[string]any{"id": "/ra/no-principal", "properties": map[string]any{"roleDefinitionId": contributor}},
			map[string]any{"id": "/ra/other", "properties": map[string]any{
				"principalId":      "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
				"roleDefinitionId": contributor,
			}},
		},
	})}

	c, err := azurehelper.NewRBACClient(cfgWithTransport(tr))
	require.NoError(t, err)

	has, err := c.HasRoleAssignment(t.Context(), testScope, testPrincipalID, azurehelper.RoleStorageBlobDataContributor)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestAssignRoleIfMissing_CreatesWhenAbsent(t *testing.T) {
	t.Parallel()

	rt := &routeTransport{routes: []stubRoute{
		{method: http.MethodGet, status: http.StatusOK, body: string(jsonBody(map[string]any{"value": []any{}}))},
		{method: http.MethodPut, status: http.StatusCreated, body: string(jsonBody(map[string]any{"id": "/ra/new"}))},
	}}

	c, err := azurehelper.NewRBACClient(cfgWithTransport(rt))
	require.NoError(t, err)

	require.NoError(t, c.AssignRoleIfMissing(t.Context(), log.New(), azurehelper.AssignRoleInput{
		Scope:            testScope,
		PrincipalID:      testPrincipalID,
		RoleDefinitionID: azurehelper.RoleStorageBlobDataContributor,
	}))

	listIdx := rt.firstIndexOf(http.MethodGet, "roleAssignments")
	createIdx := rt.firstIndexOf(http.MethodPut, "roleAssignments")

	require.GreaterOrEqual(t, listIdx, 0, "existing assignments must be checked")
	require.GreaterOrEqual(t, createIdx, 0, "a missing assignment must be created")
	assert.Less(t, listIdx, createIdx, "the check must precede the create")
}

func TestRemoveRole_BoundsPages(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"value":    []any{},
		"nextLink": "https://management.azure.com/next",
	})}

	c, err := azurehelper.NewRBACClient(cfgWithTransport(tr))
	require.NoError(t, err)

	err = c.RemoveRole(t.Context(), log.New(), testScope, testPrincipalID, azurehelper.RoleStorageBlobDataContributor)

	var tooMany *azurehelper.TooManyRoleAssignmentPagesError
	require.ErrorAs(t, err, &tooMany)
	assert.Equal(t, testScope, tooMany.Scope)
}

// TestRemoveRole_DeletesMatchingAssignments covers every row outcome in one
// listing: malformed rows and other roles are skipped, a concurrent removal
// (404) counts as success, and a real delete failure is reported by id after
// the remaining rows are still attempted.
func TestRemoveRole_DeletesMatchingAssignments(t *testing.T) {
	t.Parallel()

	assignment := func(id, role string) map[string]any {
		return map[string]any{
			"id": "/subscriptions/" + testSub + "/providers/Microsoft.Authorization/roleAssignments/" + id,
			"properties": map[string]any{
				"principalId":      testPrincipalID,
				"roleDefinitionId": testRoleDefinitions + role,
			},
		}
	}

	listing := jsonBody(map[string]any{"value": []any{
		nil,
		map[string]any{"properties": map[string]any{"principalId": testPrincipalID}},
		assignment("ra-reader", azurehelper.RoleStorageBlobDataReader),
		assignment("ra-ok", azurehelper.RoleStorageBlobDataContributor),
		assignment("ra-gone", azurehelper.RoleStorageBlobDataContributor),
		assignment("ra-denied", azurehelper.RoleStorageBlobDataContributor),
	}})

	tests := []struct {
		name    string
		deny    bool
		wantErr bool
	}{
		{name: "all deletes succeed"},
		{name: "one delete is denied", deny: true, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			deniedStatus, deniedCode := http.StatusOK, ""
			if tc.deny {
				deniedStatus, deniedCode = http.StatusForbidden, "AuthorizationFailed"
			}

			rt := &routeTransport{routes: []stubRoute{
				{method: http.MethodGet, status: http.StatusOK, body: string(listing)},
				{method: http.MethodDelete, pathSub: "ra-ok", status: http.StatusOK, body: "{}"},
				{method: http.MethodDelete, pathSub: "ra-gone", status: http.StatusNotFound, code: "RoleAssignmentNotFound"},
				{method: http.MethodDelete, pathSub: "ra-denied", status: deniedStatus, code: deniedCode, body: "{}"},
			}}

			c, err := azurehelper.NewRBACClient(cfgWithTransport(rt))
			require.NoError(t, err)

			err = c.RemoveRole(t.Context(), log.New(), testScope, testPrincipalID, azurehelper.RoleStorageBlobDataContributor)
			if tc.wantErr {
				require.ErrorContains(t, err, "deleting role assignment")
				require.ErrorContains(t, err, "ra-denied")
			} else {
				require.NoError(t, err)
			}

			for _, id := range []string{"ra-ok", "ra-gone", "ra-denied"} {
				assert.True(t, rt.sawMethodOnPath(http.MethodDelete, id), "matching assignment %s must be deleted", id)
			}

			assert.False(t, rt.sawMethodOnPath(http.MethodDelete, "ra-reader"), "an assignment for another role must survive")
		})
	}
}

// tokenCredential returns a caller-supplied token, or err when set, so tests
// can drive the claim parsing and token failures directly.
type tokenCredential struct {
	err   error
	token string
}

func (c tokenCredential) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	if c.err != nil {
		return azcore.AccessToken{}, c.err
	}

	return azcore.AccessToken{Token: c.token, ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// recordingCredential captures the scopes ResolvePrincipal requested so tests
// can assert sovereign-cloud audiences.
type recordingCredential struct {
	token  string
	scopes []string
}

func (c *recordingCredential) GetToken(_ context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	c.scopes = append([]string(nil), opts.Scopes...)

	return azcore.AccessToken{Token: c.token, ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// recordingTransport captures the request line of every call so tests can
// assert on the query Azure actually receives.
type recordingTransport struct {
	body    []byte
	urls    []string
	methods []string
	bodies  []string
	status  int
}

func (r *recordingTransport) Do(req *http.Request) (*http.Response, error) {
	r.urls = append(r.urls, req.URL.String())
	r.methods = append(r.methods, req.Method)

	if req.Body != nil {
		if b, err := io.ReadAll(req.Body); err == nil {
			r.bodies = append(r.bodies, string(b))
		}
	}

	return &http.Response{
		Request:    req,
		StatusCode: r.status,
		Status:     http.StatusText(r.status),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(r.body))),
	}, nil
}

// jwtWithClaims builds an unsigned JWT whose payload carries claims; only the
// payload segment is ever read.
func jwtWithClaims(claims map[string]any) string {
	payload, err := json.Marshal(claims)
	if err != nil {
		panic(err)
	}

	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}
