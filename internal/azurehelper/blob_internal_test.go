//go:build azure

package azurehelper

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWithPrefix exercises the variadic-option plumbing without needing a
// live Azure storage account. Verifies that WithPrefix populates the private
// listBlobsOptions.prefix field that ListBlobs reads into
// azblobcontainer.ListBlobsFlatOptions.Prefix.
func TestWithPrefix(t *testing.T) {
	t.Parallel()

	o := &listBlobsOptions{}
	WithPrefix("state/")(o)
	assert.Equal(t, "state/", o.prefix)
}

// stubTokenCredential returns a fixed bearer token without contacting AAD,
// letting copySourceAuth tests verify header construction without network.
type stubTokenCredential struct {
	token string
}

func (s stubTokenCredential) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: s.token, ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// errTokenCredential always fails GetToken; lets the OAuth branch surface
// acquisition errors without a real credential.
type errTokenCredential struct{ err error }

func (e errTokenCredential) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{}, e.err
}

func TestCopySourceAuth_SasTokenAppendsQuery(t *testing.T) {
	t.Parallel()

	c := &BlobClient{
		accountName:    "acct",
		endpointSuffix: "core.windows.net",
		method:         AuthMethodSasToken,
		sasToken:       "?sv=2024-01-01&sig=abc",
	}

	gotURL, opts, err := c.copySourceAuth(t.Context(), "src", "k")
	require.NoError(t, err)
	assert.Nil(t, opts, "SAS-token auth should not set CopySourceAuthorization")
	assert.Equal(t, "https://acct.blob.core.windows.net/src/k?sv=2024-01-01&sig=abc", gotURL)
}

func TestCopySourceAuth_AccessKeyOmitsHeader(t *testing.T) {
	t.Parallel()

	c := &BlobClient{
		accountName:    "acct",
		endpointSuffix: "core.windows.net",
		method:         AuthMethodAccessKey,
	}

	gotURL, opts, err := c.copySourceAuth(t.Context(), "src", "k")
	require.NoError(t, err)
	assert.Nil(t, opts, "access-key auth should not set CopySourceAuthorization")
	assert.Equal(t, "https://acct.blob.core.windows.net/src/k", gotURL)
}

func TestCopySourceAuth_OAuthSetsBearerHeader(t *testing.T) {
	t.Parallel()

	methods := []AuthMethod{
		AuthMethodAzureAD,
		AuthMethodServicePrincipal,
		AuthMethodOIDC,
		AuthMethodMSI,
	}

	for _, m := range methods {
		t.Run(string(m), func(t *testing.T) {
			t.Parallel()

			c := &BlobClient{
				accountName:    "acct",
				endpointSuffix: "core.windows.net",
				method:         m,
				credential:     stubTokenCredential{token: "tok-123"},
			}

			gotURL, opts, err := c.copySourceAuth(t.Context(), "src", "k")
			require.NoError(t, err)
			require.NotNil(t, opts)
			require.NotNil(t, opts.CopySourceAuthorization)
			assert.Equal(t, "Bearer tok-123", *opts.CopySourceAuthorization)
			assert.Equal(t, "https://acct.blob.core.windows.net/src/k", gotURL)
		})
	}
}

// scopeCapturingCredential records the Scopes passed to GetToken so a test
// can assert the audience matches the cloud the BlobClient was built for.
type scopeCapturingCredential struct {
	gotScopes []string
}

func (s *scopeCapturingCredential) GetToken(_ context.Context, o policy.TokenRequestOptions) (azcore.AccessToken, error) {
	s.gotScopes = append(s.gotScopes, o.Scopes...)
	return azcore.AccessToken{Token: "tok", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func TestCopySourceAuth_OAuthScopePerCloud(t *testing.T) {
	t.Parallel()

	tests := []struct {
		suffix string
		want   string
	}{
		{"core.windows.net", "https://storage.azure.com/.default"},
		{"core.usgovcloudapi.net", "https://storage.azure.com/.default"},
		{"core.chinacloudapi.cn", "https://storage.chinacloudapi.cn/.default"},
	}

	for _, tc := range tests {
		t.Run(tc.suffix, func(t *testing.T) {
			t.Parallel()

			cred := &scopeCapturingCredential{}
			c := &BlobClient{
				accountName:    "acct",
				endpointSuffix: tc.suffix,
				method:         AuthMethodAzureAD,
				credential:     cred,
			}

			_, _, err := c.copySourceAuth(t.Context(), "src", "k")
			require.NoError(t, err)
			require.Equal(t, []string{tc.want}, cred.gotScopes)
		})
	}
}

func TestCopySourceAuth_OAuthMissingCredential(t *testing.T) {
	t.Parallel()

	c := &BlobClient{
		accountName:    "acct",
		endpointSuffix: "core.windows.net",
		method:         AuthMethodAzureAD,
	}

	_, _, err := c.copySourceAuth(t.Context(), "src", "k")

	var typedErr *CredentialMissingError
	if !errors.As(err, &typedErr) {
		t.Fatalf("expected CredentialMissingError, got %v", err)
	}
}

func TestCopySourceAuth_OAuthTokenAcquisitionError(t *testing.T) {
	t.Parallel()

	c := &BlobClient{
		accountName:    "acct",
		endpointSuffix: "core.windows.net",
		method:         AuthMethodAzureAD,
		credential:     errTokenCredential{err: errors.New("aad down")},
	}

	_, _, err := c.copySourceAuth(t.Context(), "src", "k")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aad down")
}
