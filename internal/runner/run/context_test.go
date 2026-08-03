package run_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModuleVersionResolverSharedPerRunWithRacing pins the contract behind
// [run.WithModuleVersionResolver]: every lookup on one run's context hands
// back the same resolver, so concurrent units resolving the same module and
// constraint query the registry once, while a second run's context starts
// with a cold cache instead of inheriting state from the first.
func TestModuleVersionResolverSharedPerRunWithRacing(t *testing.T) {
	t.Parallel()

	var versionsHits atomic.Int64

	server := newCountingVersionsServer(t, &versionsHits)
	source := "tfr://" + server.Listener.Addr().String() + "/foo/bar/baz"
	l := logger.CreateLogger()

	ctx := run.WithModuleVersionResolver(t.Context())
	run.ModuleVersionResolverFromContext(ctx).WithHTTPClient(server.Client())

	var wg sync.WaitGroup

	for range 10 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Fetch the handle from the context on every use, the way the
			// download path does. If lookups stopped returning the one shared
			// resolver, the fresh fallback would not trust the test server's
			// certificate and Pin would fail.
			pinned, err := run.ModuleVersionResolverFromContext(ctx).Pin(
				ctx, l, tfimpl.OpenTofu, source, "~> 3.0",
			)
			assert.NoError(t, err)
			assert.Equal(t, source+"?version=3.3.0", pinned)
		}()
	}

	wg.Wait()

	assert.Equal(t, int64(1), versionsHits.Load())

	// A second installed resolver stands in for a second run: its cache must
	// start cold, proving memoization lives on the run's context rather than
	// in package-level state.
	otherCtx := run.WithModuleVersionResolver(t.Context())
	run.ModuleVersionResolverFromContext(otherCtx).WithHTTPClient(server.Client())

	pinned, err := run.ModuleVersionResolverFromContext(otherCtx).Pin(
		otherCtx, l, tfimpl.OpenTofu, source, "~> 3.0",
	)
	require.NoError(t, err)
	assert.Equal(t, source+"?version=3.3.0", pinned)
	assert.Equal(t, int64(2), versionsHits.Load())
}

// TestModuleVersionResolverFromContextFallback pins the documented fallback:
// a context without an installed resolver yields a working resolver whose
// memoization is scoped to that single handle, so two lookups do not share a
// cache.
func TestModuleVersionResolverFromContextFallback(t *testing.T) {
	t.Parallel()

	var versionsHits atomic.Int64

	server := newCountingVersionsServer(t, &versionsHits)
	source := "tfr://" + server.Listener.Addr().String() + "/foo/bar/baz"
	l := logger.CreateLogger()

	first := run.ModuleVersionResolverFromContext(t.Context()).WithHTTPClient(server.Client())

	for range 2 {
		pinned, err := first.Pin(t.Context(), l, tfimpl.OpenTofu, source, "~> 3.0")
		require.NoError(t, err)
		assert.Equal(t, source+"?version=3.3.0", pinned)
	}

	assert.Equal(t, int64(1), versionsHits.Load())

	second := run.ModuleVersionResolverFromContext(t.Context()).WithHTTPClient(server.Client())

	pinned, err := second.Pin(t.Context(), l, tfimpl.OpenTofu, source, "~> 3.0")
	require.NoError(t, err)
	assert.Equal(t, source+"?version=3.3.0", pinned)
	assert.Equal(t, int64(2), versionsHits.Load())
}

// newCountingVersionsServer stands up a TLS test server that speaks the
// service-discovery and list-versions endpoints of the module-registry
// protocol, recording every list-versions request in versionsHits.
func newCountingVersionsServer(t *testing.T, versionsHits *atomic.Int64) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/terraform.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"modules.v1":"/v1/modules/"}`))
		assert.NoError(t, err)
	})

	mux.HandleFunc("/v1/modules/foo/bar/baz/versions", func(w http.ResponseWriter, _ *http.Request) {
		versionsHits.Add(1)

		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"modules":[{"versions":[{"version":"3.3.0"},{"version":"2.0.0"}]}]}`))
		assert.NoError(t, err)
	})

	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)

	return server
}
