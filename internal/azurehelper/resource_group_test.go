package azurehelper_test

import (
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func TestNewResourceGroupClient_NilConfig(t *testing.T) {
	t.Parallel()

	// Config presence is a caller invariant checked upstream, so it panics.
	assert.Panics(t, func() { _, _ = azurehelper.NewResourceGroupClient(nil) })
}

func TestNewResourceGroupClient_MissingSubscription(t *testing.T) {
	t.Parallel()

	// subscription_id is user supplied, so a missing value is a user error.
	_, err := azurehelper.NewResourceGroupClient(&azurehelper.AzureConfig{
		Method:        azurehelper.AuthMethodAzureAD,
		ClientOptions: azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})
	require.ErrorIs(t, err, azurehelper.ErrSubscriptionIDRequired)
}

func TestNewResourceGroupClient_MissingCredential(t *testing.T) {
	t.Parallel()

	// The auth method is user supplied, so an ARM-incapable one is a user error.
	_, err := azurehelper.NewResourceGroupClient(&azurehelper.AzureConfig{
		Method:         azurehelper.AuthMethodAccessKey,
		SubscriptionID: testSub,
		AccessKey:      "key",
		ClientOptions:  azcore.ClientOptions{Cloud: cloud.AzurePublic},
	})

	var unsupported *azurehelper.UnsupportedAuthForOpError
	require.ErrorAs(t, err, &unsupported)
}

func TestResourceGroup_RequiresName(t *testing.T) {
	t.Parallel()

	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{})})

	// An empty name is a caller invariant violation, so it panics rather than errors.
	assert.Panics(t, func() { _, _ = c.Exists(t.Context(), "") })
	assert.Panics(t, func() { _ = c.EnsureResourceGroup(t.Context(), log.New(), "", "eastus") })
	assert.Panics(t, func() { _ = c.EnsureDeleted(t.Context(), log.New(), "") })
}

func TestResourceGroup_Exists_True(t *testing.T) {
	t.Parallel()

	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusNoContent, body: nil})

	exists, err := c.Exists(t.Context(), "rg")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestResourceGroup_Exists_False(t *testing.T) {
	t.Parallel()

	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusNotFound, body: jsonBody(map[string]any{
		"error": map[string]any{"code": "ResourceGroupNotFound", "message": "not found"},
	})})

	exists, err := c.Exists(t.Context(), "rg")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestResourceGroup_EnsureResourceGroup_RequiresLocation(t *testing.T) {
	t.Parallel()
	// 404 -> not exists -> location validation kicks in; location is user
	// supplied, so a missing value is a user error.
	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusNotFound, body: jsonBody(map[string]any{
		"error": map[string]any{"code": "ResourceGroupNotFound"},
	})})

	require.ErrorIs(t, c.EnsureResourceGroup(t.Context(), log.New(), "rg", ""), azurehelper.ErrLocationRequiredForRG)
}

func TestResourceGroup_EnsureResourceGroup_NoopWhenExists(t *testing.T) {
	t.Parallel()
	// 204 -> exists -> CreateOrUpdate must not be called, and missing location is fine.
	c := newTestResourceGroupClient(t, &stubTransport{status: http.StatusNoContent, body: nil})

	require.NoError(t, c.EnsureResourceGroup(t.Context(), log.New(), "rg", ""))
}

func TestResourceGroup_Exists_Failures(t *testing.T) {
	t.Parallel()

	// A not-found error code is honored even when the status is not the 404
	// CheckExistence already treats as absent.
	c := newTestResourceGroupClient(t, &routeTransport{routes: []stubRoute{
		{method: http.MethodHead, status: http.StatusBadRequest, code: "ResourceNotFound"},
	}})

	exists, err := c.Exists(t.Context(), "rg")
	require.NoError(t, err)
	assert.False(t, exists)

	c = newTestResourceGroupClient(t, &routeTransport{routes: []stubRoute{
		{method: http.MethodHead, status: http.StatusForbidden, code: "AuthorizationFailed"},
	}})

	_, err = c.Exists(t.Context(), "rg")
	require.ErrorContains(t, err, "checking resource group existence")
}

func TestResourceGroup_EnsureResourceGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		want      string
		routes    []stubRoute
		wantWrite bool
	}{
		{
			name: "existence check failure is propagated",
			routes: []stubRoute{
				{method: http.MethodHead, status: http.StatusForbidden, code: "AuthorizationFailed"},
			},
			want: "checking resource group existence",
		},
		{
			name: "creates a missing group",
			routes: []stubRoute{
				{method: http.MethodHead, status: http.StatusNotFound},
				{method: http.MethodPut, status: http.StatusCreated, body: `{"location":"eastus"}`},
			},
			wantWrite: true,
		},
		{
			name: "create failure is wrapped",
			routes: []stubRoute{
				{method: http.MethodHead, status: http.StatusNotFound},
				{method: http.MethodPut, status: http.StatusForbidden, code: "AuthorizationFailed"},
			},
			want:      "creating resource group rg",
			wantWrite: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rt := &routeTransport{routes: tc.routes}
			c := newTestResourceGroupClient(t, rt)

			err := c.EnsureResourceGroup(t.Context(), log.New(), "rg", "eastus")
			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tc.wantWrite, rt.sawBodyOnPath(http.MethodPut, "/resourcegroups/rg", `{"location":"eastus"}`),
				"the group must be created in the requested location only when missing")
		})
	}
}

// TestResourceGroup_EnsureDeleted covers both halves of the long-running
// delete: the initial request and the poll that follows a 202, each of which
// treats a missing group as already deleted.
func TestResourceGroup_EnsureDeleted(t *testing.T) {
	t.Parallel()

	const pollURL = "https://management.azure.com/operationresults/rg-delete"

	accepted := stubRoute{
		method:  http.MethodDelete,
		status:  http.StatusAccepted,
		headers: map[string]string{"Location": pollURL},
	}

	tests := []struct {
		name   string
		want   string
		routes []stubRoute
	}{
		{
			name:   "deleted synchronously",
			routes: []stubRoute{{method: http.MethodDelete, status: http.StatusOK}},
		},
		{
			name:   "already gone",
			routes: []stubRoute{{method: http.MethodDelete, status: http.StatusNotFound, code: "ResourceGroupNotFound"}},
		},
		{
			name:   "delete request fails",
			routes: []stubRoute{{method: http.MethodDelete, status: http.StatusForbidden, code: "AuthorizationFailed"}},
			want:   "starting resource group delete rg",
		},
		{
			name: "gone by the time it is polled",
			routes: []stubRoute{
				accepted,
				{method: http.MethodGet, pathSub: "/operationresults/", status: http.StatusNotFound, code: "ResourceGroupNotFound"},
			},
		},
		{
			name: "poll fails",
			routes: []stubRoute{
				accepted,
				{method: http.MethodGet, pathSub: "/operationresults/", status: http.StatusForbidden, code: "AuthorizationFailed"},
			},
			want: "waiting for resource group delete rg",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := newTestResourceGroupClient(t, &routeTransport{routes: tc.routes})

			err := c.EnsureDeleted(t.Context(), log.New(), "rg")
			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)

				return
			}

			require.NoError(t, err)
		})
	}
}

func newTestResourceGroupClient(t *testing.T, tr policy.Transporter) *azurehelper.ResourceGroupClient {
	t.Helper()

	c, err := azurehelper.NewResourceGroupClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	return c
}
