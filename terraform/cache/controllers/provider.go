package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strconv"

	"github.com/gruntwork-io/terragrunt/terraform/cache/handlers"
	"github.com/gruntwork-io/terragrunt/terraform/cache/models"
	"github.com/gruntwork-io/terragrunt/terraform/cache/router"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
	"github.com/gruntwork-io/terragrunt/terraform/getproviders"
	"github.com/labstack/echo/v4"
)

const (
	// name using for the discovery
	providerName = "providers.v1"
	// URL path to this controller
	providerPath = "/providers"

	// The platform name used to request provider caching, e.g `terraform providers block -platform=cache_provider`
	PlatformNameCacheProvider = "cache_provider"
	// The status returned when making a request to the caching provider.
	// It is needed to prevent further loading of providers by terraform, and at the same time make sure that the request was processed successfully.
	HTTPStatusCacheProvider = http.StatusLocked

	// Provider's assets consist of three files/URLs: zipped binary, hashes and signature
	ProviderDownloadURLName         ProviderURLName = "download_url"
	ProviderSHASumsURLName          ProviderURLName = "shasums_url"
	ProviderSHASumsSignatureURLName ProviderURLName = "shasums_signature_url"
)

var (
	// ProviderURLNames contains urls that must be modified to forward terraform requests through this server.
	ProviderURLNames = []ProviderURLName{
		ProviderDownloadURLName,
		ProviderSHASumsURLName,
		ProviderSHASumsSignatureURLName,
	}
)

type ProviderURLName string

type ProviderController struct {
	Authorization   *handlers.Authorization
	ReverseProxy    *handlers.ReverseProxy
	Downloader      *DownloaderController
	ProviderService *services.ProviderService

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
	router.GET("/:cache_request_id/:registry_name/:namespace/:name/versions", controller.findVersionsAction)

	// Get All Platforms for a Single Version
	// https://developer.hashicorp.com/terraform/cloud-docs/api-docs/private-registry/provider-versions-platforms#get-all-platforms-for-a-single-version
	router.GET("/:cache_request_id/:registry_name/:namespace/:name/:version/download/:os/:arch", controller.findPlatformsAction)
}

func (controller *ProviderController) findVersionsAction(ctx echo.Context) error {
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
	return controller.ReverseProxy.NewRequest(ctx, provider.VersionURL())
}

func (controller *ProviderController) findPlatformsAction(ctx echo.Context) error {
	var (
		registryName   = ctx.Param("registry_name")
		namespace      = ctx.Param("namespace")
		name           = ctx.Param("name")
		version        = ctx.Param("version")
		os             = ctx.Param("os")
		arch           = ctx.Param("arch")
		cacheRequestID = ctx.Param("cache_request_id")

		proxyURL = controller.Downloader.ProviderProxyURL()
	)

	provider := &models.Provider{
		RegistryName: registryName,
		Namespace:    namespace,
		Name:         name,
		Version:      version,
		OS:           os,
		Arch:         arch,
	}

	return controller.ReverseProxy.
		WithRewrite(func(req *httputil.ProxyRequest) {
			// Remove all encoding parameters, otherwise we will not be able to modify the body response without decoding.
			req.Out.Header.Del("Accept-Encoding")
		}).
		WithModifyResponse(func(resp *http.Response) error {
			var body map[string]json.RawMessage

			if cacheRequestID != "" {
				var responseBody = new(getproviders.Package)

				if err := handlers.DecodeJSONBody(resp, responseBody); err != nil {
					return err
				}
				provider.Package = responseBody

				controller.ProviderService.CacheProvider(ctx.Request().Context(), cacheRequestID, provider)
				return ctx.NoContent(HTTPStatusCacheProvider)
			}

			return handlers.ModifyJSONBody(resp, &body, func() error {
				for _, name := range ProviderURLNames {
					linkBytes, ok := body[string(name)]
					if !ok || linkBytes == nil {
						continue
					}
					link := string(linkBytes)

					link, err := strconv.Unquote(link)
					if err != nil {
						return err
					}

					linkURL, err := url.Parse(link)
					if err != nil {
						return err
					}

					// Modify link to htpp://{localhost_host}/downloads/provider/{remote_host}/{remote_path}
					linkURL.Path = path.Join(proxyURL.Path, linkURL.Host, linkURL.Path)
					linkURL.Scheme = proxyURL.Scheme
					linkURL.Host = proxyURL.Host

					link = strconv.Quote(linkURL.String())
					body[string(name)] = []byte(link)
				}

				return nil
			})
		}).
		NewRequest(ctx, provider.PlatformURL())
}
