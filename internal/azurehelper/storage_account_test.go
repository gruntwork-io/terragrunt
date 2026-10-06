package azurehelper_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
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

func TestNewStorageAccountClient_Validation(t *testing.T) {
	t.Parallel()

	// A nil config or an empty account name is guaranteed by config validation
	// upstream, so reaching here is a caller bug and panics.
	assert.Panics(t, func() { _, _ = azurehelper.NewStorageAccountClient(nil) })

	assert.Panics(t, func() {
		_, _ = azurehelper.NewStorageAccountClient(&azurehelper.AzureConfig{
			SubscriptionID: testSub, Credential: fakeCredential{}, ResourceGroup: "rg",
		})
	})

	// subscription_id, the auth method, and resource_group_name are user
	// supplied, so a missing value is a user error.
	_, err := azurehelper.NewStorageAccountClient(&azurehelper.AzureConfig{
		Credential: fakeCredential{}, ResourceGroup: "rg", AccountName: testAccount,
	})
	require.ErrorIs(t, err, azurehelper.ErrSubscriptionIDRequired)

	_, err = azurehelper.NewStorageAccountClient(&azurehelper.AzureConfig{
		SubscriptionID: testSub, ResourceGroup: "rg", AccountName: testAccount,
	})

	var unsupported *azurehelper.UnsupportedAuthForOpError
	require.ErrorAs(t, err, &unsupported)

	_, err = azurehelper.NewStorageAccountClient(&azurehelper.AzureConfig{
		SubscriptionID: testSub, Credential: fakeCredential{}, AccountName: testAccount,
	})
	require.ErrorIs(t, err, azurehelper.ErrResourceGroupNameRequired)
}

func TestStorageAccount_Exists_True(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"name":     testAccount,
		"location": "eastus",
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "NewStorageAccountClient")

	exists, err := sc.Exists(t.Context())
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestStorageAccount_Exists_False(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusNotFound, body: jsonBody(map[string]any{
		"error": map[string]string{"code": "ResourceNotFound", "message": "not found"},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "NewStorageAccountClient")

	exists, err := sc.Exists(t.Context())
	require.NoError(t, err)
	assert.False(t, exists, "404 must report the account as absent")
}

func TestStorageAccount_GetKeys(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: listKeysBody(
		"key1", "first-key==",
		"key2", "second-key==",
	)}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "NewStorageAccountClient")

	keys, err := sc.GetKeys(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"first-key==", "second-key=="}, keys)
}

func TestStorageAccount_GetKeys_EmptyError(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"keys": []map[string]string{},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	_, err = sc.GetKeys(t.Context())
	require.ErrorIs(t, err, azurehelper.ErrNoAccessKeysReturned)
}

func TestStorageAccount_Delete_NotFoundIsNoop(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusNotFound, body: jsonBody(map[string]any{
		"error": map[string]string{"code": "ResourceNotFound", "message": "gone"},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	require.NoError(
		t,
		sc.EnsureDeleted(t.Context(), log.New()),
		"delete on a missing account must be a no-op",
	)
}

func TestStorageAccount_Create_RequiresLocation(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	// location is user supplied, so a missing value is a user error.
	require.ErrorIs(
		t,
		sc.Create(t.Context(), log.New(), &azurehelper.StorageAccountConfig{}),
		azurehelper.ErrLocationRequired,
	)
}

func TestStorageAccount_Create_NameMismatch(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	assert.Panics(t, func() {
		_ = sc.Create(t.Context(), log.New(), &azurehelper.StorageAccountConfig{
			Name:     "different-name",
			Location: "eastus",
		})
	})
}

func TestStorageAccount_Create_NilConfig(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	assert.Panics(t, func() { _ = sc.Create(t.Context(), log.New(), nil) })
}

func TestStorageAccount_Create_RejectsUnknownAccessTier(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	// access_tier is user supplied, so an unknown value is a user error.
	err = sc.Create(t.Context(), log.New(), &azurehelper.StorageAccountConfig{
		Name:       testAccount,
		Location:   "eastus",
		AccessTier: "Frozen",
	})

	var unknownTier *azurehelper.UnknownAccessTierError
	require.ErrorAs(t, err, &unknownTier)
	assert.Equal(t, "Frozen", unknownTier.Tier)
}

func TestStorageAccount_Create_RejectsUnknownMinimumTLSVersion(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	// minimum_tls_version is user supplied, so an unknown value is a user error.
	// TLS1_1 is deprecated on Azure and is not one of the accepted values.
	err = sc.Create(t.Context(), log.New(), &azurehelper.StorageAccountConfig{
		Name:              testAccount,
		Location:          "eastus",
		MinimumTLSVersion: "TLS1_1",
	})

	var unknownTLS *azurehelper.UnknownMinimumTLSVersionError
	require.ErrorAs(t, err, &unknownTLS)
	assert.Equal(t, "TLS1_1", unknownTLS.Version)
}

func TestStorageAccount_Create_DefaultsMinimumTLSVersion(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"properties": map[string]any{"provisioningState": "Succeeded"},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	// An unset minimum_tls_version must default to TLS1_2, not Azure's implicit
	// TLS1_0.
	require.NoError(t, sc.Create(t.Context(), log.New(), &azurehelper.StorageAccountConfig{
		Name:     testAccount,
		Location: "eastus",
	}))
	assert.Contains(t, tr.lastPutBody(), `"minimumTlsVersion":"TLS1_2"`, "default minimum TLS version must reach the request")
}

func TestStorageAccount_Create_SetsMinimumTLSVersion(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"properties": map[string]any{"provisioningState": "Succeeded"},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	require.NoError(t, sc.Create(t.Context(), log.New(), &azurehelper.StorageAccountConfig{
		Name:              testAccount,
		Location:          "eastus",
		MinimumTLSVersion: "TLS1_3",
	}))
	assert.Contains(t, tr.lastPutBody(), `"minimumTlsVersion":"TLS1_3"`, "configured minimum TLS version must reach the request")
}

func TestStorageAccount_Create_SetsExplicitTLS12(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"properties": map[string]any{"provisioningState": "Succeeded"},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	// An explicit TLS1_2 must reach the request, same as the default path.
	require.NoError(t, sc.Create(t.Context(), log.New(), &azurehelper.StorageAccountConfig{
		Name:              testAccount,
		Location:          "eastus",
		MinimumTLSVersion: "TLS1_2",
	}))
	assert.Contains(t, tr.lastPutBody(), `"minimumTlsVersion":"TLS1_2"`, "explicit minimum TLS version must reach the request")
}

func TestStorageAccount_GetKeys_FiltersEmptyValues(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: listKeysBody(
		"key1", "",
		"key2", "second-key==",
	)}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	keys, err := sc.GetKeys(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"second-key=="}, keys)
}

func TestStorageAccount_EnableVersioning(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"properties": map[string]any{"isVersioningEnabled": false},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	require.NoError(t, sc.EnableVersioning(t.Context(), log.New()))
	assert.Contains(
		t,
		tr.lastPutBody(),
		`"isVersioningEnabled":true`,
		"PUT body must enable versioning",
	)
}

func TestStorageAccount_IsVersioningEnabled(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"properties": map[string]any{"isVersioningEnabled": true},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	on, err := sc.IsVersioningEnabled(t.Context())
	require.NoError(t, err)
	assert.True(t, on)
}

func TestStorageAccount_SoftDeleteRetention(t *testing.T) {
	t.Parallel()

	// Enabled policies must surface their day counts so drift detection can
	// compare them against the desired retention.
	on := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"properties": map[string]any{
			"deleteRetentionPolicy":          map[string]any{"enabled": true, "days": 30},
			"containerDeleteRetentionPolicy": map[string]any{"enabled": true, "days": 30},
		},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(on))
	require.NoError(t, err, "setup")

	blobDays, containerDays, err := sc.SoftDeleteRetention(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int32(30), blobDays)
	assert.Equal(t, int32(30), containerDays)

	// A disabled (or absent) policy reports 0 days, i.e. soft delete is off.
	off := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"properties": map[string]any{
			"deleteRetentionPolicy": map[string]any{"enabled": false},
		},
	})}

	sc, err = azurehelper.NewStorageAccountClient(cfgWithTransport(off))
	require.NoError(t, err, "setup")

	blobDays, containerDays, err = sc.SoftDeleteRetention(t.Context())
	require.NoError(t, err, "soft delete off")
	assert.Zero(t, blobDays, "a disabled policy reports no retention")
	assert.Zero(t, containerDays, "a disabled policy reports no retention")
}

func TestStorageAccount_EnableSoftDelete_ClampsOutOfRange(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	require.NoError(t, sc.EnableSoftDelete(t.Context(), log.New(), 99999))
	assert.Contains(
		t,
		tr.lastPutBody(),
		`"days":7`,
		"out-of-range retention must clamp to the default",
	)

	require.NoError(t, sc.EnableSoftDelete(t.Context(), log.New(), 30), "in-range retention")

	assert.Contains(t, tr.lastPutBody(), `"days":30`, "in-range retention must reach the request")
}

func TestFindResourceGroupForAccount_BoundsPages(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"value":    []any{},
		"nextLink": "https://management.azure.com/next",
	})}

	_, err := azurehelper.FindResourceGroupForAccount(
		t.Context(),
		cfgWithTransport(tr),
		testAccount,
	)

	var tooMany *azurehelper.TooManyStorageAccountPagesError
	require.ErrorAs(t, err, &tooMany)
	assert.Equal(t, testAccount, tooMany.Account)
	assert.Equal(t, testSub, tooMany.SubscriptionID)
	assert.Equal(t, 100, tooMany.MaxPages)
}

// TestARMClients_RejectCloudWithoutResourceManager verifies every ARM client
// constructor surfaces the SDK's refusal of a cloud that has no Resource
// Manager endpoint, rather than silently targeting the public cloud.
func TestARMClients_RejectCloudWithoutResourceManager(t *testing.T) {
	t.Parallel()

	tests := []struct {
		build func(cfg *azurehelper.AzureConfig) error
		name  string
		want  string
	}{
		{
			name: "rbac",
			build: func(cfg *azurehelper.AzureConfig) error {
				_, err := azurehelper.NewRBACClient(cfg)
				return err
			},
			want: "creating armauthorization client factory",
		},
		{
			name: "resource group",
			build: func(cfg *azurehelper.AzureConfig) error {
				_, err := azurehelper.NewResourceGroupClient(cfg)
				return err
			},
			want: "creating resource groups client",
		},
		{
			name: "storage account",
			build: func(cfg *azurehelper.AzureConfig) error {
				_, err := azurehelper.NewStorageAccountClient(cfg)
				return err
			},
			want: "creating armstorage client factory",
		},
		{
			name: "resource group lookup",
			build: func(cfg *azurehelper.AzureConfig) error {
				_, err := azurehelper.FindResourceGroupForAccount(t.Context(), cfg, testAccount)
				return err
			},
			want: "creating armstorage accounts client",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := cfgWithTransport(&stubTransport{status: http.StatusOK})
			cfg.ClientOptions.Cloud = cloud.Configuration{
				ActiveDirectoryAuthorityHost: cloud.AzurePublic.ActiveDirectoryAuthorityHost,
			}

			err := tc.build(cfg)
			require.ErrorContains(t, err, tc.want)
			require.ErrorContains(t, err, "Resource Manager")
		})
	}
}

// TestStorageAccount_WrapsServiceFailures verifies every control-plane call
// surfaces an unexpected answer as a wrapped error naming the account, and
// that the Azure error code survives the wrapping.
func TestStorageAccount_WrapsServiceFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		call func(ctx context.Context, sc *azurehelper.StorageAccountClient) error
		name string
		want string
	}{
		{
			name: "exists",
			call: func(ctx context.Context, sc *azurehelper.StorageAccountClient) error {
				_, err := sc.Exists(ctx)
				return err
			},
			want: `get storage account "acct"`,
		},
		{
			name: "create",
			call: func(ctx context.Context, sc *azurehelper.StorageAccountClient) error {
				return sc.Create(ctx, log.New(), &azurehelper.StorageAccountConfig{Location: "eastus"})
			},
			want: `begin create storage account "acct"`,
		},
		{
			name: "ensure deleted",
			call: func(ctx context.Context, sc *azurehelper.StorageAccountClient) error {
				return sc.EnsureDeleted(ctx, log.New())
			},
			want: `delete storage account "acct"`,
		},
		{
			name: "enable versioning",
			call: func(ctx context.Context, sc *azurehelper.StorageAccountClient) error {
				return sc.EnableVersioning(ctx, log.New())
			},
			want: `get blob service properties for "acct"`,
		},
		{
			name: "is versioning enabled",
			call: func(ctx context.Context, sc *azurehelper.StorageAccountClient) error {
				_, err := sc.IsVersioningEnabled(ctx)
				return err
			},
			want: `get blob service properties for "acct"`,
		},
		{
			name: "soft delete retention",
			call: func(ctx context.Context, sc *azurehelper.StorageAccountClient) error {
				_, _, err := sc.SoftDeleteRetention(ctx)
				return err
			},
			want: `get blob service properties for "acct"`,
		},
		{
			name: "get keys",
			call: func(ctx context.Context, sc *azurehelper.StorageAccountClient) error {
				_, err := sc.GetKeys(ctx)
				return err
			},
			want: `list keys for "acct"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// 403 is terminal for the SDK retry policy, so each call fails fast.
			tr := &stubTransport{status: http.StatusForbidden, body: jsonBody(map[string]any{
				"error": map[string]string{"code": "AuthorizationFailed", "message": "no permission"},
			})}

			sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
			require.NoError(t, err, "setup")

			err = tc.call(t.Context(), sc)
			require.ErrorContains(t, err, tc.want)

			var respErr *azcore.ResponseError
			require.ErrorAs(t, err, &respErr)
			assert.Equal(t, "AuthorizationFailed", respErr.ErrorCode)
		})
	}
}

func TestStorageAccount_Create_FailedProvisioning(t *testing.T) {
	t.Parallel()

	// ARM accepts the request but the operation ends in a Failed state.
	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"properties": map[string]any{"provisioningState": "Failed"},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	err = sc.Create(t.Context(), log.New(), &azurehelper.StorageAccountConfig{Location: "eastus"})
	require.ErrorContains(t, err, `create storage account "acct"`)
	assert.NotContains(t, err.Error(), "begin create", "the failure must come from the poll, not the request")
}

func TestStorageAccount_Create_ResourceGroupMismatch(t *testing.T) {
	t.Parallel()

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(&stubTransport{status: http.StatusOK}))
	require.NoError(t, err, "setup")

	// The client is bound to one resource group, so a different one is a caller bug.
	assert.Panics(t, func() {
		_ = sc.Create(t.Context(), log.New(), &azurehelper.StorageAccountConfig{
			ResourceGroupName: "other-rg",
			Location:          "eastus",
		})
	})
}

func TestStorageAccount_Create_SetsAccessTier(t *testing.T) {
	t.Parallel()

	for _, tier := range []string{"Hot", "Cool", "Cold", "Premium"} {
		t.Run(tier, func(t *testing.T) {
			t.Parallel()

			tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
				"properties": map[string]any{"provisioningState": "Succeeded"},
			})}

			sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
			require.NoError(t, err, "setup")

			require.NoError(t, sc.Create(t.Context(), log.New(), &azurehelper.StorageAccountConfig{
				Location:   "eastus",
				AccessTier: tier,
			}))
			assert.Contains(t, tr.lastPutBody(), `"accessTier":"`+tier+`"`, "configured access tier must reach the request")
		})
	}
}

func TestStorageAccount_EnsureDeleted(t *testing.T) {
	t.Parallel()

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(&stubTransport{status: http.StatusOK}))
	require.NoError(t, err, "setup")

	require.NoError(t, sc.EnsureDeleted(t.Context(), log.New()))
}

// TestStorageAccount_UpdateBlobServiceProperties_WriteFailure verifies a
// failed write after a successful read is reported as the write that failed.
func TestStorageAccount_UpdateBlobServiceProperties_WriteFailure(t *testing.T) {
	t.Parallel()

	rt := &routeTransport{routes: []stubRoute{
		{method: http.MethodGet, status: http.StatusOK, body: "{}"},
		{method: http.MethodPut, status: http.StatusForbidden, code: "AuthorizationFailed"},
	}}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(rt))
	require.NoError(t, err, "setup")

	require.ErrorContains(t, sc.EnableVersioning(t.Context(), log.New()), `set blob service properties for "acct"`)
}

// TestStorageAccount_BlobServicePropertiesAbsent verifies a blob service with
// no properties reads as versioning off and soft delete off.
func TestStorageAccount_BlobServicePropertiesAbsent(t *testing.T) {
	t.Parallel()

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(&stubTransport{status: http.StatusOK, body: []byte("{}")}))
	require.NoError(t, err, "setup")

	on, err := sc.IsVersioningEnabled(t.Context())
	require.NoError(t, err)
	assert.False(t, on)

	blobDays, containerDays, err := sc.SoftDeleteRetention(t.Context())
	require.NoError(t, err)
	assert.Zero(t, blobDays)
	assert.Zero(t, containerDays)
}

func TestStorageAccount_GetKeys_AllEmpty(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: listKeysBody("key1", "", "key2", "")}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	_, err = sc.GetKeys(t.Context())
	require.ErrorIs(t, err, azurehelper.ErrAllAccessKeysEmpty)
}

func TestFindResourceGroupForAccount_RequiresARMConfig(t *testing.T) {
	t.Parallel()

	// The account name comes from validated config, so an empty one is a caller bug.
	assert.Panics(t, func() {
		_, _ = azurehelper.FindResourceGroupForAccount(t.Context(), cfgWithTransport(&stubTransport{}), "")
	})

	noSub := cfgWithTransport(&stubTransport{})
	noSub.SubscriptionID = ""

	_, err := azurehelper.FindResourceGroupForAccount(t.Context(), noSub, testAccount)
	require.ErrorIs(t, err, azurehelper.ErrSubscriptionIDRequired)

	_, err = azurehelper.FindResourceGroupForAccount(t.Context(), &azurehelper.AzureConfig{
		SubscriptionID: testSub,
		Method:         azurehelper.AuthMethodSasToken,
	}, testAccount)

	var unsupported *azurehelper.UnsupportedAuthForOpError
	require.ErrorAs(t, err, &unsupported)
	assert.Equal(t, "resource group lookup", unsupported.Operation)
}

func TestFindResourceGroupForAccount(t *testing.T) {
	t.Parallel()

	accountID := func(group string) string {
		return "/subscriptions/" + testSub + group + "/providers/Microsoft.Storage/storageAccounts/" + testAccount
	}

	tests := []struct {
		check    func(t *testing.T, group string, err error)
		name     string
		accounts []any
	}{
		{
			name: "found",
			accounts: []any{
				map[string]any{"name": "other", "id": accountID("/resourceGroups/other-rg")},
				map[string]any{"name": testAccount, "id": accountID("/resourceGroups/state-rg")},
			},
			check: func(t *testing.T, group string, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "state-rg", group)
			},
		},
		{
			// ARM is inconsistent about the segment's case.
			name:     "found with lowercase segment",
			accounts: []any{map[string]any{"name": testAccount, "id": accountID("/resourcegroups/state-rg")}},
			check: func(t *testing.T, group string, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "state-rg", group)
			},
		},
		{
			name:     "id without a resource group",
			accounts: []any{map[string]any{"name": testAccount, "id": accountID("")}},
			check: func(t *testing.T, _ string, err error) {
				t.Helper()

				var unparsable *azurehelper.UnparsableResourceIDError
				require.ErrorAs(t, err, &unparsable)
				assert.Equal(t, accountID(""), unparsable.ID)
			},
		},
		{
			name:     "id ending at the resource group segment",
			accounts: []any{map[string]any{"name": testAccount, "id": "/subscriptions/" + testSub + "/resourceGroups"}},
			check: func(t *testing.T, _ string, err error) {
				t.Helper()

				var unparsable *azurehelper.UnparsableResourceIDError
				require.ErrorAs(t, err, &unparsable)
			},
		},
		{
			name:     "not found",
			accounts: []any{map[string]any{"name": "other", "id": accountID("/resourceGroups/other-rg")}},
			check: func(t *testing.T, _ string, err error) {
				t.Helper()

				var notFound *azurehelper.StorageAccountNotFoundError
				require.ErrorAs(t, err, &notFound)
				assert.Equal(t, testAccount, notFound.Account)
				assert.Equal(t, testSub, notFound.SubscriptionID)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{"value": tc.accounts})}

			group, err := azurehelper.FindResourceGroupForAccount(t.Context(), cfgWithTransport(tr), testAccount)
			tc.check(t, group, err)
		})
	}
}

func TestFindResourceGroupForAccount_ListFailure(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusForbidden, body: jsonBody(map[string]any{
		"error": map[string]string{"code": "AuthorizationFailed", "message": "no permission"},
	})}

	_, err := azurehelper.FindResourceGroupForAccount(t.Context(), cfgWithTransport(tr), testAccount)
	require.ErrorContains(t, err, `listing storage accounts in subscription "sub"`)
}

// jsonBody marshals body to JSON, panicking on error since test inputs are literals.
func jsonBody(body any) []byte {
	b, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}

	return b
}

// listKeysBody builds the ListKeys JSON payload from (name, value) pairs.
func listKeysBody(pairs ...string) []byte {
	keys := make([]map[string]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		keys = append(keys, map[string]string{"keyName": pairs[i], "value": pairs[i+1]})
	}

	return jsonBody(map[string]any{"keys": keys})
}

// stubTransport answers every request with one canned status and body while
// recording PUT request bodies for content assertions.
type stubTransport struct {
	body      []byte
	putBodies []string
	mu        sync.Mutex
	status    int
}

func (s *stubTransport) Do(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodPut && req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err == nil {
			s.mu.Lock()
			s.putBodies = append(s.putBodies, string(b))
			s.mu.Unlock()
		}
	}

	return &http.Response{
		Request:    req,
		StatusCode: s.status,
		Status:     http.StatusText(s.status),
		Body:       io.NopCloser(strings.NewReader(string(s.body))),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

// lastPutBody returns the most recent recorded PUT body, empty when none.
func (s *stubTransport) lastPutBody() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.putBodies) == 0 {
		return ""
	}

	return s.putBodies[len(s.putBodies)-1]
}

// fakeCredential satisfies azcore.TokenCredential without contacting AAD.
type fakeCredential struct{}

func (fakeCredential) GetToken(
	_ context.Context,
	_ policy.TokenRequestOptions,
) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "fake", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func cfgWithTransport(tr policy.Transporter) *azurehelper.AzureConfig {
	return &azurehelper.AzureConfig{
		Credential:     fakeCredential{},
		SubscriptionID: testSub,
		ResourceGroup:  "rg",
		AccountName:    testAccount,
		CloudConfig:    cloud.AzurePublic,
		ClientOptions:  policy.ClientOptions{Transport: tr, Cloud: cloud.AzurePublic},
		Method:         azurehelper.AuthMethodAzureAD,
	}
}
