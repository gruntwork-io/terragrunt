package controllers

import (
	"net/url"
	"path"

	"github.com/gruntwork-io/terragrunt/terraform/registry/handlers"
	"github.com/gruntwork-io/terragrunt/terraform/registry/models"
	"github.com/gruntwork-io/terragrunt/terraform/registry/router"
	"github.com/gruntwork-io/terragrunt/terraform/registry/services"
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

func (controller *DownloaderController) ProviderURL() *url.URL {
	proxyURL := *controller.ReverseProxy.ServerURL
	proxyURL.Path = path.Join(proxyURL.Path, controller.basePath, downloadProviderPath)
	return &proxyURL
}

// Paths implements router.Controller.Register
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
		return ctx.File(cache.Filename)
	}

	return controller.ReverseProxy.NewRequest(ctx, provider.DownloadURL)
}
