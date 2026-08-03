package cache_test

import (
	"context"
	"io"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// TestDownloadRouteRequiresSecretSegment verifies that the provider download
// route is reachable only via the secret path segment the server generates for
// itself. A caller that does not know the segment (another process on the same
// host) cannot reach the credential-injecting reverse proxy, while
// unauthenticated service discovery stays open for the tofu client.
func TestDownloadRouteRequiresSecretSegment(t *testing.T) {
	t.Parallel()

	const token = "x-api-key:per-run-secret-token"

	l := logger.CreateLogger()
	tmp := t.TempDir()

	server := cache.NewServer(
		cache.WithHostname("127.0.0.1"),
		cache.WithToken(token),
		cache.WithLogger(l),
		cache.WithProviderService(services.NewProviderService(tmp, tmp, nil, l)),
		cache.WithProxyProviderHandler(
			handlers.NewProxyProviderHandler(l, vhttp.NewNoNetworkClient(), nil),
		),
	)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	ln, err := server.Listen(ctx)
	require.NoError(t, err)

	errGroup, ctx := errgroup.WithContext(ctx)
	errGroup.Go(func() error { return server.Run(ctx, ln) })

	// Requests that arrive before Serve accepts wait in the listener backlog, so
	// no readiness poll is needed.
	base := "http://" + ln.Addr().String()
	client := &http.Client{Timeout: 5 * time.Second}

	segment := server.DownloaderController.Segment()

	const wrongSegment = "AAAAAAAAAAAAAAAAAAAAAAAAAA"

	// Service discovery is intentionally open: the tofu client fetches it
	// without a token.
	require.Equal(t, http.StatusOK,
		getStatus(t, ctx, client, base+"/.well-known/terraform.json"),
		"discovery must stay open")

	// The pre-hardening URL shape (no secret segment) no longer routes.
	require.Equal(t, http.StatusNotFound,
		getStatus(t, ctx, client, base+"/downloads/registry.terraform.io/provider.zip"),
		"download route must not be reachable without the secret segment")

	require.Equal(t, http.StatusNotFound,
		getStatus(t, ctx, client, base+"/downloads/"+wrongSegment+"/registry.terraform.io/provider.zip"),
		"download route must reject an incorrect secret segment")

	// The proxy's client refuses every request, so a routed request fails inside
	// the proxy rather than 404ing. Any status but 404 proves the route matched.
	require.NotEqual(t, http.StatusNotFound,
		getStatus(t, ctx, client, base+"/downloads/"+segment+"/example.invalid/provider.zip"),
		"correct secret segment must resolve the download route")

	cancel()
	require.NoError(t, errGroup.Wait())
}

// TestDownloadSegmentIsRedactedFromLogs verifies that the secret segment never
// reaches a log entry, where it would otherwise travel into a bug report as a
// working download URL.
func TestDownloadSegmentIsRedactedFromLogs(t *testing.T) {
	t.Parallel()

	hook := new(urlFieldHook)

	// Successful requests are logged at trace level, failed ones at error level;
	// trace captures the URI either way, so the assertion below does not depend
	// on how far into the proxy the request gets.
	l := logger.CreateLogger().WithOptions(
		log.WithLevel(log.TraceLevel),
		log.WithOutput(io.Discard),
		log.WithHooks(hook),
	)
	tmp := t.TempDir()

	server := cache.NewServer(
		cache.WithHostname("127.0.0.1"),
		cache.WithLogger(l),
		cache.WithProviderService(services.NewProviderService(tmp, tmp, nil, l)),
		cache.WithProxyProviderHandler(
			handlers.NewProxyProviderHandler(l, vhttp.NewNoNetworkClient(), nil),
		),
	)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	ln, err := server.Listen(ctx)
	require.NoError(t, err)

	errGroup, ctx := errgroup.WithContext(ctx)
	errGroup.Go(func() error { return server.Run(ctx, ln) })

	segment := server.DownloaderController.Segment()

	getStatus(t, ctx, &http.Client{Timeout: 5 * time.Second},
		"http://"+ln.Addr().String()+"/downloads/"+segment+"/example.invalid/provider.zip")

	// Shutting the server down first joins the request goroutine that logs.
	cancel()
	require.NoError(t, errGroup.Wait())

	uris := hook.URIs()
	require.Len(t, uris, 1)
	require.NotContains(t, uris[0], segment, "the secret segment must not reach the logs")
	require.Contains(t, uris[0], "example.invalid", "the rest of the URI must still be logged")
}

// urlFieldHook collects the URI of every request the cache server logs.
type urlFieldHook struct {
	uris []string
	mu   sync.Mutex
}

func (hook *urlFieldHook) Levels() []logrus.Level {
	return log.AllLevels.ToLogrusLevels()
}

func (hook *urlFieldHook) Fire(entry *logrus.Entry) error {
	uri, ok := entry.Data[placeholders.CacheServerURLKeyName].(string)
	if !ok {
		return nil
	}

	hook.mu.Lock()
	defer hook.mu.Unlock()

	hook.uris = append(hook.uris, uri)

	return nil
}

func (hook *urlFieldHook) URIs() []string {
	hook.mu.Lock()
	defer hook.mu.Unlock()

	return slices.Clone(hook.uris)
}

func getStatus(t *testing.T, ctx context.Context, client *http.Client, url string) int {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	return resp.StatusCode
}
