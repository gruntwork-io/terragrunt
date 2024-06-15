package handlers

import (
	"github.com/gruntwork-io/terragrunt/terraform/cache/models"
	"github.com/gruntwork-io/terragrunt/terraform/cache/router"
	"github.com/labstack/echo/v4"
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

type ProviderHandler interface {
	// CanHandleProvider returns true if the given provider can be handled by this handler.
	CanHandleProvider(provider *models.Provider) bool

	// GetVersions serves a request that returns all versions for a single provider.
	GetVersions(ctx echo.Context, provider *models.Provider) error

	// GetPlatform serves a request that returns a provider for a specific platform.
	GetPlatform(ctx echo.Context, provider *models.Provider, downloaderController router.Controller, cacheRequestID string) error

	// Download serves a request to download the target file.
	Download(ctx echo.Context, provider *models.Provider) error
}

type CommonProviderHandler struct {
	// includeProviders and excludeProviders are sets of provider matching patterns that together define which providers are eligible to be potentially installed from the corresponding Source.
	includeProviders models.Providers
	excludeProviders models.Providers
}

func NewCommonProviderHandler(includes, excludes *[]string) *CommonProviderHandler {
	var includeProviders, excludeProviders models.Providers

	if includes != nil {
		includeProviders = models.ParseProviders(*includes...)
	}

	if excludes != nil {
		excludeProviders = models.ParseProviders(*excludes...)
	}

	return &CommonProviderHandler{
		includeProviders: includeProviders,
		excludeProviders: excludeProviders,
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
