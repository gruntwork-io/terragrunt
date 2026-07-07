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

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func TestNewStorageAccountClient_Validation(t *testing.T) {
	t.Parallel()

	if _, err := azurehelper.NewStorageAccountClient(nil); err == nil {
		t.Fatal("expected error for nil config")
	}

	if _, err := azurehelper.NewStorageAccountClient(&azurehelper.AzureConfig{
		Credential: fakeCredential{}, ResourceGroup: "rg", AccountName: testAccount,
	}); err == nil {
		t.Fatal("expected error for missing subscription")
	}

	if _, err := azurehelper.NewStorageAccountClient(&azurehelper.AzureConfig{
		SubscriptionID: testSub, ResourceGroup: "rg", AccountName: testAccount,
	}); err == nil {
		t.Fatal("expected error for missing credential")
	}

	if _, err := azurehelper.NewStorageAccountClient(&azurehelper.AzureConfig{
		SubscriptionID: testSub, Credential: fakeCredential{}, ResourceGroup: "rg",
	}); err == nil {
		t.Fatal("expected error for missing account name")
	}

	if _, err := azurehelper.NewStorageAccountClient(&azurehelper.AzureConfig{
		SubscriptionID: testSub, Credential: fakeCredential{}, AccountName: testAccount,
	}); err == nil {
		t.Fatal("expected error for missing resource group")
	}
}

func TestStorageAccount_Exists_True(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"name":     testAccount,
		"location": "eastus",
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("NewStorageAccountClient: %v", err)
	}

	exists, err := sc.Exists(t.Context())
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if !exists {
		t.Error("Exists = false, want true")
	}
}

func TestStorageAccount_Exists_False(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusNotFound, body: jsonBody(map[string]any{
		"error": map[string]string{"code": "ResourceNotFound", "message": "not found"},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("NewStorageAccountClient: %v", err)
	}

	exists, err := sc.Exists(t.Context())
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if exists {
		t.Error("Exists = true, want false for 404")
	}
}

func TestStorageAccount_GetKeys(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: listKeysBody(
		"key1", "first-key==",
		"key2", "second-key==",
	)}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("NewStorageAccountClient: %v", err)
	}

	keys, err := sc.GetKeys(t.Context())
	if err != nil {
		t.Fatalf("GetKeys: %v", err)
	}

	if len(keys) != 2 || keys[0] != "first-key==" || keys[1] != "second-key==" {
		t.Errorf("GetKeys = %v", keys)
	}
}

func TestStorageAccount_GetKeys_EmptyError(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"keys": []map[string]string{},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if _, err := sc.GetKeys(t.Context()); err == nil {
		t.Fatal("expected error when no keys returned")
	}
}

func TestStorageAccount_Delete_NotFoundIsNoop(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusNotFound, body: jsonBody(map[string]any{
		"error": map[string]string{"code": "ResourceNotFound", "message": "gone"},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := sc.EnsureDeleted(t.Context(), log.New()); err != nil {
		t.Errorf("Delete on missing account should be no-op, got %v", err)
	}
}

func TestStorageAccount_Create_RequiresLocation(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := sc.Create(t.Context(), log.New(), &azurehelper.StorageAccountConfig{}); err == nil {
		t.Fatal("expected error when Location is empty")
	}
}

func TestStorageAccount_Create_NameMismatch(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	err = sc.Create(t.Context(), log.New(), &azurehelper.StorageAccountConfig{
		Name:     "different-name",
		Location: "eastus",
	})
	if err == nil {
		t.Fatal("expected error when Name does not match client")
	}
}

func TestStorageAccount_Create_NilConfig(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := sc.Create(t.Context(), log.New(), nil); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestStorageAccount_Create_RejectsUnknownAccessTier(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	err = sc.Create(t.Context(), log.New(), &azurehelper.StorageAccountConfig{
		Name:       testAccount,
		Location:   "eastus",
		AccessTier: "Frozen",
	})
	if err == nil {
		t.Fatal("expected error for unknown access tier")
	}

	if !strings.Contains(err.Error(), "access tier") {
		t.Errorf("error %q should mention access tier", err)
	}
}

func TestStorageAccount_GetKeys_FiltersEmptyValues(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: listKeysBody(
		"key1", "",
		"key2", "second-key==",
	)}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	keys, err := sc.GetKeys(t.Context())
	if err != nil {
		t.Fatalf("GetKeys: %v", err)
	}

	if len(keys) != 1 || keys[0] != "second-key==" {
		t.Errorf("GetKeys = %v; want [second-key==]", keys)
	}
}

func TestStorageAccount_EnableVersioning(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"properties": map[string]any{"isVersioningEnabled": false},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := sc.EnableVersioning(t.Context(), log.New()); err != nil {
		t.Fatalf("EnableVersioning: %v", err)
	}

	body := tr.lastPutBody()
	if !strings.Contains(body, `"isVersioningEnabled":true`) {
		t.Errorf("PUT body %q must enable versioning", body)
	}
}

func TestStorageAccount_IsVersioningEnabled(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"properties": map[string]any{"isVersioningEnabled": true},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	on, err := sc.IsVersioningEnabled(t.Context())
	if err != nil {
		t.Fatalf("IsVersioningEnabled: %v", err)
	}

	if !on {
		t.Errorf("IsVersioningEnabled = false, want true")
	}
}

func TestStorageAccount_EnableSoftDelete_ClampsOutOfRange(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := sc.EnableSoftDelete(t.Context(), log.New(), 99999); err != nil {
		t.Fatalf("EnableSoftDelete: %v", err)
	}

	if body := tr.lastPutBody(); !strings.Contains(body, `"days":7`) {
		t.Errorf("PUT body %q must carry the clamped default days", body)
	}

	if err := sc.EnableSoftDelete(t.Context(), log.New(), 30); err != nil {
		t.Fatalf("EnableSoftDelete in-range: %v", err)
	}

	if body := tr.lastPutBody(); !strings.Contains(body, `"days":30`) {
		t.Errorf("PUT body %q must carry the requested days", body)
	}
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
