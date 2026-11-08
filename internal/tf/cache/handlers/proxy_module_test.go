package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyModuleHandlerForwardsToUpstream(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name              string
		modulesV1         string
		target            string
		restPath          string
		wantUpstreamURL   string
		wantAuthorization string
	}{
		{
			name:              "relative modules endpoint uses the registry host",
			modulesV1:         "/v1/modules/",
			target:            "/proxy?page=2",
			restPath:          "acme/vpc/aws/versions",
			wantUpstreamURL:   "https://registry.example.com/v1/modules/acme/vpc/aws/versions?page=2",
			wantAuthorization: "Bearer upstream-token",
		},
		{
			name:            "absolute modules endpoint is used as the base",
			modulesV1:       "https://modules.example.com/api/modules/",
			target:          "/proxy",
			restPath:        "acme/vpc/aws/versions",
			wantUpstreamURL: "https://modules.example.com/api/modules/acme/vpc/aws/versions",
		},
		{
			name:              "trailing slash of the rest path is kept",
			modulesV1:         "/v1/modules/",
			target:            "/proxy",
			restPath:          "acme/vpc/aws/",
			wantUpstreamURL:   "https://registry.example.com/v1/modules/acme/vpc/aws/",
			wantAuthorization: "Bearer upstream-token",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var upstream *http.Request

			c := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
				upstream = req

				return vhttp.Respond(http.StatusOK, []byte(`{"modules":[]}`), nil), nil
			})

			creds := (&cliconfig.Config{}).CredentialsSource(
				map[string]string{"TF_TOKEN_registry_example_com": "upstream-token"},
			)
			handler := handlers.NewProxyModuleHandler(
				logger.CreateLogger(),
				c,
				creds,
				fakeDiscoverer{urls: &handlers.RegistryURLs{ModulesV1: tc.modulesV1}},
				[]string{"registry.example.com"},
			)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.target, nil)
			req.Header.Set("Authorization", "Bearer cache-server-token")

			rec := httptest.NewRecorder()

			require.NoError(t, handler.Proxy(router.NewResponseWriter(rec), req, "registry.example.com", tc.restPath))

			require.NotNil(t, upstream, "the request should reach the upstream registry")
			assert.Equal(t, tc.wantUpstreamURL, upstream.URL.String())
			assert.Equal(
				t,
				tc.wantAuthorization,
				upstream.Header.Get("Authorization"),
				"upstream should get only the user credentials for its host",
			)
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.JSONEq(t, `{"modules":[]}`, rec.Body.String())
		})
	}
}

func TestProxyModuleHandlerFailures(t *testing.T) {
	t.Parallel()

	errDiscovery := errors.New("discovery failed")

	testCases := []struct {
		discoverer   fakeDiscoverer
		assertErr    func(t *testing.T, err error)
		name         string
		registryName string
	}{
		{
			name:         "registry that is not configured",
			registryName: "other.example.com",
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var httpErr *router.HTTPError

				require.ErrorAs(t, err, &httpErr)
				assert.Equal(t, http.StatusNotFound, httpErr.Code)
			},
		},
		{
			name:         "discovery fails",
			registryName: "registry.example.com",
			discoverer:   fakeDiscoverer{err: errDiscovery},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, errDiscovery)
			},
		},
		{
			name:         "modules endpoint that is not a valid url",
			registryName: "registry.example.com",
			discoverer:   fakeDiscoverer{urls: &handlers.RegistryURLs{ModulesV1: "https://bad host/modules"}},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var urlErr *url.Error

				require.ErrorAs(t, err, &urlErr)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := handlers.NewProxyModuleHandler(
				logger.CreateLogger(),
				vhttp.NewNoNetworkClient(),
				nil,
				tc.discoverer,
				[]string{"registry.example.com"},
			)

			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/proxy", nil)

			tc.assertErr(t, handler.Proxy(router.NewResponseWriter(rec), req, tc.registryName, "acme/vpc/aws/versions"))
		})
	}
}

// fakeDiscoverer answers discovery with fixed values.
type fakeDiscoverer struct {
	err  error
	urls *handlers.RegistryURLs
}

func (d fakeDiscoverer) DiscoveryURL(context.Context, string) (*handlers.RegistryURLs, error) {
	return d.urls, d.err
}
