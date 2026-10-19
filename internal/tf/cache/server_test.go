package cache_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// TestServerDiscoveryURLUsesTheHandlerThatClaimsTheRegistry pins that discovery is answered by the first handler whose include and exclude settings claim the registry, so the mirror a user configured for a host is the one asked where that host's API lives, and no other handler is contacted.
func TestServerDiscoveryURLUsesTheHandlerThatClaimsTheRegistry(t *testing.T) {
	t.Parallel()

	const registryName = "registry.example.com"

	refusing := &stubProviderHandler{
		registryName: "other.example.com",
		urls:         &handlers.RegistryURLs{ModulesV1: "/refusing/modules", ProvidersV1: "/refusing/providers"},
	}
	claiming := &stubProviderHandler{
		registryName: registryName,
		urls:         &handlers.RegistryURLs{ModulesV1: "/claiming/modules", ProvidersV1: "/claiming/providers"},
	}

	server := cache.NewServer(
		cache.WithLogger(logger.CreateLogger()),
		cache.WithProviderHandlers(refusing, claiming),
	)

	urls, err := server.DiscoveryURL(t.Context(), registryName)
	require.NoError(t, err)

	assert.Equal(t, claiming.urls, urls)
	assert.Equal(t, int64(1), claiming.calls.Load())
	assert.Zero(
		t,
		refusing.calls.Load(),
		"a handler that does not claim the registry must never be asked to discover it",
	)
}

// TestServerDiscoveryURLFallsBackToTheDefaultRegistry pins that a registry no handler claims still resolves to the default registry URLs, because returning nothing there breaks provider downloads for every registry the user has not configured a handler for.
func TestServerDiscoveryURLFallsBackToTheDefaultRegistry(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		handler *stubProviderHandler
		name    string
	}{
		{
			name: "no handlers configured",
		},
		{
			name: "no handler claims the registry",
			handler: &stubProviderHandler{
				registryName: "other.example.com",
				urls:         &handlers.RegistryURLs{ModulesV1: "/other/modules", ProvidersV1: "/other/providers"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := []cache.Option{cache.WithLogger(logger.CreateLogger())}
			if tc.handler != nil {
				opts = append(opts, cache.WithProviderHandlers(tc.handler))
			}

			urls, err := cache.NewServer(opts...).DiscoveryURL(t.Context(), "registry.example.com")
			require.NoError(t, err)
			assert.Equal(t, handlers.DefaultRegistryURLs, urls)

			if tc.handler != nil {
				assert.Zero(t, tc.handler.calls.Load(), "a refusing handler must not be contacted")
			}
		})
	}
}

// TestWithCacheProviderHTTPStatusCode pins that the configured status code reaches the provider controller verbatim, since that code is what OpenTofu/Terraform sees while a provider is being cached and is how the client learns to retry rather than fail.
func TestWithCacheProviderHTTPStatusCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		opts     []cache.Option
		wantCode int
	}{
		{
			name:     "configured status code",
			opts:     []cache.Option{cache.WithCacheProviderHTTPStatusCode(http.StatusLocked)},
			wantCode: http.StatusLocked,
		},
		{
			name: "unset status code is not defaulted by the server",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := append([]cache.Option{cache.WithLogger(logger.CreateLogger())}, tc.opts...)

			assert.Equal(
				t,
				tc.wantCode,
				cache.NewServer(opts...).ProviderController.CacheProviderHTTPStatusCode,
			)
		})
	}
}

// TestWithProxyModuleHandlerRegistersTheModuleEndpoint pins that modules.v1 is advertised in service discovery only when a module proxy is configured, because a client that discovers the endpoint sends module requests the server would otherwise have no handler to answer.
func TestWithProxyModuleHandlerRegistersTheModuleEndpoint(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		wantModulesV1      string
		proxyModuleHandler bool
	}{
		{
			name: "without a proxy module handler",
		},
		{
			name:               "with a proxy module handler",
			wantModulesV1:      "/v1/modules/",
			proxyModuleHandler: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()

			opts := []cache.Option{cache.WithLogger(l)}
			if tc.proxyModuleHandler {
				opts = append(opts, cache.WithProxyModuleHandler(
					handlers.NewProxyModuleHandler(l, vhttp.NewNoNetworkClient(), nil, nil, nil),
				))
			}

			endpoints := discoveryEndpoints(t, cache.NewServer(opts...))

			assert.Equal(
				t,
				"/v1/providers",
				endpoints["providers.v1"],
				"the provider API is always advertised",
			)

			if tc.wantModulesV1 == "" {
				assert.NotContains(
					t,
					endpoints,
					"modules.v1",
					"the module API must not be advertised without a module proxy",
				)

				return
			}

			assert.Equal(t, tc.wantModulesV1, endpoints["modules.v1"])
		})
	}
}

// TestServerListenReportsBindFailure pins that an address the cache server cannot bind is reported to the caller, since a swallowed bind failure leaves units running against a cache server that never accepts a connection.
func TestServerListenReportsBindFailure(t *testing.T) {
	t.Parallel()

	lc := &net.ListenConfig{}

	taken, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, taken.Close()) })

	addr, ok := taken.Addr().(*net.TCPAddr)
	require.True(t, ok)

	server := cache.NewServer(
		cache.WithHostname("127.0.0.1"),
		cache.WithPort(addr.Port),
		cache.WithLogger(logger.CreateLogger()),
	)

	ln, err := server.Listen(t.Context(), venvtest.NewOSWithEmptyEnv())
	assert.Nil(t, ln, "no listener is handed back when the address cannot be bound")

	var opErr *net.OpError

	require.ErrorAs(t, err, &opErr)
}

// TestServerRunReportsServeFailure pins that a listener the server cannot serve on ends the run with an error, since returning nil there reads as a clean shutdown while no unit can reach the cache.
func TestServerRunReportsServeFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	tmp := t.TempDir()

	server := cache.NewServer(
		cache.WithHostname("127.0.0.1"),
		cache.WithLogger(l),
		cache.WithProviderService(services.NewProviderService(tmp, tmp, nil, l, venvtest.NewOSWithEmptyEnv())),
	)

	ln, err := server.Listen(t.Context(), venvtest.NewOSWithEmptyEnv())
	require.NoError(t, err)
	require.NoError(t, ln.Close())

	require.ErrorIs(t, server.Run(t.Context(), ln), net.ErrClosed)
}

const (
	stubRegistryName = "registry.example.com"
	stubToken        = "stub-token"
)

var (
	// errRateLimited is a registry that went on rate limiting through every try.
	errRateLimited = &helpers.StatusError{
		URL:        "https://registry.example.com/v1/providers/example/tiny/versions",
		StatusCode: http.StatusTooManyRequests,
		RetryAfter: 1500 * time.Millisecond,
	}

	// errUnauthorized is a registry that refused the request for good.
	errUnauthorized = &helpers.StatusError{
		URL:        "https://registry.example.com/v1/providers/example/tiny/versions",
		StatusCode: http.StatusUnauthorized,
	}
)

// TestProviderVersionsWhenTheRegistryRefuses pins the versions answer while a
// registry is rate limiting or unavailable: 503 with the wait the registry
// asked for, rounded up to whole seconds, so OpenTofu/Terraform tries again.
// It used to be 200 with no versions, which reads as a provider with no
// releases. Versions another installation method found are still served, and
// a registry that refused for good keeps the answer it had.
func TestProviderVersionsWhenTheRegistryRefuses(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name           string
		wantRetryAfter string
		handlers       []*stubProviderHandler
		wantVersions   []string
		wantCode       int
	}{
		{
			name:           "the registry is rate limiting",
			handlers:       []*stubProviderHandler{{versionsErr: errRateLimited}},
			wantCode:       http.StatusServiceUnavailable,
			wantRetryAfter: "2",
		},
		{
			name: "another installation method has the versions",
			handlers: []*stubProviderHandler{
				{versionsErr: errRateLimited},
				{versions: models.Versions{{Version: "1.0.0"}}},
			},
			wantCode:     http.StatusOK,
			wantVersions: []string{"1.0.0"},
		},
		{
			name:         "the registry refused for good",
			handlers:     []*stubProviderHandler{{versionsErr: errUnauthorized}},
			wantCode:     http.StatusOK,
			wantVersions: []string{},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			providerHandlers := make([]handlers.ProviderHandler, 0, len(tt.handlers))

			for _, h := range tt.handlers {
				h.registryName = stubRegistryName
				providerHandlers = append(providerHandlers, h)
			}

			server := cache.NewServer(
				cache.WithToken(stubToken),
				cache.WithLogger(logger.CreateLogger()),
				cache.WithProviderHandlers(providerHandlers...),
			)

			rec := serveProviderRequest(t, server, "/v1/providers/"+stubRegistryName+"/example/tiny/versions")

			require.Equal(t, tt.wantCode, rec.Code)
			assert.Equal(t, tt.wantRetryAfter, rec.Header().Get("Retry-After"))

			if tt.wantVersions == nil {
				return
			}

			var body struct {
				Versions []struct {
					Version string `json:"version"`
				} `json:"versions"`
			}

			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

			versions := make([]string, 0, len(body.Versions))
			for _, v := range body.Versions {
				versions = append(versions, v.Version)
			}

			assert.Equal(t, tt.wantVersions, versions)
		})
	}
}

// TestProviderPlatformWhenNoInstallationMethodDescribesIt pins the answer to a
// caching request for a platform no installation method could describe. The
// provider service was handed a provider with no download details in this
// case and panicked, which the server reported as a bare 500. Now nothing is
// queued for caching, and the status says why: the registry is refusing
// requests for now, it refused this one for good, or no method has the
// platform.
func TestProviderPlatformWhenNoInstallationMethodDescribesIt(t *testing.T) {
	t.Parallel()

	tc := []struct {
		platformErr    error
		name           string
		wantRetryAfter string
		wantCode       int
	}{
		{
			name:           "the registry is rate limiting",
			platformErr:    errRateLimited,
			wantCode:       http.StatusServiceUnavailable,
			wantRetryAfter: "2",
		},
		{
			name:        "the registry refused for good",
			platformErr: errUnauthorized,
			wantCode:    http.StatusBadGateway,
		},
		{
			name:     "no installation method has the platform",
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()
			tmp := t.TempDir()
			providerService := services.NewProviderService(tmp, tmp, nil, l, venvtest.NewOSWithEmptyEnv())

			server := cache.NewServer(
				cache.WithToken(stubToken),
				cache.WithLogger(l),
				cache.WithProviderService(providerService),
				cache.WithProviderHandlers(&stubProviderHandler{
					registryName: stubRegistryName,
					platformErr:  tt.platformErr,
				}),
				cache.WithCacheProviderHTTPStatusCode(http.StatusLocked),
			)

			const requestID = "3f1c1d0a-6a43-4ae9-9d6b-7a1d3c0d9e2f"

			rec := serveProviderRequest(
				t,
				server,
				"/v1/providers/"+requestID+"/"+stubRegistryName+"/example/tiny/1.0.0/download/linux/amd64",
			)

			require.Equal(t, tt.wantCode, rec.Code)
			assert.Equal(t, tt.wantRetryAfter, rec.Header().Get("Retry-After"))

			queued, err := providerService.WaitForCacheReady(requestID)
			require.NoError(t, err)
			assert.Empty(t, queued, "a platform nobody described must not be queued for caching")
		})
	}
}

// TestProviderEndpointsWhenTheRegistryKeepsRefusing pins the same answers
// with nothing stubbed between the router and the registry: the direct
// installation method and its registry client, in front of a registry that
// rate limits every try. The registry may refuse its discovery document or
// the provider lookups. Either way the versions and platform endpoints answer
// 503 with the registry's wait, every lookup is tried four times, nothing is
// remembered between lookups, and nothing is queued for caching.
func TestProviderEndpointsWhenTheRegistryKeepsRefusing(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name            string
		wantDiscoveries int32
		wantLookups     int32
		refuseDiscovery bool
	}{
		{
			name:            "the registry refuses provider lookups",
			wantDiscoveries: 1,
			wantLookups:     8,
		},
		{
			name:            "the registry refuses its discovery document",
			refuseDiscovery: true,
			wantDiscoveries: 8,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				var discoveries, lookups atomic.Int32

				refusal := http.Header{"Retry-After": {"1"}}

				registry := vhttp.NewMemClient(func(_ context.Context, r *http.Request) (*http.Response, error) {
					if strings.HasSuffix(r.URL.Path, "/.well-known/terraform.json") {
						discoveries.Add(1)

						if tt.refuseDiscovery {
							return vhttp.Respond(http.StatusTooManyRequests, nil, refusal), nil
						}

						return vhttp.Respond(http.StatusOK, []byte(`{"providers.v1":"/v1/providers/"}`), nil), nil
					}

					lookups.Add(1)

					return vhttp.Respond(http.StatusTooManyRequests, nil, refusal), nil
				})

				l := logger.CreateLogger()
				tmp := t.TempDir()
				providerService := services.NewProviderService(tmp, tmp, nil, l, venvtest.NewOSWithEmptyEnv())

				server := cache.NewServer(
					cache.WithToken(stubToken),
					cache.WithLogger(l),
					cache.WithProviderService(providerService),
					cache.WithProviderHandlers(handlers.NewDirectProviderHandler(
						l,
						registry,
						new(cliconfig.ProviderInstallationDirect),
						nil,
					)),
					cache.WithCacheProviderHTTPStatusCode(http.StatusLocked),
				)

				rec := serveProviderRequest(t, server, "/v1/providers/"+stubRegistryName+"/example/tiny/versions")

				assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "versions")
				assert.Equal(t, "1", rec.Header().Get("Retry-After"), "versions")

				const requestID = "3f1c1d0a-6a43-4ae9-9d6b-7a1d3c0d9e2f"

				rec = serveProviderRequest(
					t,
					server,
					"/v1/providers/"+requestID+"/"+stubRegistryName+"/example/tiny/1.0.0/download/linux/amd64",
				)

				assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "platform")
				assert.Equal(t, "1", rec.Header().Get("Retry-After"), "platform")

				queued, err := providerService.WaitForCacheReady(requestID)
				require.NoError(t, err)
				assert.Empty(t, queued, "a platform nobody described must not be queued for caching")

				assert.Equal(t, tt.wantDiscoveries, discoveries.Load(), "discovery requests")
				assert.Equal(t, tt.wantLookups, lookups.Load(), "version and platform lookups")
			})
		})
	}
}

// serveProviderRequest sends an authorized GET for urlPath through server's
// router and returns what it answered.
func serveProviderRequest(t *testing.T, server *cache.Server, urlPath string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, urlPath, nil)
	req.Header.Set("Authorization", "Bearer "+stubToken)

	rec := httptest.NewRecorder()

	server.Router.ServeHTTP(rec, req)

	return rec
}

// stubProviderHandler claims a single registry, answers version lookups with
// what it was given, describes no platform, and counts how many times it is
// asked to discover a registry.
type stubProviderHandler struct {
	versionsErr  error
	platformErr  error
	urls         *handlers.RegistryURLs
	registryName string
	versions     models.Versions
	calls        atomic.Int64
}

func (h *stubProviderHandler) String() string {
	return "stub " + h.registryName
}

func (h *stubProviderHandler) CanHandleProvider(provider *models.Provider) bool {
	return provider.RegistryName == h.registryName
}

func (h *stubProviderHandler) GetVersions(
	_ context.Context,
	_ *models.Provider,
) (models.Versions, error) {
	return h.versions, h.versionsErr
}

func (h *stubProviderHandler) GetPlatform(
	_ context.Context,
	_ *models.Provider,
) (*models.ResponseBody, error) {
	return nil, h.platformErr
}

func (h *stubProviderHandler) DiscoveryURL(
	_ context.Context,
	_ string,
) (*handlers.RegistryURLs, error) {
	h.calls.Add(1)

	return h.urls, nil
}

func discoveryEndpoints(t *testing.T, server *cache.Server) map[string]string {
	t.Helper()

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		"/.well-known/terraform.json",
		nil,
	)
	rec := httptest.NewRecorder()

	server.Router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	endpoints := make(map[string]string)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &endpoints))

	return endpoints
}
