package cache_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
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
		cache.WithProxyProviderHandler(handlers.NewProxyProviderHandler(l, nil)),
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

	// The upstream host is unresolvable, so a routed request fails inside the
	// proxy rather than 404ing. Any status but 404 proves the route matched,
	// without depending on network access.
	require.NotEqual(t, http.StatusNotFound,
		getStatus(t, ctx, client, base+"/downloads/"+segment+"/example.invalid/provider.zip"),
		"correct secret segment must resolve the download route")

	cancel()
	require.NoError(t, errGroup.Wait())
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
