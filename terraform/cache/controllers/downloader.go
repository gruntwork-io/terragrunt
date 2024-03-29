package controllers

import (
	"net/url"
	"path"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform/cache/handlers"
	"github.com/gruntwork-io/terragrunt/terraform/cache/models"
	"github.com/gruntwork-io/terragrunt/terraform/cache/router"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
	"github.com/labstack/echo/v4"
)

const (
	downloadPath         = "/downloads"
	downloadProviderPath = "/provider"
)

type DownloaderController struct {
	ReverseProxy    *handlers.ReverseProxy
	ProviderService *services.ProviderService

	basePath string
}

// ProviderProxyURL returns URL for using as a cache to download remote archives through this controller with caching
func (controller *DownloaderController) ProviderProxyURL() *url.URL {
	cacheURL := *controller.ReverseProxy.ServerURL
	cacheURL.Path = path.Join(cacheURL.Path, controller.basePath, downloadProviderPath)
	return &cacheURL
}

// Register implements router.Controller.Register
func (controller *DownloaderController) Register(router *router.Router) {
	router = router.Group(downloadPath)
	controller.basePath = router.Prefix()

	// Download provider
	router.GET(downloadProviderPath+"/:remote_host/:remote_path", controller.downloadProviderAction)
}

func (controller *DownloaderController) downloadProviderAction(ctx echo.Context) error {
	var (
		remoteHost = ctx.Param("remote_host")
		remotePath = ctx.Param("remote_path")
	)

	provider := &models.Provider{
		DownloadURL: &url.URL{
			Scheme: "https",
			Host:   remoteHost,
			Path:   "/" + remotePath,
		},
	}

	if cache := controller.ProviderService.GetProviderCache(provider); cache != nil {
		log.Debugf("Provider %q uses cached file %q", cache.Provider, cache.Archive())
		return ctx.File(cache.Archive())
	}

	return controller.ReverseProxy.NewRequest(ctx, provider.DownloadURL)
}