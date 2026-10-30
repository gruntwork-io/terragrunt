package handlers_test

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const discoveryRegistry = "registry.example.com"

var (
	// discoveryDocument is what discoveryRegistry serves at
	// /.well-known/terraform.json.
	discoveryDocument = []byte(`{"modules.v1":"/discovered/modules/","providers.v1":"/discovered/providers/"}`)

	// discoveredURLs is what discoveryDocument names.
	discoveredURLs = &handlers.RegistryURLs{
		ModulesV1:   "/discovered/modules/",
		ProvidersV1: "/discovered/providers/",
	}
)

// TestCommonProviderHandlerDiscoveryURLSharesOneRequestWithRacing pins that
// callers who miss the cache at once cost the registry one discovery request
// between them. Every unit of a `run --all` looks up the registry's service
// URLs before it asks for any provider, so without this the first wave of
// units sends the registry one request each. synctest parks every caller
// before the held request is released, so the sharing is observed rather
// than assumed.
func TestCommonProviderHandlerDiscoveryURLSharesOneRequestWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const callers = 16

		var hits atomic.Int32

		release := make(chan struct{})

		c := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
			hits.Add(1)
			<-release

			return vhttp.Respond(http.StatusOK, discoveryDocument, nil), nil
		})

		handler := handlers.NewCommonProviderHandler(logger.CreateLogger(), c, nil, nil)

		urls := make([]*handlers.RegistryURLs, callers)
		errs := make([]error, callers)

		var wg sync.WaitGroup

		for i := range callers {
			wg.Go(func() {
				urls[i], errs[i] = handler.DiscoveryURL(t.Context(), discoveryRegistry)
			})
		}

		synctest.Wait()
		close(release)
		wg.Wait()

		assert.Equal(t, int32(1), hits.Load(), "every caller must be waiting on one discovery request")

		for i := range callers {
			require.NoError(t, errs[i], "caller %d", i)
			assert.Equal(t, discoveredURLs, urls[i], "caller %d must get the shared answer", i)
		}
	})
}

// TestCommonProviderHandlerDiscoveryURLRetriesARefusal pins how discovery
// treats each answer. A refusal for now is tried again, waiting as long as
// the registry asks. A refusal that outlasts the tries reaches the caller as
// a [*helpers.StatusError], so the cache server can tell OpenTofu/Terraform
// to try again, and is not remembered. A registry without a discovery
// document (404, or 500) gets the default URLs at once, as before.
func TestCommonProviderHandlerDiscoveryURLRetriesARefusal(t *testing.T) {
	t.Parallel()

	refusal := func() *http.Response {
		return vhttp.Respond(http.StatusTooManyRequests, nil, http.Header{"Retry-After": {"1"}})
	}

	tc := []struct {
		wantURLs     *handlers.RegistryURLs
		name         string
		answers      []func() *http.Response
		wantStatus   int
		wantRequests int32
		wantWait     time.Duration
	}{
		{
			name: "the registry refuses once, then answers",
			answers: []func() *http.Response{
				refusal,
				func() *http.Response { return vhttp.Respond(http.StatusOK, discoveryDocument, nil) },
			},
			wantURLs:     discoveredURLs,
			wantRequests: 2,
			wantWait:     time.Second,
		},
		{
			name:         "the registry keeps refusing",
			answers:      []func() *http.Response{refusal},
			wantStatus:   http.StatusTooManyRequests,
			wantRequests: 4,
			wantWait:     3 * time.Second,
		},
		{
			name: "the registry has no discovery document",
			answers: []func() *http.Response{
				func() *http.Response { return vhttp.Respond(http.StatusNotFound, nil, nil) },
			},
			wantURLs:     handlers.DefaultRegistryURLs,
			wantRequests: 1,
		},
		{
			name: "the registry fails to serve its discovery document",
			answers: []func() *http.Response{
				func() *http.Response { return vhttp.Respond(http.StatusInternalServerError, nil, nil) },
			},
			wantURLs:     handlers.DefaultRegistryURLs,
			wantRequests: 1,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				var requests atomic.Int32

				c := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
					n := int(requests.Add(1))

					return tt.answers[min(n, len(tt.answers))-1](), nil
				})

				handler := handlers.NewCommonProviderHandler(logger.CreateLogger(), c, nil, nil)

				start := time.Now()

				urls, err := handler.DiscoveryURL(t.Context(), discoveryRegistry)

				assert.Equal(t, tt.wantWait, time.Since(start))
				assert.Equal(t, tt.wantRequests, requests.Load())

				if tt.wantStatus != 0 {
					var statusErr *helpers.StatusError

					require.ErrorAs(t, err, &statusErr)
					assert.Equal(t, tt.wantStatus, statusErr.StatusCode)
					assert.True(t, statusErr.Transient())
					assert.Nil(t, urls)

					_, err = handler.DiscoveryURL(t.Context(), discoveryRegistry)
					require.Error(t, err)
					assert.Equal(t, 2*tt.wantRequests, requests.Load(), "a refusal must not be remembered")

					return
				}

				require.NoError(t, err)
				assert.Equal(t, tt.wantURLs, urls)

				again, err := handler.DiscoveryURL(t.Context(), discoveryRegistry)
				require.NoError(t, err)
				assert.Equal(t, tt.wantURLs, again)
				assert.Equal(t, tt.wantRequests, requests.Load(), "a final answer must be remembered")
			})
		})
	}
}

// TestCommonProviderHandlerDiscoveryURLConcurrentHitsAndMissesWithRacing pins that parallel
// lookups all resolve while some callers read a cached registry and others store the
// registries they discover. Cache hits return without logging or HTTP, so no shared step
// orders their reads against the stores.
func TestCommonProviderHandlerDiscoveryURLConcurrentHitsAndMissesWithRacing(t *testing.T) {
	t.Parallel()

	const (
		callers        = 8
		cachedRegistry = "cached.example.com"
	)

	body := []byte(`{"modules.v1":"/discovered/modules/","providers.v1":"/discovered/providers/"}`)

	c := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return vhttp.Respond(http.StatusOK, body, nil), nil
	})

	handler := handlers.NewCommonProviderHandler(logger.CreateLogger(), c, nil, nil)

	cached := &handlers.RegistryURLs{
		ModulesV1:   "/cached/modules/",
		ProvidersV1: "/cached/providers/",
	}
	handler.SetDiscoveryURLCache(cachedRegistry, cached)

	var (
		ctx       = t.Context()
		start     = make(chan struct{})
		group     sync.WaitGroup
		hits      = make([]*handlers.RegistryURLs, callers)
		hitErrs   = make([]error, callers)
		misses    = make([]*handlers.RegistryURLs, callers)
		missErrs  = make([]error, callers)
		discovery = &handlers.RegistryURLs{
			ModulesV1:   "/discovered/modules/",
			ProvidersV1: "/discovered/providers/",
		}
	)

	for i := range callers {
		group.Go(func() {
			<-start

			hits[i], hitErrs[i] = handler.DiscoveryURL(ctx, cachedRegistry)
		})

		group.Go(func() {
			<-start

			misses[i], missErrs[i] = handler.DiscoveryURL(ctx, "registry-"+strconv.Itoa(i)+".example.com")
		})
	}

	close(start)
	group.Wait()

	for i := range callers {
		require.NoErrorf(t, hitErrs[i], "hit %d", i)
		require.NoErrorf(t, missErrs[i], "miss %d", i)
		assert.Equalf(t, cached, hits[i], "hit %d", i)
		assert.Equalf(t, discovery, misses[i], "miss %d", i)
	}
}
