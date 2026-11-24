package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectProviderHandlerGetVersions(t *testing.T) {
	t.Parallel()

	c := vhttp.NewMemClient(registryRoutes(map[string]string{
		"/.well-known/terraform.json":            discoveryDocument,
		"/v1/providers/example/example/versions": `{"versions":[{"version":"1.0.0"},{"version":"1.1.0"}]}`,
	}))

	handler := newDirectHandler(c)
	assert.Equal(t, "direct", handler.String())

	versions, err := handler.GetVersions(t.Context(), mirroredProvider())
	require.NoError(t, err)

	got := make([]string, 0, len(versions))
	for _, v := range versions {
		got = append(got, v.Version)
	}

	assert.Equal(t, []string{"1.0.0", "1.1.0"}, got)
}

func TestDirectProviderHandlerGetPlatformResolvesRelativeURLs(t *testing.T) {
	t.Parallel()

	c := vhttp.NewMemClient(registryRoutes(map[string]string{
		"/.well-known/terraform.json": discoveryDocument,
		"/v1/providers/example/example/1.0.0/download/linux/amd64": `{
			"filename": "terraform-provider-example_1.0.0_linux_amd64.zip",
			"download_url": "terraform-provider-example_1.0.0_linux_amd64.zip"
		}`,
	}))

	body, err := newDirectHandler(c).GetPlatform(t.Context(), mirroredProvider())
	require.NoError(t, err)
	require.NotNil(t, body)

	assert.Equal(
		t,
		"https://registry.example.com/v1/providers/example/example/1.0.0/download/linux/terraform-provider-example_1.0.0_linux_amd64.zip",
		body.DownloadURL,
		"a relative download url should resolve against the platform url",
	)
}

func TestDirectProviderHandlerFailures(t *testing.T) {
	t.Parallel()

	cut := errors.New("connection reset")

	testCases := []struct {
		cached    *handlers.RegistryURLs
		respond   vhttp.Handler
		assertErr func(t *testing.T, err error)
		name      string
	}{
		{
			name:    "discovery refused",
			respond: respondWith(http.StatusForbidden, ""),
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.Error(t, err)
			},
		},
		{
			name:    "providers endpoint that is not a valid url",
			cached:  &handlers.RegistryURLs{ProvidersV1: "https://bad host/v1/providers"},
			respond: respondWith(http.StatusOK, "{}"),
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var urlErr *url.Error

				require.ErrorAs(t, err, &urlErr)
			},
		},
		{
			name:   "registry unreachable",
			cached: handlers.DefaultRegistryURLs,
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				return nil, cut
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, cut)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := newDirectHandler(vhttp.NewMemClient(tc.respond))
			if tc.cached != nil {
				handler.SetDiscoveryURLCache("registry.example.com", tc.cached)
			}

			versions, err := handler.GetVersions(t.Context(), mirroredProvider())
			tc.assertErr(t, err)
			assert.Nil(t, versions)

			body, err := handler.GetPlatform(t.Context(), mirroredProvider())
			tc.assertErr(t, err)
			assert.Nil(t, body)
		})
	}
}

func TestCommonProviderHandlerCanHandleProvider(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		includes *[]string
		excludes *[]string
		name     string
		want     bool
	}{
		{
			name: "no filters",
			want: true,
		},
		{
			name:     "provider excluded",
			excludes: &[]string{"registry.example.com/example/*"},
		},
		{
			name:     "provider included",
			includes: &[]string{"registry.example.com/*/*"},
			want:     true,
		},
		{
			name:     "provider not included",
			includes: &[]string{"registry.other.com/*/*"},
		},
		{
			name:     "exclude wins over include",
			includes: &[]string{"registry.example.com/*/*"},
			excludes: &[]string{"registry.example.com/example/example"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := handlers.NewCommonProviderHandler(
				logger.CreateLogger(),
				vhttp.NewNoNetworkClient(),
				tc.includes,
				tc.excludes,
			)

			assert.Equal(t, tc.want, handler.CanHandleProvider(mirroredProvider()))
		})
	}
}

func TestCommonProviderHandlerDiscoveryURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		respond vhttp.Handler
		want    *handlers.RegistryURLs
		name    string
		wantErr bool
	}{
		{
			name:    "discovered urls",
			respond: respondWith(http.StatusOK, `{"providers.v1":"/custom/providers/"}`),
			want:    &handlers.RegistryURLs{ProvidersV1: "/custom/providers/"},
		},
		{
			name:    "registry without a discovery document falls back to defaults",
			respond: respondWith(http.StatusNotFound, ""),
			want:    handlers.DefaultRegistryURLs,
		},
		{
			name:    "registry refusing discovery fails",
			respond: respondWith(http.StatusForbidden, ""),
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := handlers.NewCommonProviderHandler(logger.CreateLogger(), vhttp.NewMemClient(tc.respond), nil, nil)

			urls, err := handler.DiscoveryURL(t.Context(), "registry.example.com")
			if tc.wantErr {
				require.Error(t, err)
				assert.Nil(t, urls)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, urls)
		})
	}
}

// newDirectHandler returns a direct handler that talks to the registry through c.
func newDirectHandler(c vhttp.Client) *handlers.DirectProviderHandler {
	return handlers.NewDirectProviderHandler(
		logger.CreateLogger(),
		c,
		cliconfig.NewProviderInstallationDirect(nil, nil),
		nil,
	)
}

// registryRoutes returns a handler serving bodies by URL path, and 404 otherwise.
func registryRoutes(bodies map[string]string) vhttp.Handler {
	return func(_ context.Context, req *http.Request) (*http.Response, error) {
		body, ok := bodies[req.URL.Path]
		if !ok {
			return vhttp.Respond(http.StatusNotFound, nil, nil), nil
		}

		return vhttp.Respond(http.StatusOK, []byte(body), http.Header{"Content-Type": []string{"application/json"}}), nil
	}
}
