package azurehelper

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAssertionFile = "/var/run/secrets/azure/tokens/azure-identity-token"

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
		require.Fail(t, "no request must be made without a bearer token")

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
		require.Fail(t, "no request must be made when no request url is configured")

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

func TestFetchOIDCAssertion_Failures(t *testing.T) {
	t.Parallel()

	const tokenURL = "https://pipelines.example/token"

	tests := []struct {
		handler vhttp.Handler
		name    string
		url     string
		want    string
		nilCtx  bool
	}{
		{
			name: "unparsable request url",
			url:  "https://pipelines.example/\x7f",
			want: "parsing OIDC request url",
		},
		{
			// A nil context is the one input http.NewRequestWithContext rejects
			// once the url has already parsed.
			name:   "request cannot be built",
			url:    tokenURL,
			nilCtx: true,
			want:   "building OIDC token request",
		},
		{
			name: "transport failure",
			url:  tokenURL,
			handler: func(_ context.Context, _ *http.Request) (*http.Response, error) {
				return nil, errors.New("connection refused")
			},
			want: "requesting OIDC token",
		},
		{
			name: "unreadable response body",
			url:  tokenURL,
			handler: func(_ context.Context, _ *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(failingReader{})}, nil
			},
			want: "reading OIDC token response",
		},
		{
			name: "response is not json",
			url:  tokenURL,
			handler: func(_ context.Context, _ *http.Request) (*http.Response, error) {
				return oidcJSON(http.StatusOK, "not json"), nil
			},
			want: "decoding OIDC token response",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := tc.handler
			if h == nil {
				h = func(_ context.Context, _ *http.Request) (*http.Response, error) {
					return nil, errors.New("no request must be made")
				}
			}

			ctx := t.Context()
			if tc.nilCtx {
				ctx = nil
			}

			_, err := fetchOIDCAssertion(ctx, oidcTestVenv(map[string]string{}, h), tc.url, "runner-token", "value")
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestOIDCAssertionSource_TokenWinsOverRequestURL(t *testing.T) {
	t.Parallel()

	v := oidcTestVenv(map[string]string{
		"ACTIONS_ID_TOKEN_REQUEST_URL":   "https://pipelines.example/token",
		"ACTIONS_ID_TOKEN_REQUEST_TOKEN": "runner-token",
	}, func(_ context.Context, _ *http.Request) (*http.Response, error) {
		require.Fail(t, "no request must be made when a token is set")

		return nil, nil
	})

	getAssertion := oidcAssertionSource(logger.CreateLogger(), v, &AzureSessionConfig{OIDCToken: "assertion-jwt"})
	require.NotNil(t, getAssertion)

	assertion, err := getAssertion(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "assertion-jwt", assertion)
}

func TestOIDCAssertionSource_TokenFileWinsOverRequestURL(t *testing.T) {
	t.Parallel()

	v := oidcTestVenv(map[string]string{
		"ACTIONS_ID_TOKEN_REQUEST_URL":   "https://pipelines.example/token",
		"ACTIONS_ID_TOKEN_REQUEST_TOKEN": "runner-token",
	}, nil)

	cfg := &AzureSessionConfig{OIDCTokenFilePath: testAssertionFile}

	assert.Nil(t, oidcAssertionSource(logger.CreateLogger(), v, cfg))
}

func TestOIDCAssertionSource_RequestURLWithoutTokenOrTokenFile(t *testing.T) {
	t.Parallel()

	v := oidcTestVenv(map[string]string{
		"ACTIONS_ID_TOKEN_REQUEST_URL":   "https://pipelines.example/token",
		"ACTIONS_ID_TOKEN_REQUEST_TOKEN": "runner-token",
	}, func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return oidcJSON(http.StatusOK, `{"value":"assertion-jwt"}`), nil
	})

	getAssertion := oidcAssertionSource(logger.CreateLogger(), v, &AzureSessionConfig{})
	require.NotNil(t, getAssertion)

	assertion, err := getAssertion(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "assertion-jwt", assertion)
}

func TestOIDCAssertionSource_TokenMustMatchTokenFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check    func(t *testing.T, assertion string, err error)
		name     string
		contents string
		write    bool
	}{
		{
			name:     "same value",
			contents: "assertion-jwt",
			write:    true,
			check: func(t *testing.T, assertion string, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.Equal(t, "assertion-jwt", assertion)
			},
		},
		{
			name:     "different value",
			contents: "other-jwt",
			write:    true,
			check: func(t *testing.T, _ string, err error) {
				t.Helper()

				var mismatch *OIDCTokenMismatchError
				require.ErrorAs(t, err, &mismatch)
				assert.Equal(t, testAssertionFile, mismatch.TokenFilePath)
			},
		},
		{
			name: "missing token file",
			check: func(t *testing.T, _ string, err error) {
				t.Helper()

				require.ErrorIs(t, err, fs.ErrNotExist)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewMemMapFS()
			if tc.write {
				require.NoError(t, vfs.WriteFile(fsys, testAssertionFile, []byte(tc.contents), 0o600))
			}

			v := oidcTestVenv(map[string]string{}, nil).WithFS(fsys)
			cfg := &AzureSessionConfig{OIDCToken: "assertion-jwt", OIDCTokenFilePath: testAssertionFile}

			getAssertion := oidcAssertionSource(logger.CreateLogger(), v, cfg)
			require.NotNil(t, getAssertion)

			assertion, err := getAssertion(t.Context())
			tc.check(t, assertion, err)
		})
	}
}

func TestOIDCAssertionSource_AzureDevOpsServiceConnection(t *testing.T) {
	t.Parallel()

	var got *http.Request

	v := oidcTestVenv(map[string]string{
		"SYSTEM_OIDCREQUESTURI":          "https://dev.azure.example/oidc",
		"ARM_OIDC_REQUEST_TOKEN":         "request-token",
		"SYSTEM_ACCESSTOKEN":             "ado-token",
		"ACTIONS_ID_TOKEN_REQUEST_URL":   "https://pipelines.example/token",
		"ACTIONS_ID_TOKEN_REQUEST_TOKEN": "runner-token",
	}, func(_ context.Context, req *http.Request) (*http.Response, error) {
		got = req

		return oidcJSON(http.StatusOK, `{"oidcToken":"assertion-jwt"}`), nil
	})

	cfg := &AzureSessionConfig{
		ADOServiceConnectionID: "service-connection",
		OIDCToken:              "token-jwt",
		OIDCTokenFilePath:      testAssertionFile,
	}

	getAssertion := oidcAssertionSource(logger.CreateLogger(), v, cfg)
	require.NotNil(t, getAssertion)

	assertion, err := getAssertion(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "assertion-jwt", assertion)

	require.NotNil(t, got)
	assert.Equal(t, http.MethodPost, got.Method)
	assert.Equal(t, "dev.azure.example", got.URL.Host)
	assert.Equal(t, "7.1", got.URL.Query().Get("api-version"))
	assert.Equal(t, "service-connection", got.URL.Query().Get("serviceConnectionId"))
	assert.Equal(t, "Bearer request-token", got.Header.Get("Authorization"))
	assert.Equal(t, "Suppress", got.Header.Get("X-Tfs-Fedauthredirect"))
}

func TestOIDCAssertionSource_AzureDevOpsServiceConnectionFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check  func(t *testing.T, err error)
		name   string
		body   string
		status int
	}{
		{
			name:   "rejected request token",
			status: http.StatusUnauthorized,
			body:   "denied",
			check: func(t *testing.T, err error) {
				t.Helper()

				var failed *OIDCTokenRequestFailedError
				require.ErrorAs(t, err, &failed)
				assert.Equal(t, http.StatusUnauthorized, failed.StatusCode)
			},
		},
		{
			name:   "response without the token field",
			status: http.StatusOK,
			body:   `{"value":"assertion-jwt"}`,
			check: func(t *testing.T, err error) {
				t.Helper()

				var missing *OIDCTokenFieldMissingError
				require.ErrorAs(t, err, &missing)
				assert.Equal(t, "oidcToken", missing.Field)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := oidcTestVenv(map[string]string{
				"SYSTEM_OIDCREQUESTURI": "https://dev.azure.example/oidc",
				"SYSTEM_ACCESSTOKEN":    "ado-token",
			}, func(_ context.Context, _ *http.Request) (*http.Response, error) {
				return oidcJSON(tc.status, tc.body), nil
			})

			cfg := &AzureSessionConfig{ADOServiceConnectionID: "service-connection"}

			getAssertion := oidcAssertionSource(logger.CreateLogger(), v, cfg)
			require.NotNil(t, getAssertion)

			_, err := getAssertion(t.Context())
			tc.check(t, err)
		})
	}
}

func TestOIDCAssertionSource_IncompleteServiceConnectionUsesToken(t *testing.T) {
	t.Parallel()

	v := oidcTestVenv(map[string]string{
		"SYSTEM_OIDCREQUESTURI": "https://dev.azure.example/oidc",
	}, func(_ context.Context, _ *http.Request) (*http.Response, error) {
		require.Fail(t, "no request must be made without a request token")

		return nil, nil
	})

	cfg := &AzureSessionConfig{ADOServiceConnectionID: "service-connection", OIDCToken: "assertion-jwt"}

	getAssertion := oidcAssertionSource(logger.CreateLogger(), v, cfg)
	require.NotNil(t, getAssertion)

	assertion, err := getAssertion(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "assertion-jwt", assertion)
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

// failingReader fails every read, standing in for a connection dropped
// mid-body.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("connection reset")
}
