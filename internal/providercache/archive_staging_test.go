package providercache_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/gruntwork-io/terragrunt/internal/providercache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// TestProviderCacheConcurrentRunsStageArchivesApartWithRacing pins the isolation contract between
// Terragrunt runs sharing a machine: each cache server stages downloaded archives where no other
// server will touch them, so one run finishing does not delete an archive another run is still
// unpacking. Both runs warm up the same provider platform, which is what used to collide.
func TestProviderCacheConcurrentRunsStageArchivesApartWithRacing(t *testing.T) {
	t.Parallel()

	registryName, upstreamURL := startFakeProviderRegistry(t)

	firstRun := startProviderCacheRun(t, registryName, upstreamURL)
	secondRun := startProviderCacheRun(t, registryName, upstreamURL)

	firstArchive := firstRun.warmUpProviderArchive(t)
	secondArchive := secondRun.warmUpProviderArchive(t)

	assert.NotEqual(t, firstArchive, secondArchive,
		"concurrent runs must stage the same provider archive at paths of their own")

	firstRun.shutdown(t)

	assert.NoFileExists(t, firstArchive, "a run must clean up the archives it staged")
	assert.FileExists(t, secondArchive, "a run must not clean up archives staged by another run")

	secondRun.shutdown(t)

	assert.NoFileExists(t, secondArchive, "a run must clean up the archives it staged")
}

// startFakeProviderRegistry serves the tiny warm-up provider over plain HTTP, and returns the
// registry name to address it by along with its URL.
func startFakeProviderRegistry(t *testing.T) (string, string) {
	t.Helper()

	var (
		providerOS   = runtime.GOOS
		providerArch = runtime.GOARCH
	)

	archiveName := fmt.Sprintf(
		"terraform-provider-%s_%s_%s_%s.zip",
		warmupProviderName,
		warmupProviderVersion,
		providerOS,
		providerArch,
	)
	archive := buildWarmupProviderArchive(t)

	platformJSONPath := strings.Join([]string{
		"/v1/providers",
		warmupProviderNamespace,
		warmupProviderName,
		warmupProviderVersion,
		"download",
		providerOS,
		providerArch,
	}, "/")
	archiveURLPath := "/archives/" + archiveName

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case platformJSONPath:
			w.Header().Set("Content-Type", "application/json")

			body := fmt.Sprintf(
				`{"os":%q,"arch":%q,"filename":%q,"download_url":%q}`,
				providerOS,
				providerArch,
				archiveName,
				"http://"+r.Host+archiveURLPath,
			)

			if _, err := io.WriteString(w, body); err != nil {
				t.Errorf("upstream platform response write failed: %v", err)
			}
		case archiveURLPath:
			if _, err := w.Write(archive); err != nil {
				t.Errorf("upstream archive write failed: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(upstream.Close)

	return strings.TrimPrefix(upstream.URL, "http://"), upstream.URL
}

// providerCacheRun is a single cache server with the provider cache directory of its own that a
// Terragrunt run would have.
type providerCacheRun struct {
	service      *services.ProviderService
	server       *cache.Server
	serverGroup  *errgroup.Group
	cancel       context.CancelFunc
	registryName string
	token        string
}

func startProviderCacheRun(t *testing.T, registryName, upstreamURL string) *providerCacheRun {
	t.Helper()

	l := logger.CreateLogger()
	service := services.NewProviderService(
		helpers.TmpDirWOSymlinks(t),
		helpers.TmpDirWOSymlinks(t),
		nil,
		l,
		venvtest.NewOSWithEmptyEnv(),
	)

	// The pre-populated discovery cache points version and platform lookups at
	// the fake upstream over plain HTTP, without DNS lookups.
	directHandler := handlers.NewDirectProviderHandler(
		l,
		vhttp.NewOSClient(),
		new(cliconfig.ProviderInstallationDirect),
		nil,
	)
	directHandler.SetDiscoveryURLCache(registryName, &handlers.RegistryURLs{
		ProvidersV1: upstreamURL + "/v1/providers",
	})

	token := fmt.Sprintf("%s:%s", providercache.APIKeyAuth, uuid.New().String())

	server := cache.NewServer(
		cache.WithToken(token),
		cache.WithProviderService(service),
		cache.WithProviderHandlers(directHandler),
		cache.WithProxyProviderHandler(handlers.NewProxyProviderHandler(l, vhttp.NewOSClient(), nil)),
		cache.WithCacheProviderHTTPStatusCode(providercache.CacheProviderHTTPStatusCode),
		cache.WithLogger(l),
	)

	ctx, cancel := context.WithCancel(t.Context())

	ln, err := server.Listen(ctx, venvtest.NewOSWithEmptyEnv())
	require.NoError(t, err)

	t.Cleanup(func() {
		cancel()

		if err := ln.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("listener close failed: %v", err)
		}
	})

	serverGroup, serverCtx := errgroup.WithContext(ctx)
	serverGroup.Go(func() error { return server.Run(serverCtx, ln) })

	return &providerCacheRun{
		service:      service,
		server:       server,
		serverGroup:  serverGroup,
		cancel:       cancel,
		registryName: registryName,
		token:        token,
	}
}

// warmUpProviderArchive caches the warm-up provider and returns the path the archive was staged at,
// which the run holds until it shuts down.
func (run *providerCacheRun) warmUpProviderArchive(t *testing.T) string {
	t.Helper()

	requestID := uuid.New().String()

	downloadURL := run.server.ProviderController.URL()
	downloadURL.Path += "/" + strings.Join([]string{
		requestID,
		run.registryName,
		warmupProviderNamespace,
		warmupProviderName,
		warmupProviderVersion,
		"download",
		runtime.GOOS,
		runtime.GOARCH,
	}, "/")

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, downloadURL.String(), nil)
	require.NoError(t, err)

	req.Header.Set("Authorization", "Bearer "+run.token)

	// A bounded client keeps a stuck cache server from hanging the test
	// until the suite timeout.
	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, providercache.CacheProviderHTTPStatusCode, resp.StatusCode)

	cachedProviders, err := run.service.WaitForCacheReady(requestID)
	require.NoError(t, err)
	require.Len(t, cachedProviders, 1)

	cachedProvider, ok := cachedProviders[0].(*services.ProviderCache)
	require.True(t, ok)

	archivePath := cachedProvider.ArchivePath()
	require.NotEmpty(t, archivePath, "a warmed up provider keeps its staged archive until shutdown")

	return archivePath
}

func (run *providerCacheRun) shutdown(t *testing.T) {
	t.Helper()

	run.cancel()
	require.NoError(t, run.serverGroup.Wait())
}
