//go:build azure

package azurehelper_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// jsonBody marshals body to JSON and panics on error. Used only by tests
// where the input is a literal map[string]any with no marshalling pitfalls.
func jsonBody(body any) []byte {
	b, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}

	return b
}

// stubTransport is a policy.Transporter that returns a fresh *http.Response
// built from a status and body each call, so the azcore pipeline owns and
// closes the body. Avoids bodyclose lint complaints from sharing a single
// pre-built *http.Response across calls.
type stubTransport struct {
	body   []byte
	status int
}

func (s *stubTransport) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{
		Request:    req,
		StatusCode: s.status,
		Status:     http.StatusText(s.status),
		Body:       io.NopCloser(strings.NewReader(string(s.body))),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
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

	exists, err := sc.Exists(context.Background())
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

	exists, err := sc.Exists(context.Background())
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if exists {
		t.Error("Exists = true, want false for 404")
	}
}

func TestStorageAccount_GetKeys(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"keys": []map[string]string{
			{"keyName": "key1", "value": "first-key=="},
			{"keyName": "key2", "value": "second-key=="},
		},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("NewStorageAccountClient: %v", err)
	}

	keys, err := sc.GetKeys(context.Background())
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

	if _, err := sc.GetKeys(context.Background()); err == nil {
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

	if err := sc.Delete(context.Background(), log.New()); err != nil {
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

	if err := sc.Create(context.Background(), log.New(), &azurehelper.StorageAccountConfig{}); err == nil {
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

	err = sc.Create(context.Background(), log.New(), &azurehelper.StorageAccountConfig{
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

	if err := sc.Create(context.Background(), log.New(), nil); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestStorageAccount_EnableVersioning(t *testing.T) {
	t.Parallel()

	tr := &stubTransport{status: http.StatusOK, body: jsonBody(map[string]any{
		"properties": map[string]any{"isVersioningEnabled": true},
	})}

	sc, err := azurehelper.NewStorageAccountClient(cfgWithTransport(tr))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := sc.EnableVersioning(context.Background(), log.New()); err != nil {
		t.Fatalf("EnableVersioning: %v", err)
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

	on, err := sc.IsVersioningEnabled(context.Background())
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

	// Out-of-range value is clamped, not rejected.
	if err := sc.EnableSoftDelete(context.Background(), log.New(), 99999); err != nil {
		t.Fatalf("EnableSoftDelete: %v", err)
	}

	// In-range value also accepted.
	if err := sc.EnableSoftDelete(context.Background(), log.New(), 30); err != nil {
		t.Fatalf("EnableSoftDelete in-range: %v", err)
	}
}
