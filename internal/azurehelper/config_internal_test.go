//go:build azure

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
