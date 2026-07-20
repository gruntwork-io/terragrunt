package providercache_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/gruntwork-io/terragrunt/internal/providercache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

const (
	// concurrentWarmupRequests fans out enough clients to reliably overlap
	// in-flight requests for the same provider platform.
	concurrentWarmupRequests = 20

	warmupProviderNamespace = "example"
	warmupProviderName      = "tiny"
	warmupProviderVersion   = "1.0.0"
)

// TestProviderCacheConcurrentWarmupWithRacing pins the warm-up dedup contract
// of the cache server: concurrent download requests for the same provider
// platform all receive the cache-provider status code while warm-up runs in
// the background, WaitForCacheReady converges on a single ready cache entry,
// and the upstream registry serves the provider archive exactly once.
func TestProviderCacheConcurrentWarmupWithRacing(t *testing.T) {
	t.Parallel()

	var (
		archiveHitsMu sync.Mutex
		archiveHits   int
	)

	providerOS := runtime.GOOS
	providerArch := runtime.GOARCH

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
			archiveHitsMu.Lock()

			archiveHits++

			archiveHitsMu.Unlock()

			if _, err := w.Write(archive); err != nil {
				t.Errorf("upstream archive write failed: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(upstream.Close)

	registryName := strings.TrimPrefix(upstream.URL, "http://")

	l := logger.CreateLogger()
	providerCacheDir := helpers.TmpDirWOSymlinks(t)
	pluginCacheDir := helpers.TmpDirWOSymlinks(t)

	providerService := services.NewProviderService(providerCacheDir, pluginCacheDir, nil, l)

	// The pre-populated discovery cache points version and platform lookups at
	// the fake upstream over plain HTTP, without DNS lookups.
	directHandler := handlers.NewDirectProviderHandler(
		l,
		new(cliconfig.ProviderInstallationDirect),
		nil,
	)
	directHandler.SetDiscoveryURLCache(registryName, &handlers.RegistryURLs{
		ProvidersV1: upstream.URL + "/v1/providers",
	})

	token := fmt.Sprintf("%s:%s", providercache.APIKeyAuth, uuid.New().String())

	server := cache.NewServer(
		cache.WithToken(token),
		cache.WithProviderService(providerService),
		cache.WithProviderHandlers(directHandler),
		cache.WithProxyProviderHandler(handlers.NewProxyProviderHandler(l, nil)),
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

	srvGroup, srvCtx := errgroup.WithContext(ctx)
	srvGroup.Go(func() error { return server.Run(srvCtx, ln) })

	requestID := uuid.New().String()

	downloadURL := server.ProviderController.URL()
	downloadURL.Path += "/" + strings.Join([]string{
		requestID,
		registryName,
		warmupProviderNamespace,
		warmupProviderName,
		warmupProviderVersion,
		"download",
		providerOS,
		providerArch,
	}, "/")

	statusCodes := make([]int, concurrentWarmupRequests)

	reqGroup, reqCtx := errgroup.WithContext(ctx)

	for i := range concurrentWarmupRequests {
		reqGroup.Go(func() error {
			req, err := http.NewRequestWithContext(
				reqCtx,
				http.MethodGet,
				downloadURL.String(),
				nil,
			)
			if err != nil {
				return err
			}

			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}

			statusCodes[i] = resp.StatusCode

			return resp.Body.Close()
		})
	}

	require.NoError(t, reqGroup.Wait())

	for i, statusCode := range statusCodes {
		assert.Equal(t, providercache.CacheProviderHTTPStatusCode, statusCode,
			"request %d should be told the provider is being cached in the background", i)
	}

	cachedProviders, err := providerService.WaitForCacheReady(requestID)
	require.NoError(t, err,
		"every concurrent request must converge on a successfully warmed cache")
	require.Len(t, cachedProviders, 1,
		"all requests target one provider platform, so exactly one cache entry becomes ready")

	packageDir := filepath.Join(
		providerCacheDir,
		registryName,
		warmupProviderNamespace,
		warmupProviderName,
		warmupProviderVersion,
		providerOS+"_"+providerArch,
	)
	assert.DirExists(t, packageDir,
		"warm-up should unpack the provider archive into the provider cache dir")

	archiveHitsMu.Lock()

	hits := archiveHits

	archiveHitsMu.Unlock()

	assert.Equal(t, 1, hits,
		"concurrent warm-up requests for the same provider platform must share a single upstream archive download")

	cancel()
	require.NoError(t, srvGroup.Wait())
}

// buildWarmupProviderArchive returns an in-memory zip archive holding a single
// fake provider binary, small enough to make warm-up nearly instant.
func buildWarmupProviderArchive(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	fw, err := zw.Create(
		fmt.Sprintf("terraform-provider-%s_v%s_x5", warmupProviderName, warmupProviderVersion),
	)
	require.NoError(t, err)

	_, err = fw.Write([]byte("fake provider binary"))
	require.NoError(t, err)

	require.NoError(t, zw.Close())

	return buf.Bytes()
}
