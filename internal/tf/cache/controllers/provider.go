// Package controllers provides the implementation of the controller for the provider endpoints.
package controllers

import (
	"net/http"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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
	AuthMiddleware              router.MiddlewareFunc
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
	//
	// Each endpoint is registered twice: under a cache request id, which makes
	// the server cache the provider, and without one, which makes it proxy.

	// Get All Versions for a Single Provider
	// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/
	// provider-versions-platforms#get-all-versions-for-a-single-provider
	controller.GET(
		"/{cache_request_id}/{registry_name}/{namespace}/{name}/versions",
		controller.getVersionsAction,
	)
	controller.GET(
		"/{registry_name}/{namespace}/{name}/versions",
		controller.getVersionsAction,
	)

	// Get a Platform
	// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/
	// provider-versions-platforms#get-a-platform
	controller.GET(
		"/{cache_request_id}/{registry_name}/{namespace}/{name}/{version}/download/{os}/{arch}",
		controller.getPlatformsAction,
	)
	controller.GET(
		"/{registry_name}/{namespace}/{name}/{version}/download/{os}/{arch}",
		controller.getPlatformsAction,
	)
}

func (controller *ProviderController) getVersionsAction(w router.ResponseWriter, r *http.Request) error {
	var (
		registryName = r.PathValue("registry_name")
		namespace    = r.PathValue("namespace")
		name         = r.PathValue("name")
	)

	provider := &models.Provider{
		RegistryName: registryName,
		Namespace:    namespace,
		Name:         name,
	}

	var allVersions models.Versions

	for _, handler := range controller.ProviderHandlers {
		if handler.CanHandleProvider(provider) {
			versions, err := handler.GetVersions(r.Context(), provider)
			if err != nil {
				controller.Logger.Errorf(
					"Failed to get provider versions from %q: %s",
					handler,
					err.Error(),
				)
			}

			if versions != nil {
				allVersions = append(allVersions, versions...)
			}
		}
	}

	validVersions, invalidVersions := allVersions.FilterValid()
	for _, v := range invalidVersions {
		controller.Logger.Warnf(
			"Skipping invalid version %q for provider %s",
			v,
			provider.Address(),
		)
	}

	versions := struct {
		ID       string          `json:"id"`
		Versions models.Versions `json:"versions"`
	}{
		ID:       provider.Address(),
		Versions: validVersions,
	}

	return router.JSON(w, http.StatusOK, versions)
}

func (controller *ProviderController) getPlatformsAction(w router.ResponseWriter, r *http.Request) error {
	var (
		registryName   = r.PathValue("registry_name")
		namespace      = r.PathValue("namespace")
		name           = r.PathValue("name")
		version        = r.PathValue("version")
		os             = r.PathValue("os")
		arch           = r.PathValue("arch")
		cacheRequestID = r.PathValue("cache_request_id")
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
		return controller.ProxyProviderHandler.GetPlatform(
			w,
			r,
			provider,
			controller.DownloaderController,
		)
	}

	var (
		resp *models.ResponseBody
		err  error
	)

	for _, handler := range controller.ProviderHandlers {
		if handler.CanHandleProvider(provider) {
			resp, err = handler.GetPlatform(r.Context(), provider)
			if err != nil {
				controller.Logger.Errorf(
					"Failed to get provider platform from %q: %s",
					handler,
					err.Error(),
				)
			}

			if resp != nil {
				break
			}
		}
	}

	provider.ResponseBody = resp

	// start caching and return 423 status
	controller.ProviderService.CacheProvider(r.Context(), cacheRequestID, provider)

	w.WriteHeader(controller.CacheProviderHTTPStatusCode)

	return nil
}
