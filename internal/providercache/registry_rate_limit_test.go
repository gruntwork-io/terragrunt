package providercache_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
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

// rateLimitedUnits is how many units initialize at once against the
// rate-limited registry.
const rateLimitedUnits = 24

// TestProviderCacheSharesRegistryRequestsUnderARateLimitWithRacing pins how
// the cache server treats a registry that rate limits, the failure reported in
// https://github.com/gruntwork-io/terragrunt/issues/3234. Many units ask the
// cache server for the same providers at once, and the registry answers 429
// once it has served as many metadata requests as there are distinct ones.
// The server must send each distinct request once and share the answer, so
// every unit gets the versions it asked for and every provider is cached,
// and the registry never has a reason to refuse. Before, each unit cost the
// registry its own requests, and every refused one reached the unit as a
// provider with no versions or no download URL.
func TestProviderCacheSharesRegistryRequestsUnderARateLimitWithRacing(t *testing.T) {
	t.Parallel()

	providerNames := []string{"alpha", "bravo", "charlie"}

	// One versions listing and one platform lookup per provider.
	metadataLimit := 2 * len(providerNames)

	archive := buildWarmupProviderArchive(t)

	var (
		registryMu sync.Mutex
		metadata   int
		refused    int
	)

	// admit counts a metadata request and refuses it once the limit is spent,
	// the way a registry behind a rate limit does.
	admit := func(w http.ResponseWriter) bool {
		registryMu.Lock()
		defer registryMu.Unlock()

		if metadata >= metadataLimit {
			refused++

			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)

			return false
		}

		metadata++

		return true
	}

	// The registry limits metadata requests (version listings and platform
	// lookups) only. Archives and checksums are always served.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body string

		switch {
		case strings.HasSuffix(r.URL.Path, "/versions"):
			if !admit(w) {
				return
			}

			body = rateLimitedVersionsListing()
		case strings.Contains(r.URL.Path, "/download/"):
			if !admit(w) {
				return
			}

			archiveName := rateLimitedArchiveName(r.URL.Path)
			body = warmupPlatformResponse(archiveName, "http://"+r.Host, "/archives/"+archiveName, archive)
		case r.URL.Path == warmupShasumsPath:
			var sums strings.Builder

			for _, name := range providerNames {
				sums.WriteString(warmupShasums(rateLimitedArchiveFor(name), archive))
			}

			body = sums.String()
		case strings.HasPrefix(r.URL.Path, "/archives/"):
			_, err := w.Write(archive)
			assert.NoError(t, err, "upstream archive write failed")

			return
		default:
			w.WriteHeader(http.StatusNotFound)

			return
		}

		_, err := io.WriteString(w, body)
		assert.NoError(t, err, "upstream response write failed")
	}))
	t.Cleanup(upstream.Close)

	l := logger.CreateLogger()
	providerCacheDir := helpers.TmpDirWOSymlinks(t)
	pluginCacheDir := helpers.TmpDirWOSymlinks(t)

	// A temp dir of its own keeps the provider lock files away from sibling tests.
	tempDir := helpers.TmpDirWOSymlinks(t)
	v := venvtest.NewOSWithEmptyEnv().WithTempDir(func() string { return tempDir })

	providerService := services.NewProviderService(providerCacheDir, pluginCacheDir, nil, l, v)

	directHandler := handlers.NewDirectProviderHandler(
		l,
		vhttp.NewOSClient(),
		new(cliconfig.ProviderInstallationDirect),
		nil,
	)
	directHandler.SetDiscoveryURLCache(warmupRegistryName, &handlers.RegistryURLs{
		ProvidersV1: upstream.URL + "/v1/providers",
	})

	token := fmt.Sprintf("%s:%s", providercache.APIKeyAuth, uuid.New().String())

	server := cache.NewServer(
		cache.WithToken(token),
		cache.WithProviderService(providerService),
		cache.WithProviderHandlers(directHandler),
		cache.WithProxyProviderHandler(handlers.NewProxyProviderHandler(l, vhttp.NewOSClient(), nil)),
		cache.WithCacheProviderHTTPStatusCode(providercache.CacheProviderHTTPStatusCode),
		cache.WithLogger(l),
	)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ln, err := server.Listen(ctx, venvtest.NewOSWithEmptyEnv())
	require.NoError(t, err)

	t.Cleanup(func() {
		if err := ln.Close(); !errors.Is(err, net.ErrClosed) {
			assert.NoError(t, err, "listener close failed")
		}
	})

	srvGroup, srvCtx := errgroup.WithContext(ctx)
	srvGroup.Go(func() error { return server.Run(srvCtx, ln) })

	baseURL := server.ProviderController.URL()

	// A bounded client keeps a stuck cache server from hanging the test
	// until the suite timeout.
	client := &http.Client{Timeout: 30 * time.Second}

	requestIDs := make([]string, rateLimitedUnits)
	reqGroup, reqCtx := errgroup.WithContext(ctx)

	for unit := range rateLimitedUnits {
		requestIDs[unit] = uuid.New().String()

		reqGroup.Go(func() error {
			for _, name := range providerNames {
				prefix := strings.Join([]string{
					baseURL.String(),
					requestIDs[unit],
					warmupRegistryName,
					warmupProviderNamespace,
					name,
				}, "/")

				status, body, err := getWithToken(reqCtx, client, prefix+"/versions", token)
				if err != nil {
					return err
				}

				if assert.Equal(t, http.StatusOK, status, "unit %d: versions of %s: %s", unit, name, body) {
					var answer struct {
						Versions json.RawMessage `json:"versions"`
					}

					if err := json.Unmarshal(body, &answer); err != nil {
						return fmt.Errorf("unit %d: versions of %s: %w", unit, name, err)
					}

					assert.JSONEq(t, rateLimitedVersions(), string(answer.Versions), "unit %d: versions of %s", unit, name)
				}

				platformURL := strings.Join([]string{
					prefix,
					warmupProviderVersion,
					"download",
					runtime.GOOS,
					runtime.GOARCH,
				}, "/")

				status, _, err = getWithToken(reqCtx, client, platformURL, token)
				if err != nil {
					return err
				}

				assert.Equal(t, providercache.CacheProviderHTTPStatusCode, status, "unit %d: platform of %s", unit, name)
			}

			return nil
		})
	}

	require.NoError(t, reqGroup.Wait())

	for unit, requestID := range requestIDs {
		cached, err := providerService.WaitForCacheReady(requestID)
		require.NoError(t, err, "unit %d: every provider must be cached", unit)
		assert.Len(t, cached, len(providerNames), "unit %d", unit)
	}

	registryMu.Lock()
	gotMetadata, gotRefused := metadata, refused
	registryMu.Unlock()

	assert.Equal(t, metadataLimit, gotMetadata, "each distinct metadata request must reach the registry once")
	assert.Zero(t, gotRefused, "the registry must never have to refuse a request")

	cancel()
	require.NoError(t, srvGroup.Wait())
}

// rateLimitedArchiveName is the archive the platform lookup at urlPath
// describes. The provider name is the path segment before the version.
func rateLimitedArchiveName(urlPath string) string {
	parts := strings.Split(urlPath, "/")
	for i, part := range parts {
		if part == warmupProviderVersion && i > 0 {
			return rateLimitedArchiveFor(parts[i-1])
		}
	}

	return rateLimitedArchiveFor("unknown")
}

func rateLimitedArchiveFor(name string) string {
	return fmt.Sprintf(
		"terraform-provider-%s_%s_%s_%s.zip",
		name,
		warmupProviderVersion,
		runtime.GOOS,
		runtime.GOARCH,
	)
}

// rateLimitedVersionsListing is the registry's versions listing for every
// provider it serves: one version, on the running platform.
func rateLimitedVersionsListing() string {
	return fmt.Sprintf(
		`{"versions":[{"version":%q,"protocols":["5.0"],"platforms":[{"os":%q,"arch":%q}]}]}`,
		warmupProviderVersion,
		runtime.GOOS,
		runtime.GOARCH,
	)
}

// rateLimitedVersions is the versions array a unit must be served.
func rateLimitedVersions() string {
	return fmt.Sprintf(
		`[{"version":%q,"protocols":["5.0"],"platforms":[{"os":%q,"arch":%q}]}]`,
		warmupProviderVersion,
		runtime.GOOS,
		runtime.GOARCH,
	)
}

// getWithToken sends an authorized GET and returns the status and body.
func getWithToken(ctx context.Context, client *http.Client, url, token string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}

	body, readErr := io.ReadAll(resp.Body)

	return resp.StatusCode, body, errors.Join(readErr, resp.Body.Close())
}
