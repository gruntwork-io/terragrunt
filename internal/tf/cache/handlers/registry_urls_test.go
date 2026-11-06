package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const discoveryDocument = `{"modules.v1":"/v1/modules/","providers.v1":"/v1/providers/"}`

func TestDiscoveryURLReadsWellKnownDocument(t *testing.T) {
	t.Parallel()

	var requested string

	c := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		requested = req.URL.String()

		return vhttp.Respond(http.StatusOK, []byte(discoveryDocument), nil), nil
	})

	urls, err := handlers.DiscoveryURL(t.Context(), c, "registry.example.com")
	require.NoError(t, err)

	assert.Equal(t, "https://registry.example.com/.well-known/terraform.json", requested)
	assert.Equal(t, &handlers.RegistryURLs{ModulesV1: "/v1/modules/", ProvidersV1: "/v1/providers/"}, urls)
}

func TestDiscoveryURLFailures(t *testing.T) {
	t.Parallel()

	cut := errors.New("connection reset")

	testCases := []struct {
		respond      vhttp.Handler
		assertErr    func(t *testing.T, err error)
		name         string
		registryName string
		wantOffline  bool
	}{
		{
			name:         "registry name that makes an invalid url",
			registryName: "bad host",
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var urlErr *url.Error

				require.ErrorAs(t, err, &urlErr)
			},
			wantOffline: true,
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
			wantOffline: true,
		},
		{
			name:        "registry without a discovery document",
			respond:     respondWith(http.StatusNotFound, ""),
			assertErr:   requireNotFoundWellKnown,
			wantOffline: true,
		},
		{
			name:        "registry failing to serve the discovery document",
			respond:     respondWith(http.StatusInternalServerError, ""),
			assertErr:   requireNotFoundWellKnown,
			wantOffline: true,
		},
		{
			name:    "registry refusing the request",
			respond: respondWith(http.StatusForbidden, ""),
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.Error(t, err)
			},
		},
		{
			name: "document cut short",
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				resp := vhttp.Respond(http.StatusOK, nil, nil)
				resp.Body = io.NopCloser(failingReader{err: cut})

				return resp, nil
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, cut)
			},
		},
		{
			name:    "document that is not json",
			respond: respondWith(http.StatusOK, "not json"),
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var syntaxErr *json.SyntaxError

				require.ErrorAs(t, err, &syntaxErr)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			respond := tc.respond
			if respond == nil {
				respond = respondWith(http.StatusOK, discoveryDocument)
			}

			registryName := tc.registryName
			if registryName == "" {
				registryName = "registry.example.com"
			}

			urls, err := handlers.DiscoveryURL(t.Context(), vhttp.NewMemClient(respond), registryName)

			tc.assertErr(t, err)
			assert.Nil(t, urls)
			assert.Equal(t, tc.wantOffline, handlers.IsOfflineError(err), "offline classification should match the failure")
		})
	}
}

func TestNotFoundWellKnownURLErrorNamesTheDocument(t *testing.T) {
	t.Parallel()

	_, err := handlers.DiscoveryURL(
		t.Context(),
		vhttp.NewMemClient(respondWith(http.StatusNotFound, "")),
		"registry.example.com",
	)

	var notFound handlers.NotFoundWellKnownURLError

	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, ".well-known/terraform.json not found", notFound.Error())
}

func TestRegistryURLsString(t *testing.T) {
	t.Parallel()

	urls := &handlers.RegistryURLs{ModulesV1: "/v1/modules/", ProvidersV1: "/v1/providers/"}

	assert.JSONEq(t, discoveryDocument, urls.String(), "registry urls should print as the discovery document")
}

// failingReader fails every read with err.
type failingReader struct {
	err error
}

func (r failingReader) Read([]byte) (int, error) {
	return 0, r.err
}

// respondWith returns a handler that answers every request with status and body.
func respondWith(status int, body string) vhttp.Handler {
	return func(context.Context, *http.Request) (*http.Response, error) {
		return vhttp.Respond(status, []byte(body), nil), nil
	}
}

// requireNotFoundWellKnown asserts err reports a missing discovery document.
func requireNotFoundWellKnown(t *testing.T, err error) {
	t.Helper()

	var notFound handlers.NotFoundWellKnownURLError

	require.ErrorAs(t, err, &notFound)
}
