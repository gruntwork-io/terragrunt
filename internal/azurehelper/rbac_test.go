//go:build azure

package azurehelper_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	testPrincipalID = "11111111-2222-3333-4444-555555555555"
	testScope       = "/subscriptions/" + testSub + "/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/" + testAccount
)

func TestNewRBACClient_Validation(t *testing.T) {
	t.Parallel()

	tt := []struct {
		cfg  *azurehelper.AzureConfig
		name string
	}{
		{name: "nil config", cfg: nil},
		{name: "missing subscription", cfg: &azurehelper.AzureConfig{Credential: fakeCredential{}}},
		{
			name: "data-plane auth cannot manage rbac",
			cfg: &azurehelper.AzureConfig{
				SubscriptionID: testSub,
				Method:         azurehelper.AuthMethodAccessKey,
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := azurehelper.NewRBACClient(tc.cfg)
			require.Error(t, err)
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
	assert.Contains(t, err.Error(), "AuthorizationFailed")
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
						"id":         "/ra/1",
						"properties": map[string]any{"roleDefinitionId": tc.roleDef},
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

	_, err = azurehelper.ResolvePrincipal(t.Context(), nil)
	require.ErrorIs(t, err, azurehelper.ErrAzureConfigRequired)
}

// tokenCredential returns a caller-supplied token so tests can drive the
// claim parsing directly.
type tokenCredential struct {
	token string
}

func (c tokenCredential) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
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
