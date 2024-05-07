package handlers

import (
	"github.com/gruntwork-io/terragrunt/terraform/cache/models"
	"github.com/labstack/echo/v4"
)

type ProviderHandler interface {
	// CanHandleProvider returns true if the given provider can be handled by this handler.
	CanHandleProvider(provider *models.Provider) bool

	// GetVersions serves a request that returns all versions for a single provider.
	GetVersions(ctx echo.Context, provider *models.Provider) error

	// GetPlatfrom serves a request that returns a provider for a specific platform.
	GetPlatfrom(ctx echo.Context, provider *models.Provider, downloaderPrefix, cacheRequestID string) error

	// Download serves a request to download the target file.
	Download(ctx echo.Context, provider *models.Provider) error
}
