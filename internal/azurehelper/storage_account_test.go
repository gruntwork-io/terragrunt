//go:build azure

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

	require.NoError(t, sc.EnsureDeleted(t.Context(), log.New()), "delete on a missing account must be a no-op")
}

func TestStorageAccount_Create_RequiresLocation(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	require.NoError(t, err, "setup")

	// location is user supplied, so a missing value is a user error.
	require.ErrorIs(t, sc.Create(t.Context(), log.New(), &azurehelper.StorageAccountConfig{}), azurehelper.ErrLocationRequired)
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
	assert.Contains(t, tr.lastPutBody(), `"isVersioningEnabled":true`, "PUT body must enable versioning")
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
	assert.Contains(t, tr.lastPutBody(), `"days":7`, "out-of-range retention must clamp to the default")

	require.NoError(t, sc.EnableSoftDelete(t.Context(), log.New(), 30), "in-range retention")

	assert.Contains(t, tr.lastPutBody(), `"days":30`, "in-range retention must reach the request")
}

func TestFindResourceGroupForAccount_BoundsPages(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"value":    []any{},
		"nextLink": "https://management.azure.com/next",
	})}

	_, err := azurehelper.FindResourceGroupForAccount(t.Context(), cfgWithTransport(tr), testAccount)

	var tooMany *azurehelper.TooManyStorageAccountPagesError
	require.ErrorAs(t, err, &tooMany)
	assert.Equal(t, testAccount, tooMany.Account)
	assert.Equal(t, testSub, tooMany.SubscriptionID)
	assert.Equal(t, 100, tooMany.MaxPages)
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

func (fakeCredential) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
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
