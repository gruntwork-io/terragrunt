// Package handlers provides the interfaces and common implementations for handling provider requests.
package handlers

import (
	"context"
	"sync"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/sync/singleflight"
)

// discoveryTimeout bounds one shared discovery request, tries and waits
// included. It runs apart from the callers waiting on it, and the client
// underneath carries no timeout of its own.
const discoveryTimeout = time.Minute

type CommonProviderHandler struct {
	logger     log.Logger
	httpClient vhttp.Client

	// discoveryURLCache stores discovered registry URLs
	discoveryURLCache map[string]*RegistryURLs

	// discoveries shares one discovery request per registry among the
	// callers that miss the cache at once.
	discoveries singleflight.Group

	// includeProviders and excludeProviders are sets of provider matching patterns that together define which providers are eligible to be potentially installed from the corresponding Source.
	includeProviders models.Providers
	excludeProviders models.Providers

	discoveryURLCacheMu sync.Mutex
}

// NewCommonProviderHandler returns a new `CommonProviderHandler` instance with the defined values.
// c is used for service-discovery requests; pass [vhttp.NewOSClient]
// in production or a [vhttp.NewMemClient] in tests.
func NewCommonProviderHandler(
	l log.Logger,
	c vhttp.Client,
	includes, excludes *[]string,
) *CommonProviderHandler {
	var includeProviders, excludeProviders models.Providers

	if includes != nil {
		includeProviders = models.ParseProviders(*includes...)
	}

	if excludes != nil {
		excludeProviders = models.ParseProviders(*excludes...)
	}

	return &CommonProviderHandler{
		logger:            l,
		httpClient:        c,
		includeProviders:  includeProviders,
		excludeProviders:  excludeProviders,
		discoveryURLCache: make(map[string]*RegistryURLs),
	}
}

// CanHandleProvider implements ProviderHandler.CanHandleProvider
func (handler *CommonProviderHandler) CanHandleProvider(provider *models.Provider) bool {
	switch {
	case handler.excludeProviders.Find(provider) != nil:
		return false
	case len(handler.includeProviders) > 0:
		return handler.includeProviders.Find(provider) != nil
	default:
		return true
	}
}

// SetDiscoveryURLCache pre-populates the discovery cache for a given registry.
func (handler *CommonProviderHandler) SetDiscoveryURLCache(
	registryName string,
	urls *RegistryURLs,
) {
	handler.discoveryURLCacheMu.Lock()
	defer handler.discoveryURLCacheMu.Unlock()

	handler.discoveryURLCache[registryName] = urls
}

// DiscoveryURL implements ProviderHandler.DiscoveryURL.
//
// Concurrent calls for the same registry share one discovery request, which
// is tried again while the registry refuses it for now (see
// [helpers.StatusError.Transient]). A caller whose ctx ends gets ctx.Err() at
// once. The shared request goes on for the other callers, and its answer is
// still remembered.
func (handler *CommonProviderHandler) DiscoveryURL(
	ctx context.Context,
	registryName string,
) (*RegistryURLs, error) {
	if urls, ok := handler.cachedDiscoveryURL(registryName); ok {
		return urls, nil
	}

	// The shared request must not end with the caller that happened to start
	// it, so it runs under a context that keeps ctx's values but not its
	// cancellation.
	sharedCtx := context.WithoutCancel(ctx)

	result := handler.discoveries.DoChan(registryName, func() (any, error) {
		return handler.discover(sharedCtx, registryName)
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-result:
		if res.Err != nil {
			return nil, res.Err
		}

		urls, _ := res.Val.(*RegistryURLs)

		return urls, nil
	}
}

// discover asks registryName for its service URLs, trying again while it
// refuses for now, and remembers the answer. A registry that cannot be
// reached, or that has no discovery document, gets the default URLs.
func (handler *CommonProviderHandler) discover(
	ctx context.Context,
	registryName string,
) (*RegistryURLs, error) {
	// A caller that missed the cache just before the previous discovery of
	// this registry stored its answer starts a discovery of its own.
	if urls, ok := handler.cachedDiscoveryURL(registryName); ok {
		return urls, nil
	}

	ctx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	defer cancel()

	var urls *RegistryURLs

	err := helpers.RetryRefusals(ctx, func(ctx context.Context) error {
		var err error

		urls, err = DiscoveryURL(ctx, handler.httpClient, registryName)

		return err
	})
	if err != nil {
		if !IsOfflineError(err) {
			return nil, err
		}

		urls = DefaultRegistryURLs
		handler.logger.Debugf(
			"Unable to discover %q registry URLs, reason: %q, use default URLs: %s",
			registryName,
			err,
			urls,
		)
	} else {
		handler.logger.Debugf("Discovered %q registry URLs: %s", registryName, urls)
	}

	handler.SetDiscoveryURLCache(registryName, urls)

	return urls, nil
}

func (handler *CommonProviderHandler) cachedDiscoveryURL(registryName string) (*RegistryURLs, bool) {
	handler.discoveryURLCacheMu.Lock()
	defer handler.discoveryURLCacheMu.Unlock()

	urls, ok := handler.discoveryURLCache[registryName]

	return urls, ok
}
