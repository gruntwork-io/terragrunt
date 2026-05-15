package providercache_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/gruntwork-io/terragrunt/internal/providercache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// fakeDiscoverer pretends the upstream registry advertised the given modules.v1
// path during well-known discovery. Used to bypass real network discovery in tests.
type fakeDiscoverer struct {
	modulesV1 string
}

func (d *fakeDiscoverer) DiscoveryURL(_ context.Context, _ string) (*handlers.RegistryURLs, error) {
	return &handlers.RegistryURLs{
		ProvidersV1: "/v1/providers",
		ModulesV1:   d.modulesV1,
	}, nil
}

// TestNestedModuleCredentials reproduces issue #5970: when TG_PROVIDER_CACHE is on,
// the cache server was forwarding nested module-registry requests with its own
// x-api-key bearer token instead of the user's real upstream credentials, causing
// 403s. The cache server must strip its own auth header and re-inject the user's
// configured credentials when proxying modules.v1 requests upstream.
func TestNestedModuleCredentials(t *testing.T) {
	t.Parallel()

	const realUserToken = "real-user-token"

	var (
		upstreamHits   atomic.Int32
		upstreamAuth   atomic.Value
		upstreamReject atomic.Int32
	)

	const versionsBody = `{"modules":[{"versions":[{"version":"0.1.0"},{"version":"0.2.0"}]}]}`

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		upstreamAuth.Store(auth)
		upstreamHits.Add(1)

		if auth != "Bearer "+realUserToken {
			upstreamReject.Add(1)
			w.WriteHeader(http.StatusForbidden)

			return
		}

		switch r.URL.Path {
		case "/v1/modules/private/lambda/aws/versions":
			w.Header().Set("Content-Type", "application/json")

			if _, err := io.WriteString(w, versionsBody); err != nil {
				t.Errorf("upstream write failed: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(upstream.Close)

	registryName := strings.TrimPrefix(upstream.URL, "http://")

	// Build a credentials source that has the user's real token for the upstream host.
	cliCfg := &cliconfig.Config{
		Credentials: []cliconfig.ConfigCredentials{
			{Name: "127.0.0.1", Token: realUserToken},
		},
	}
	credsSource := cliCfg.CredentialsSource(map[string]string{})

	// The fake discoverer returns the upstream's full URL as modules.v1, so the
	// proxy targets the httptest server (HTTP, not HTTPS) without DNS lookups.
	discoverer := &fakeDiscoverer{modulesV1: upstream.URL + "/v1/modules/"}

	cacheToken := fmt.Sprintf("%s:%s", providercache.APIKeyAuth, uuid.New().String())

	providerCacheDir := helpers.TmpDirWOSymlinks(t)
	pluginCacheDir := helpers.TmpDirWOSymlinks(t)

	l := logger.CreateLogger()
	providerService := services.NewProviderService(providerCacheDir, pluginCacheDir, nil, l)
	proxyProviderHandler := handlers.NewProxyProviderHandler(l, vhttp.NewNoNetworkClient(), credsSource)
	proxyModuleHandler := handlers.NewProxyModuleHandler(l, credsSource, discoverer, []string{registryName})

	server := cache.NewServer(
		cache.WithToken(cacheToken),
		cache.WithProviderService(providerService),
		cache.WithProxyProviderHandler(proxyProviderHandler),
		cache.WithProxyModuleHandler(proxyModuleHandler),
		cache.WithCacheProviderHTTPStatusCode(providercache.CacheProviderHTTPStatusCode),
		cache.WithLogger(l),
	)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ln, err := server.Listen(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		if err := ln.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("listener close failed: %v", err)
		}
	})

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return server.Run(gctx, ln) })

	// Build the same URL OpenTofu/Terraform would hit via the host block:
	// <cache server>/v1/modules/<registry>/<module path>
	moduleURL := server.ModuleController.URL()
	moduleURL.Path += "/" + registryName + "/private/lambda/aws/versions"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, moduleURL.String(), nil)
	require.NoError(t, err)
	// OpenTofu sends the host block's TF_TOKEN_<host> value, which Terragrunt has
	// rewritten to the cache server's API key. The cache server must NOT forward
	// this token upstream; it must look up the user's real token instead.
	req.Header.Set("Authorization", "Bearer "+cacheToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, resp.Body.Close())
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "expected upstream success; body=%s", string(body))
	assert.JSONEq(t, versionsBody, string(body))

	assert.Equal(t, int32(1), upstreamHits.Load(), "upstream registry should have been hit exactly once")
	assert.Equal(t, int32(0), upstreamReject.Load(), "upstream registry should not have rejected the request")
	assert.Equal(t, "Bearer "+realUserToken, upstreamAuth.Load(),
		"cache server must forward the user's real upstream credentials, not its own API key")

	cancel()
	require.NoError(t, g.Wait())
}
