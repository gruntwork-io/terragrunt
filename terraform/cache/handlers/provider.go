// Package handlers provides the interfaces and common implementations for handling provider requests.
package handlers

import (
	"context"
	liberrors "errors"
	"strings"
	"syscall"

	"github.com/gruntwork-io/terragrunt/terraform/cache/models"
	"github.com/gruntwork-io/terragrunt/terraform/cache/router"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
	"github.com/labstack/echo/v4"
	"github.com/puzpuzpuz/xsync/v3"
)

var availablePlatforms []*models.Platform = []*models.Platform{
	{OS: "solaris", Arch: "amd64"},
	{OS: "openbsd", Arch: "386"},
	{OS: "openbsd", Arch: "arm"},
	{OS: "openbsd", Arch: "amd64"},
	{OS: "freebsd", Arch: "386"},
	{OS: "freebsd", Arch: "arm"},
	{OS: "freebsd", Arch: "amd64"},
	{OS: "linux", Arch: "386"},
	{OS: "linux", Arch: "arm"},
	{OS: "linux", Arch: "arm64"},
	{OS: "linux", Arch: "amd64"},
	{OS: "darwin", Arch: "amd64"},
	{OS: "darwin", Arch: "arm64"},
	{OS: "windows", Arch: "386"},
	{OS: "windows", Arch: "amd64"},
}

var offlineErrors = []error{
	syscall.ECONNREFUSED,
	syscall.ECONNRESET,
	syscall.ECONNABORTED,
	syscall.EHOSTUNREACH,
	syscall.ENETUNREACH,
	syscall.ENETDOWN,
}

// ProviderHandlers is a slice of ProviderHandler.
type ProviderHandlers []ProviderHandler

// DiscoveryURL looks for the first handler that can handle the given `registryName`,
// which is determined by the include and exclude settings in the `.terraformrc` CLI config file.
// If the handler is found, tries to discover its API endpoints otherwise return the default registry URLs.
func (handlers ProviderHandlers) DiscoveryURL(ctx context.Context, registryName string) (*RegistryURLs, error) {
	provider := models.ParseProvider(registryName)

	for _, handler := range handlers {
		if handler.CanHandleProvider(provider) {
			return handler.DiscoveryURL(ctx, registryName)
		}
	}

	return DefaultRegistryURLs, nil
}

type ProviderHandler interface {
	// CanHandleProvider returns true if the given provider can be handled by this handler.
	CanHandleProvider(provider *models.Provider) bool

	// GetVersions serves a request that returns all versions for a single provider.
	GetVersions(ctx echo.Context, provider *models.Provider) error

	// GetPlatform serves a request that returns a provider for a specific platform.
	GetPlatform(ctx echo.Context, provider *models.Provider, downloaderController router.Controller, cacheRequestID string) error

	// Download serves a request to download the target file.
	Download(ctx echo.Context, provider *models.Provider) error

	// DiscoveryURL discovers modules and providers API endpoints for the specified `registryName`.
	// https://developer.hashicorp.com/terraform/internals/remote-service-discovery#discovery-process
	DiscoveryURL(ctx context.Context, registryName string) (*RegistryURLs, error)
}

type CommonProviderHandler struct {
	providerService *services.ProviderService

	// includeProviders and excludeProviders are sets of provider matching patterns that together define which providers are eligible to be potentially installed from the corresponding Source.
	includeProviders models.Providers
	excludeProviders models.Providers

	// registryURLCache stores discovered registry URLs
	// We use [xsync.MapOf](https://github.com/puzpuzpuz/xsync?tab=readme-ov-file#map)
	// instead of standard `sync.Map` since it's faster and has generic types.
	registryURLCache *xsync.MapOf[string, *RegistryURLs]
}

// NewCommonProviderHandler returns a new `CommonProviderHandler` instance with the defined values.
func NewCommonProviderHandler(providerService *services.ProviderService, includes, excludes *[]string) *CommonProviderHandler {
	var includeProviders, excludeProviders models.Providers

	if includes != nil {
		includeProviders = models.ParseProviders(*includes...)
	}

	if excludes != nil {
		excludeProviders = models.ParseProviders(*excludes...)
	}

	return &CommonProviderHandler{
		providerService:  providerService,
		includeProviders: includeProviders,
		excludeProviders: excludeProviders,
		registryURLCache: xsync.NewMapOf[string, *RegistryURLs](),
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

// DiscoveryURL implements ProviderHandler.DiscoveryURL.
func (handler *CommonProviderHandler) DiscoveryURL(ctx context.Context, registryName string) (*RegistryURLs, error) {
	if urls, ok := handler.registryURLCache.Load(registryName); ok {
		return urls, nil
	}

	urls, err := DiscoveryURL(ctx, registryName)
	if err != nil {
		if !IsOfflineError(err) {
			return nil, err
		}

		urls = DefaultRegistryURLs
		handler.providerService.Logger().Debugf("Unable to discover %q registry URLs, reason: %q, use default URLs: %s", registryName, err, urls)
	} else {
		handler.providerService.Logger().Debugf("Discovered %q registry URLs: %s", registryName, urls)
	}

	handler.registryURLCache.Store(registryName, urls)

	return urls, nil
}

// IsOfflineError returns true if the given error is an offline error and can be use default URL.
func IsOfflineError(err error) bool {
	if liberrors.As(err, &NotFoundWellKnownURL{}) {
		return true
	}

	for _, connErr := range offlineErrors {
		if liberrors.Is(err, connErr) || strings.Contains(err.Error(), connErr.Error()) {
			return true
		}
	}

	return false
}
