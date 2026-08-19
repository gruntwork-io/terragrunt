package cache_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
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

	ln, err := server.Listen(t.Context())
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
		cache.WithProviderService(services.NewProviderService(tmp, tmp, nil, l, venv.OSVenv())),
	)

	ln, err := server.Listen(t.Context())
	require.NoError(t, err)
	require.NoError(t, ln.Close())

	require.ErrorIs(t, server.Run(t.Context(), ln), net.ErrClosed)
}

// stubProviderHandler claims a single registry and counts how many times it is asked to discover one.
type stubProviderHandler struct {
	urls         *handlers.RegistryURLs
	registryName string
	calls        atomic.Int64
}

func (h *stubProviderHandler) CanHandleProvider(provider *models.Provider) bool {
	return provider.RegistryName == h.registryName
}

func (h *stubProviderHandler) GetVersions(
	_ context.Context,
	_ *models.Provider,
) (models.Versions, error) {
	return nil, nil
}

func (h *stubProviderHandler) GetPlatform(
	_ context.Context,
	_ *models.Provider,
) (*models.ResponseBody, error) {
	return nil, nil
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
