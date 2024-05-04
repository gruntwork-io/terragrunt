package controllers

import (
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
	Authorization *handlers.Authorization

	RegistryHandler      *handlers.Registry
	NetworkMirrorHandler *handlers.NetworkMirror

	basePath string
}

func (controller *ProviderController) Path() string {
	return controller.basePath
}

// Endpoints implements controllers.Endpointer.Endpoints
func (controller *ProviderController) Endpoints() map[string]any {
	return map[string]any{providerName: controller.basePath}
}

// Register implements router.Controller.Register
func (controller *ProviderController) Register(router *router.Router) {
	router = router.Group(providerPath)
	controller.basePath = router.Prefix()

	if controller.Authorization != nil {
		router.Use(controller.Authorization.MiddlewareFunc())
	}

	// Api should be compliant with the Terraform Registry Protocol for providers.
	// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/provider-versions-platforms

	// Get All Versions for a Single Provider
	// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/provider-versions-platforms#get-all-versions-for-a-single-provider
	router.GET("/:network_mirror_url/:cache_request_id/:registry_name/:namespace/:name/versions", controller.getVersionsAction)

	// Get a Platform
	// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/provider-versions-platforms#get-a-platform
	router.GET("/:network_mirror_url/:cache_request_id/:registry_name/:namespace/:name/:version/download/:os/:arch", controller.getPlatformsAction)
}

func (controller *ProviderController) getVersionsAction(ctx echo.Context) error {
	var (
		networkMirrorURL = ctx.Param("network_mirror_url")
		registryName     = ctx.Param("registry_name")
		namespace        = ctx.Param("namespace")
		name             = ctx.Param("name")
	)

	provider := &models.Provider{
		RegistryName: registryName,
		Namespace:    namespace,
		Name:         name,
	}

	if networkMirrorURL != "" {
		networkMirrorURL, err := url.QueryUnescape(networkMirrorURL)
		if err != nil {
			return err
		}
		return controller.NetworkMirrorHandler.GetVersions(ctx, networkMirrorURL, provider)
	}

	return controller.RegistryHandler.GetVersions(ctx, provider)

}

func (controller *ProviderController) getPlatformsAction(ctx echo.Context) (er error) {
	var (
		networkMirrorURL = ctx.Param("network_mirror_url")
		registryName     = ctx.Param("registry_name")
		namespace        = ctx.Param("namespace")
		name             = ctx.Param("name")
		version          = ctx.Param("version")
		os               = ctx.Param("os")
		arch             = ctx.Param("arch")
		cacheRequestID   = ctx.Param("cache_request_id")
	)

	provider := &models.Provider{
		RegistryName: registryName,
		Namespace:    namespace,
		Name:         name,
		Version:      version,
		OS:           os,
		Arch:         arch,
	}

	if networkMirrorURL != "" {
		networkMirrorURL, err := url.QueryUnescape(networkMirrorURL)
		if err != nil {
			return err
		}
		return controller.NetworkMirrorHandler.GetPlatfrom(ctx, networkMirrorURL, provider, cacheRequestID)
	}

	return controller.RegistryHandler.GetPlatfrom(ctx, provider, cacheRequestID)
}
