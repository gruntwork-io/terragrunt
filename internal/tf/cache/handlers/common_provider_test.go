package handlers_test

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
