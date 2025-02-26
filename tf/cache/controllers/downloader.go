package controllers

import (
	"net/http"
	"net/url"

	"github.com/gruntwork-io/terragrunt/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/tf/cache/services"
	"github.com/labstack/echo/v4"
)

const (
	downloadPath = "/downloads"
)

type DownloaderController struct {
	*router.Router

	ProviderService      *services.ProviderService
	ProxyProviderHandler *handlers.ProxyProviderHandler
}

// Register implements router.Controller.Register
func (controller *DownloaderController) Register(router *router.Router) {
	controller.Router = router.Group(downloadPath)

	// Download provider
	controller.GET("/:remote_host/:remote_path", controller.downloadProviderAction)
}

func (controller *DownloaderController) downloadProviderAction(ctx echo.Context) error {
	var (
		remoteHost = ctx.Param("remote_host")
		remotePath = ctx.Param("remote_path")
	)

	downloadURL := url.URL{
		Scheme: "https",
		Host:   remoteHost,
		Path:   "/" + remotePath,
	}
	provider := &models.Provider{
		ResponseBody: &models.ResponseBody{
			DownloadURL: downloadURL.String(),
		},
	}

	if cache := controller.ProviderService.GetProviderCache(provider); cache != nil {
		if path := cache.ArchivePath(); path != "" {
			controller.ProviderService.Logger().Debugf("Download cached provider %s", cache.Provider)
			return ctx.File(path)
		}
	}

	if err := controller.ProxyProviderHandler.Download(ctx, provider); err != nil {
		return err
	}

	return ctx.NoContent(http.StatusNotFound)
}
