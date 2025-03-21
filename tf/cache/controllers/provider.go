// Package controllers provides the implementation of the controller for the provider endpoints.
package controllers

import (
	"net/http"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/tf/cache/services"
	"github.com/labstack/echo/v4"
)

const (
	// name using for the discovery
	providerName = "providers.v1"
	// URL path to this controller
	providerPath = "/providers"
)

type ProviderController struct {
	Logger               log.Logger
	DownloaderController router.Controller
	*router.Router
	AuthMiddleware              echo.MiddlewareFunc
	ProxyProviderHandler        *handlers.ProxyProviderHandler
	ProviderService             *services.ProviderService
	ProviderHandlers            []handlers.ProviderHandler
	Server                      http.Server
	CacheProviderHTTPStatusCode int
}

// Endpoints implements controllers.Endpointer.Endpoints
func (controller *ProviderController) Endpoints() map[string]any {
	return map[string]any{providerName: controller.URL().Path}
}

// Register implements router.Controller.Register
func (controller *ProviderController) Register(router *router.Router) {
	controller.Router = router.Group(providerPath)

	if controller.AuthMiddleware != nil {
		controller.Use(controller.AuthMiddleware)
	}

	// Api should be compliant with the Terraform Registry Protocol for providers.
	// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/provider-versions-platforms

	// Get All Versions for a Single Provider
	// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/provider-versions-platforms#get-all-versions-for-a-single-provider
	controller.GET("/:cache_request_id/:registry_name/:namespace/:name/versions", controller.getVersionsAction)

	// Get a Platform
	// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/provider-versions-platforms#get-a-platform
	controller.GET("/:cache_request_id/:registry_name/:namespace/:name/:version/download/:os/:arch", controller.getPlatformsAction)
}

func (controller *ProviderController) getVersionsAction(ctx echo.Context) error {
	var (
		registryName = ctx.Param("registry_name")
		namespace    = ctx.Param("namespace")
		name         = ctx.Param("name")
	)

	provider := &models.Provider{
		RegistryName: registryName,
		Namespace:    namespace,
		Name:         name,
	}

	var allVersions models.Versions

	for _, handler := range controller.ProviderHandlers {
		if handler.CanHandleProvider(provider) {
			versions, err := handler.GetVersions(ctx.Request().Context(), provider)
			if err != nil {
				controller.Logger.Errorf("Failed to get provider versions from %q: %s", handler, err.Error())
			}

			if versions != nil {
				allVersions = append(allVersions, versions...)
			}
		}
	}

	versions := struct {
		ID       string          `json:"id"`
		Versions models.Versions `json:"versions"`
	}{
		ID:       provider.Address(),
		Versions: allVersions,
	}

	return ctx.JSON(http.StatusOK, versions)
}

func (controller *ProviderController) getPlatformsAction(ctx echo.Context) (er error) {
	var (
		registryName   = ctx.Param("registry_name")
		namespace      = ctx.Param("namespace")
		name           = ctx.Param("name")
		version        = ctx.Param("version")
		os             = ctx.Param("os")
		arch           = ctx.Param("arch")
		cacheRequestID = ctx.Param("cache_request_id")
	)

	provider := &models.Provider{
		RegistryName: registryName,
		Namespace:    namespace,
		Name:         name,
		Version:      version,
		OS:           os,
		Arch:         arch,
	}

	if cacheRequestID == "" {
		return controller.ProxyProviderHandler.GetPlatform(ctx, provider, controller.DownloaderController)
	}

	var (
		resp *models.ResponseBody
		err  error
	)

	for _, handler := range controller.ProviderHandlers {
		if handler.CanHandleProvider(provider) {
			resp, err = handler.GetPlatform(ctx.Request().Context(), provider)
			if err != nil {
				controller.Logger.Errorf("Failed to get provider platform from %q: %s", handler, err.Error())
			}

			if resp != nil {
				break
			}
		}
	}

	provider.ResponseBody = resp

	// start caching and return 423 status
	controller.ProviderService.CacheProvider(ctx.Request().Context(), cacheRequestID, provider)

	return ctx.NoContent(controller.CacheProviderHTTPStatusCode)
}
