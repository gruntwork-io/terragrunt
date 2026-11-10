package helpers_test

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientDoCachesResponsesPerURL(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	c := vhttp.NewMemClient(func(context.Context, *http.Request) (*http.Response, error) {
		calls.Add(1)

		return vhttp.Respond(http.StatusOK, []byte(versionsListing), nil), nil
	})

	client := helpers.NewClient(c, nil)

	for range 2 {
		var value versionsValue

		require.NoError(t, client.Do(t.Context(), http.MethodGet, "https://registry.test/versions", &value))
		assert.Len(t, value.Versions, 2, "every call should decode the listing")
	}

	assert.Equal(t, int32(1), calls.Load(), "a second call for the same url should be served from the cache")
}

func TestClientDoSendsHostCredentials(t *testing.T) {
	t.Parallel()

	var authorization atomic.Value

	c := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		authorization.Store(req.Header.Get("Authorization"))

		return vhttp.Respond(http.StatusOK, []byte(versionsListing), nil), nil
	})

	creds := (&cliconfig.Config{}).CredentialsSource(map[string]string{"TF_TOKEN_registry_test": "registry-token"})

	var value versionsValue

	require.NoError(
		t,
		helpers.NewClient(c, creds).Do(t.Context(), http.MethodGet, "https://registry.test/versions", &value),
	)
	assert.Equal(t, "Bearer registry-token", authorization.Load(), "the request should carry the token for its host")
}

func TestClientDoLeavesValueEmptyForNonOKResponse(t *testing.T) {
	t.Parallel()

	c := vhttp.NewMemClient(func(context.Context, *http.Request) (*http.Response, error) {
		return vhttp.Respond(http.StatusNotFound, []byte(versionsListing), nil), nil
	})

	var value versionsValue

	require.NoError(t, helpers.NewClient(c, nil).Do(t.Context(), http.MethodGet, "https://registry.test/versions", &value))
	assert.Empty(t, value.Versions, "a failed response should not be decoded")
}

func TestClientDoFailures(t *testing.T) {
	t.Parallel()

	cut := errors.New("connection reset")
	errClose := errors.New("close failed")

	testCases := []struct {
		respond   vhttp.Handler
		assertErr func(t *testing.T, err error)
		name      string
		reqURL    string
	}{
		{
			name:   "url that does not parse",
			reqURL: "https://registry.test/%zz",
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var urlErr *url.Error

				require.ErrorAs(t, err, &urlErr)
			},
		},
		{
			name: "transport failure",
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				return nil, cut
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, cut)
			},
		},
		{
			name: "body that is not gzip despite its encoding",
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				return vhttp.Respond(http.StatusOK, []byte(notGzip), http.Header{"Content-Encoding": []string{"gzip"}}), nil
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, gzip.ErrHeader)
			},
		},
		{
			name: "transfer cut short",
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				resp := vhttp.Respond(http.StatusOK, nil, nil)
				resp.Body = io.NopCloser(errAfter(versionsListing[:10], cut))

				return resp, nil
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, cut)
			},
		},
		{
			name: "body that is not json",
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				return vhttp.Respond(http.StatusOK, []byte("not json"), nil), nil
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var syntaxErr *json.SyntaxError

				require.ErrorAs(t, err, &syntaxErr)
			},
		},
		{
			name: "body that fails to close",
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				resp := vhttp.Respond(http.StatusOK, nil, nil)
				resp.Body = failingCloseBody{Reader: strings.NewReader(versionsListing), err: errClose}

				return resp, nil
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, errClose)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			respond := tc.respond
			if respond == nil {
				respond = func(context.Context, *http.Request) (*http.Response, error) {
					return vhttp.Respond(http.StatusOK, []byte(versionsListing), nil), nil
				}
			}

			reqURL := tc.reqURL
			if reqURL == "" {
				reqURL = "https://registry.test/versions"
			}

			var value versionsValue

			tc.assertErr(t, helpers.NewClient(vhttp.NewMemClient(respond), nil).Do(t.Context(), http.MethodGet, reqURL, &value))
		})
	}
}

// versionsValue is the decoded shape of versionsListing.
type versionsValue struct {
	Versions []struct {
		Version string `json:"version"`
	} `json:"versions"`
}

// failingCloseBody is a response body whose Close fails.
type failingCloseBody struct {
	io.Reader

	err error
}

func (b failingCloseBody) Close() error {
	return b.err
}
