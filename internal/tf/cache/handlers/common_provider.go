// Package handlers provides the interfaces and common implementations for handling provider requests.
package handlers

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/puzpuzpuz/xsync/v3"
)

type CommonProviderHandler struct {
	logger     log.Logger
	httpClient vhttp.Client

	// discoveryURLCache stores discovered registry URLs
	// We use [xsync.MapOf](https://github.com/puzpuzpuz/xsync?tab=readme-ov-file#map)
	// instead of standard `sync.Map` since it's faster and has generic types.
	discoveryURLCache *xsync.MapOf[string, *RegistryURLs]

	// includeProviders and excludeProviders are sets of provider matching patterns that together define which providers are eligible to be potentially installed from the corresponding Source.
	includeProviders models.Providers
	excludeProviders models.Providers
}

// NewCommonProviderHandler returns a new `CommonProviderHandler` instance with the defined values.
// httpClient is used for service-discovery requests; pass [vhttp.NewOSClient]
// in production or a [vhttp.NewMemClient] in tests.
func NewCommonProviderHandler(l log.Logger, httpClient vhttp.Client, includes, excludes *[]string) *CommonProviderHandler {
	var includeProviders, excludeProviders models.Providers

	if includes != nil {
		includeProviders = models.ParseProviders(*includes...)
	}

	if excludes != nil {
		excludeProviders = models.ParseProviders(*excludes...)
	}

	return &CommonProviderHandler{
		logger:            l,
		httpClient:        httpClient,
		includeProviders:  includeProviders,
		excludeProviders:  excludeProviders,
		discoveryURLCache: xsync.NewMapOf[string, *RegistryURLs](),
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
func (handler *CommonProviderHandler) SetDiscoveryURLCache(registryName string, urls *RegistryURLs) {
	handler.discoveryURLCache.Store(registryName, urls)
}

// DiscoveryURL implements ProviderHandler.DiscoveryURL.
func (handler *CommonProviderHandler) DiscoveryURL(ctx context.Context, registryName string) (*RegistryURLs, error) {
	if urls, ok := handler.discoveryURLCache.Load(registryName); ok {
		return urls, nil
	}

	urls, err := DiscoveryURL(ctx, handler.httpClient, registryName)
	if err != nil {
		if !IsOfflineError(err) {
			return nil, err
		}

		urls = DefaultRegistryURLs
		handler.logger.Debugf("Unable to discover %q registry URLs, reason: %q, use default URLs: %s", registryName, err, urls)
	} else {
		handler.logger.Debugf("Discovered %q registry URLs: %s", registryName, urls)
	}

	handler.discoveryURLCache.Store(registryName, urls)

	return urls, nil
}
