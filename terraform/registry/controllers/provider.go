package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"runtime"
	"strconv"

	"github.com/gruntwork-io/terragrunt/terraform/registry/handlers"
	"github.com/gruntwork-io/terragrunt/terraform/registry/models"
	"github.com/gruntwork-io/terragrunt/terraform/registry/router"
	"github.com/gruntwork-io/terragrunt/terraform/registry/services"
	"github.com/labstack/echo/v4"
)

const (
	porviderName = "providers.v1"
	providerPath = "/providers"

	PlatformNameCacheProvider           = "cache_provider"
	PlatformNameCacheProviderAndArchive = "cache_providerandarchive"

	HTTPStatusCacheProcessing = http.StatusLocked

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

func (controller *ProviderController) Prefix() string {
	return controller.basePath
}

// Endpoints implements controllers.Endpointer.Endpoints
func (controller *ProviderController) Endpoints() map[string]any {
	return map[string]any{porviderName: controller.basePath}
}

// Paths implements router.Controller.Register
func (controller *ProviderController) Register(router *router.Router) {
	router = router.Group(providerPath)
	controller.basePath = router.Prefix()

	if controller.Authorization != nil {
		router.Use(controller.Authorization.MiddlewareFunc())
	}

	// Api should be compliant with the Terraform Registry Protocol for providers.
	// https://www.terraform.io/docs/internals/provider-registry-protocol.html#find-a-provider-package

	// Provider Versions
	router.GET("/:registry_name/:namespace/:name/versions", controller.versionsAction)

	// Find a Provider Package
	router.GET("/:registry_name/:namespace/:name/:version/download/:os/:arch", controller.findProviderAction)
}

func (controller *ProviderController) versionsAction(ctx echo.Context) error {
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

func (controller *ProviderController) findProviderAction(ctx echo.Context) (err error) {
	var (
		registryName = ctx.Param("registry_name")
		namespace    = ctx.Param("namespace")
		name         = ctx.Param("name")
		version      = ctx.Param("version")
		os           = ctx.Param("os")
		arch         = ctx.Param("arch")

		proxyURL = controller.Downloader.ProviderURL()

		needCache        bool
		needCacheArchive bool
	)

	provider := &models.Provider{
		RegistryName: registryName,
		Namespace:    namespace,
		Name:         name,
		Version:      version,
		OS:           os,
		Arch:         arch,
	}

	switch provider.Platform() {
	case PlatformNameCacheProviderAndArchive:
		needCacheArchive = true
		fallthrough
	case PlatformNameCacheProvider:
		needCache = true

		provider.OS = runtime.GOOS
		provider.Arch = runtime.GOARCH
	}

	return controller.ReverseProxy.
		WithRewrite(func(req *httputil.ProxyRequest) {
			// Remove all encoding parameters, otherwise we will not be able to modify the body response without decoding.
			req.Out.Header.Del("Accept-Encoding")
		}).
		WithModifyResponse(func(resp *http.Response) error {
			var body map[string]json.RawMessage

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

					if name == ProviderDownloadURLName {
						provider.DownloadURL = linkURL.ResolveReference(new(url.URL))
					}

					// Modify link to htpp://{localhost_host}/downloads/provider/{remote_host}/{remote_path}
					linkURL.Path = path.Join(proxyURL.Path, linkURL.Host, linkURL.Path)
					linkURL.Scheme = proxyURL.Scheme
					linkURL.Host = proxyURL.Host

					link = strconv.Quote(linkURL.String())
					body[string(name)] = []byte(link)
				}

				if needCache {
					controller.ProviderService.CacheProvider(provider, needCacheArchive)
					return ctx.NoContent(HTTPStatusCacheProcessing)
				}
				return nil
			})
		}).
		NewRequest(ctx, provider.PackageURL())
}
