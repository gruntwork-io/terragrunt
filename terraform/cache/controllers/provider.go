package controllers

import (
	"net/http"
	"net/url"

	"github.com/gruntwork-io/terragrunt/terraform/cache/handlers"
	"github.com/gruntwork-io/terragrunt/terraform/cache/models"
	"github.com/gruntwork-io/terragrunt/terraform/cache/router"
	"github.com/labstack/echo/v4"
)

const (
	// name using for the discovery
	providerName = "providers.v1"
	// URL path to this controller
	providerPath = "/providers"
)

type ProviderController struct {
	*router.Router

	Server               http.Server
	DownloaderController router.Controller

	AuthMiddleware   echo.MiddlewareFunc
	ProviderHandlers []handlers.ProviderHandler
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
	controller.GET("/:cache_request_id/:registry_prefix/:registry_name/:namespace/:name/versions", controller.getVersionsAction)

	// Get a Platform
	// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/provider-versions-platforms#get-a-platform
	controller.GET("/:cache_request_id/:registry_prefix/:registry_name/:namespace/:name/:version/download/:os/:arch", controller.getPlatformsAction)
}

func (controller *ProviderController) getVersionsAction(ctx echo.Context) error {
	var (
		registryPrefix = ctx.Param("registry_prefix")
		registryName   = ctx.Param("registry_name")
		namespace      = ctx.Param("namespace")
		name           = ctx.Param("name")
	)

	registryPrefix, err := url.QueryUnescape(registryPrefix)
	if err != nil {
		return err
	}

	provider := &models.Provider{
		RegistryPrefix: registryPrefix,
		RegistryName:   registryName,
		Namespace:      namespace,
		Name:           name,
	}

	for _, handler := range controller.ProviderHandlers {
		if handler.CanHandleProvider(provider) {
			if err := handler.GetVersions(ctx, provider); err == nil {
				break
			}
		}
	}
	return ctx.NoContent(http.StatusNotFound)
}

func (controller *ProviderController) getPlatformsAction(ctx echo.Context) (er error) {
	var (
		registryPrefix = ctx.Param("registry_prefix")
		registryName   = ctx.Param("registry_name")
		namespace      = ctx.Param("namespace")
		name           = ctx.Param("name")
		version        = ctx.Param("version")
		os             = ctx.Param("os")
		arch           = ctx.Param("arch")
		cacheRequestID = ctx.Param("cache_request_id")
	)

	registryPrefix, err := url.QueryUnescape(registryPrefix)
	if err != nil {
		return err
	}

	provider := &models.Provider{
		RegistryPrefix: registryPrefix,
		RegistryName:   registryName,
		Namespace:      namespace,
		Name:           name,
		Version:        version,
		OS:             os,
		Arch:           arch,
	}

	for _, handler := range controller.ProviderHandlers {
		if handler.CanHandleProvider(provider) {
			if err := handler.GetPlatform(ctx, provider, controller.DownloaderController, cacheRequestID); err == nil {
				break
			}
		}
	}
	return ctx.NoContent(http.StatusNotFound)
}
