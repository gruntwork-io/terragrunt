package azurehelper_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
)

func TestIsRetryable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		name string
		want bool
	}{
		{err: nil, name: "nil", want: false},
		{err: errors.New("dial tcp: timeout"), name: "unclassified (non-response) error", want: false},
		{err: errors.New("AADSTS700016: application not found"), name: "auth failure is permanent", want: false},
		{err: context.Canceled, name: "context.Canceled", want: false},
		{err: context.DeadlineExceeded, name: "context.DeadlineExceeded", want: false},
		{err: fmt.Errorf("wrap: %w", context.Canceled), name: "wrapped context.Canceled", want: false},
		{err: respErr(http.StatusUnauthorized, ""), name: "401", want: false},
		{err: respErr(http.StatusNotFound, ""), name: "404", want: false},
		{err: respErr(http.StatusTooManyRequests, ""), name: "429", want: true},
		{err: respErr(http.StatusInternalServerError, ""), name: "500", want: true},
		{err: respErr(http.StatusServiceUnavailable, ""), name: "503", want: true},
		{err: respErr(http.StatusBadRequest, ""), name: "400", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, azurehelper.IsRetryable(tc.err))
		})
	}
}

func TestIsNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		name string
		want bool
	}{
		{err: nil, name: "nil", want: false},
		{err: errors.New("nope"), name: "non-azure", want: false},
		{err: respErr(http.StatusNotFound, ""), name: "404", want: true},
		{err: respErr(http.StatusInternalServerError, "ResourceNotFound"), name: "ResourceNotFound code", want: true},
		{err: respErr(http.StatusOK, "BlobNotFound"), name: "BlobNotFound code", want: true},
		{err: respErr(http.StatusOK, "ContainerNotFound"), name: "ContainerNotFound code", want: true},
		{err: respErr(http.StatusForbidden, ""), name: "403", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, azurehelper.IsNotFound(tc.err))
		})
	}
}

// TestErrorMessages pins the text of every typed error, since each one reaches
// users as a sentence and names the value they need to fix.
func TestErrorMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		name string
		want string
	}{
		{
			name: "invalid principal id",
			err:  &azurehelper.InvalidPrincipalIDError{PrincipalID: "p"},
			want: `principal id "p" is not a valid uuid`,
		},
		{
			name: "invalid role definition id",
			err:  &azurehelper.InvalidRoleDefinitionIDError{RoleDefinitionID: "r"},
			want: `role definition id "r" is not a valid uuid`,
		},
		{
			name: "too many role assignment pages",
			err:  &azurehelper.TooManyRoleAssignmentPagesError{Scope: "/scope", MaxPages: 100},
			want: "listing role assignments at /scope exceeded 100 pages",
		},
		{
			name: "too many blob pages",
			err:  &azurehelper.TooManyBlobPagesError{Container: "state", MaxPages: 3},
			want: "listing blobs in state exceeded 3 pages",
		},
		{
			name: "too many storage account pages",
			err:  &azurehelper.TooManyStorageAccountPagesError{Account: "acct", SubscriptionID: "sub", MaxPages: 100},
			want: `finding storage account "acct" in subscription "sub" exceeded 100 pages`,
		},
		{
			name: "oidc request token missing",
			err:  &azurehelper.OIDCRequestTokenMissingError{RequestURL: "https://token.example"},
			want: "an OIDC request url (https://token.example) was provided without a request token; " +
				"in GitHub Actions grant the job `permissions: id-token: write`, " +
				"in Azure DevOps expose SYSTEM_ACCESSTOKEN to the step",
		},
		{
			name: "oidc token request failed",
			err:  &azurehelper.OIDCTokenRequestFailedError{StatusCode: http.StatusForbidden, Body: "denied"},
			want: "OIDC token request failed with status 403: denied",
		},
		{
			name: "oidc token field missing",
			err:  &azurehelper.OIDCTokenFieldMissingError{Field: "value"},
			want: `OIDC token response did not contain a "value" value`,
		},
		{
			name: "storage account not found",
			err:  &azurehelper.StorageAccountNotFoundError{Account: "acct", SubscriptionID: "sub"},
			want: `storage account "acct" was not found in subscription "sub"`,
		},
		{
			name: "unparsable resource id",
			err:  &azurehelper.UnparsableResourceIDError{ID: "/bad"},
			want: `could not read a resource group from resource id "/bad"`,
		},
		{
			name: "credential missing",
			err:  &azurehelper.CredentialMissingError{Method: azurehelper.AuthMethodMSI},
			want: `azure config has no credential for method "msi"`,
		},
		{
			name: "unsupported auth method",
			err:  &azurehelper.UnsupportedAuthMethodError{Method: "bogus"},
			want: `unsupported azure auth method "bogus"`,
		},
		{
			name: "unsupported auth for operation",
			err:  &azurehelper.UnsupportedAuthForOpError{Method: azurehelper.AuthMethodSasToken, Operation: "RBAC operations"},
			want: `RBAC operations require a token credential (auth method "sas-token" is not supported)`,
		},
		{
			name: "missing copy blob args",
			err:  &azurehelper.MissingCopyBlobArgsError{Missing: []string{"source key", "destination key"}},
			want: "copy blob requires source key, destination key",
		},
		{
			name: "destination blob exists",
			err:  &azurehelper.DestinationBlobExistsError{Container: "state", Key: "a.tfstate"},
			want: "destination blob state/a.tfstate already exists",
		},
		{
			name: "unknown authority host",
			err:  &azurehelper.UnknownAuthorityHostError{Host: "login.example"},
			want: `unknown Azure AD authority host "login.example"; cannot derive a blob endpoint suffix`,
		},
		{
			name: "unknown cloud environment",
			err:  &azurehelper.UnknownCloudEnvironmentError{Name: "mars"},
			want: `unknown cloud environment "mars" (want one of: public, government, china)`,
		},
		{
			name: "unknown access tier",
			err:  &azurehelper.UnknownAccessTierError{Tier: "Frozen"},
			want: `unknown access tier "Frozen" (want Hot, Cool, Cold, or Premium)`,
		},
		{
			name: "unknown minimum tls version",
			err:  &azurehelper.UnknownMinimumTLSVersionError{Version: "TLS1_1"},
			want: `unknown minimum TLS version "TLS1_1" (want TLS1_2 or TLS1_3)`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.err.Error())
		})
	}
}

// respErr builds an azcore.ResponseError with the given status and error code.
func respErr(status int, code string) error {
	return &azcore.ResponseError{StatusCode: status, ErrorCode: code}
}
