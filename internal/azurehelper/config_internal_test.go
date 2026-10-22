package azurehelper

import (
	"context"
	"errors"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func TestCredentialUnavailableOnError_ContinuesCredentialChain(t *testing.T) {
	t.Parallel()

	want := azcore.AccessToken{Token: "fallback-token"}
	chain, err := azidentity.NewChainedTokenCredential(
		[]azcore.TokenCredential{
			credentialUnavailableOnError{credential: &tokenCredential{err: errors.New("authentication failed")}},
			&tokenCredential{token: want},
		},
		nil,
	)
	require.NoError(t, err)

	got, err := chain.GetToken(t.Context(), policy.TokenRequestOptions{Scopes: []string{"scope"}})
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestManagedIdentityID(t *testing.T) {
	t.Parallel()

	// A resource id wins over a client id when both are set.
	both := managedIdentityID(&AzureSessionConfig{
		MSIResourceID: "/subscriptions/s/resourceGroups/rg/providers/x/id",
		ClientID:      "client-id",
	})
	rid, ok := both.(azidentity.ResourceID)
	assert.True(t, ok, "want ResourceID, got %T", both)
	assert.Equal(t, "/subscriptions/s/resourceGroups/rg/providers/x/id", string(rid))

	// A client id alone selects a user-assigned identity by client id.
	cid, ok := managedIdentityID(&AzureSessionConfig{ClientID: "client-id"}).(azidentity.ClientID)
	assert.True(t, ok, "want ClientID for client-id-only config")
	assert.Equal(t, "client-id", string(cid))

	// Neither set falls back to the system-assigned identity.
	assert.Nil(t, managedIdentityID(&AzureSessionConfig{}))
}

func TestCredentialUnavailableOnError_PassesTokenThrough(t *testing.T) {
	t.Parallel()

	want := azcore.AccessToken{Token: "cli-token"}
	got, err := credentialUnavailableOnError{credential: &tokenCredential{token: want}}.
		GetToken(t.Context(), policy.TokenRequestOptions{Scopes: []string{"scope"}})
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestChainedCredential(t *testing.T) {
	t.Parallel()

	defaultCred := &tokenCredential{token: azcore.AccessToken{Token: "default"}}

	// A usable CLI credential is placed in front of the default chain.
	_, isChain := chainedCredential(defaultCred, "", log.New()).(*azidentity.ChainedTokenCredential)
	assert.True(t, isChain, "the CLI credential must be chained ahead of the default credential")

	// An invalid tenant makes the CLI credential unavailable, so the default
	// credential is used on its own.
	assert.Same(t, defaultCred, chainedCredential(defaultCred, "bad tenant!", log.New()))

	// A chain the SDK refuses to build (it rejects a nil source) also falls
	// back to the default credential rather than failing.
	assert.Nil(t, chainedCredential(nil, "", log.New()))
}

// TestValidate_ServicePrincipalRequiresAllFields covers the guard Build cannot
// reach on its own, since Build only selects service principal auth when all
// three fields are already set.
func TestValidate_ServicePrincipalRequiresAllFields(t *testing.T) {
	t.Parallel()

	out := &AzureConfig{Method: AuthMethodServicePrincipal}

	require.ErrorContains(t, validate(out, &AzureSessionConfig{TenantID: "tid", ClientID: "cid"}), "client_secret")
	require.NoError(t, validate(out, &AzureSessionConfig{TenantID: "tid", ClientID: "cid", ClientSecret: "sec"}))
}

type tokenCredential struct {
	err   error
	token azcore.AccessToken
}

func (c *tokenCredential) GetToken(
	_ context.Context,
	_ policy.TokenRequestOptions,
) (azcore.AccessToken, error) {
	return c.token, c.err
}
