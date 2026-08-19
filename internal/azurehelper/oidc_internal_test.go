//go:build azure

package azurehelper

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The assertion provider is tested directly rather than through
// ClientAssertionCredential: the SDK performs Entra instance discovery over the
// network before it ever invokes the assertion callback, so driving it through
// the credential would make these tests depend on reaching login.microsoft.com.

func TestOIDCAssertionProvider_GitHubActions(t *testing.T) {
	t.Parallel()

	var gotURL, gotAuth string

	v := oidcTestVenv(map[string]string{
		"ACTIONS_ID_TOKEN_REQUEST_URL":   "https://pipelines.example/token",
		"ACTIONS_ID_TOKEN_REQUEST_TOKEN": "runner-token",
	}, func(_ context.Context, req *http.Request) (*http.Response, error) {
		gotURL = req.URL.String()
		gotAuth = req.Header.Get("Authorization")

		return oidcJSON(http.StatusOK, `{"value":"assertion-jwt"}`), nil
	})

	getAssertion := oidcAssertionProvider(v)
	require.NotNil(t, getAssertion, "a request url must select the assertion flow")

	assertion, err := getAssertion(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "assertion-jwt", assertion)

	assert.Contains(t, gotURL, "audience=api%3A%2F%2FAzureADTokenExchange",
		"Entra requires the token exchange audience")
	assert.Equal(t, "Bearer runner-token", gotAuth)
}

func TestOIDCAssertionProvider_AzureDevOps(t *testing.T) {
	t.Parallel()

	v := oidcTestVenv(map[string]string{
		"SYSTEM_OIDCREQUESTURI": "https://dev.azure.example/oidc",
		"SYSTEM_ACCESSTOKEN":    "ado-token",
	}, func(_ context.Context, _ *http.Request) (*http.Response, error) {
		// Azure DevOps returns the assertion under a different field.
		return oidcJSON(http.StatusOK, `{"oidcToken":"assertion-jwt"}`), nil
	})

	getAssertion := oidcAssertionProvider(v)
	require.NotNil(t, getAssertion)

	assertion, err := getAssertion(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "assertion-jwt", assertion)
}

func TestOIDCAssertionProvider_MissingRequestToken(t *testing.T) {
	t.Parallel()

	v := oidcTestVenv(map[string]string{
		"ACTIONS_ID_TOKEN_REQUEST_URL": "https://pipelines.example/token",
	}, func(_ context.Context, _ *http.Request) (*http.Response, error) {
		t.Fatal("no request must be made without a bearer token")

		return nil, nil
	})

	_, err := oidcAssertionProvider(v)(t.Context())

	var missing *OIDCRequestTokenMissingError
	require.ErrorAs(t, err, &missing)
	assert.Contains(t, err.Error(), "id-token", "the error must say how to fix the pipeline")
}

func TestOIDCAssertionProvider_EndpointFailure(t *testing.T) {
	t.Parallel()

	v := oidcTestVenv(map[string]string{
		"ACTIONS_ID_TOKEN_REQUEST_URL":   "https://pipelines.example/token",
		"ACTIONS_ID_TOKEN_REQUEST_TOKEN": "runner-token",
	}, func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return oidcJSON(http.StatusForbidden, `denied`), nil
	})

	_, err := oidcAssertionProvider(v)(t.Context())

	var failed *OIDCTokenRequestFailedError
	require.ErrorAs(t, err, &failed)
	assert.Equal(t, http.StatusForbidden, failed.StatusCode)
}

func TestOIDCAssertionProvider_MissingField(t *testing.T) {
	t.Parallel()

	v := oidcTestVenv(map[string]string{
		"ACTIONS_ID_TOKEN_REQUEST_URL":   "https://pipelines.example/token",
		"ACTIONS_ID_TOKEN_REQUEST_TOKEN": "runner-token",
	}, func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return oidcJSON(http.StatusOK, `{"unexpected":"shape"}`), nil
	})

	_, err := oidcAssertionProvider(v)(t.Context())

	var missing *OIDCTokenFieldMissingError
	require.ErrorAs(t, err, &missing)
	assert.Equal(t, "value", missing.Field)
}

// TestOIDCAssertionProvider_NoRequestURL verifies the token-file (workload
// identity) path is left alone when no request url is configured.
func TestOIDCAssertionProvider_NoRequestURL(t *testing.T) {
	t.Parallel()

	v := oidcTestVenv(map[string]string{}, func(_ context.Context, _ *http.Request) (*http.Response, error) {
		t.Fatal("no request must be made when no request url is configured")

		return nil, nil
	})

	assert.Nil(t, oidcAssertionProvider(v))
}

// TestApplyEnvFallbacks_RequestURLImpliesOIDC verifies a federated token
// request url selects the OIDC tier the way a token file does. CI injects
// these without ARM_USE_OIDC, so without it the flows would be unreachable.
func TestApplyEnvFallbacks_RequestURLImpliesOIDC(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"ACTIONS_ID_TOKEN_REQUEST_URL", "SYSTEM_OIDCREQUESTURI", "ARM_OIDC_REQUEST_URL"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			v := oidcTestVenv(map[string]string{key: "https://example/token"}, nil)

			cfg := &AzureSessionConfig{}
			applyEnvFallbacks(v.Env, cfg)

			assert.True(t, util.Deref(cfg.UseOIDC), "%s must select the OIDC tier", key)
		})
	}
}

func oidcTestVenv(env map[string]string, h vhttp.Handler) *venv.Venv {
	v := &venv.Venv{}
	if h != nil {
		v.HTTP = vhttp.NewMemClient(h)
	}

	return v.WithEnv(env)
}

func oidcJSON(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// TestApplyEnvFallbacks_ExplicitFalseWins verifies an explicitly configured
// false is never overridden by an ARM_USE_* environment variable. Go's zero
// value cannot express this, which is why the flags are pointers: a user who
// wrote `use_msi = false` has said what they want, and a stray variable on the
// runner must not silently switch the auth method.
func TestApplyEnvFallbacks_ExplicitFalseWins(t *testing.T) {
	t.Parallel()

	v := oidcTestVenv(map[string]string{
		"ARM_USE_MSI":     "true",
		"ARM_USE_OIDC":    "true",
		"ARM_USE_AZUREAD": "true",
	}, nil)

	cfg := &AzureSessionConfig{
		UseMSI:         new(false),
		UseOIDC:        new(false),
		UseAzureADAuth: new(false),
	}
	applyEnvFallbacks(v.Env, cfg)

	assert.False(t, util.Deref(cfg.UseMSI), "an explicit use_msi = false must win over ARM_USE_MSI")
	assert.False(t, util.Deref(cfg.UseOIDC), "an explicit use_oidc = false must win over ARM_USE_OIDC")
	assert.False(t, util.Deref(cfg.UseAzureADAuth), "an explicit use_azuread_auth = false must win over ARM_USE_AZUREAD")

	// An UNSET flag is still enabled by the environment.
	unset := &AzureSessionConfig{}
	applyEnvFallbacks(v.Env, unset)
	assert.True(t, util.Deref(unset.UseMSI), "an unset flag must still honor ARM_USE_MSI")
}
