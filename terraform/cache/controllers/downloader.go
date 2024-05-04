package controllers

import (
	"net/url"
	"path"

	"github.com/gruntwork-io/terragrunt/terraform/cache/handlers"
	"github.com/gruntwork-io/terragrunt/terraform/cache/router"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
	"github.com/labstack/echo/v4"
)

const (
	downloadPath         = "/downloads"
	downloadProviderPath = "/provider"
)

type DownloaderController struct {
	RegistryHandler *handlers.Registry
	ProviderService *services.ProviderService

	basePath string
}

// Register implements router.Controller.Register
func (controller *DownloaderController) Register(router *router.Router) {
	router = router.Group(downloadPath)
	controller.basePath = router.Prefix()
	controller.RegistryHandler.SetDownloadURLPath(path.Join(controller.basePath, downloadProviderPath))

	// Download provider
	router.GET(downloadProviderPath+"/:remote_host/:remote_path", controller.downloadProviderAction)
}

func (controller *DownloaderController) downloadProviderAction(ctx echo.Context) error {
	var (
		remoteHost = ctx.Param("remote_host")
		remotePath = ctx.Param("remote_path")
	)

	downloadURL := &url.URL{
		Scheme: "https",
		Host:   remoteHost,
		Path:   "/" + remotePath,
	}

	return controller.RegistryHandler.Download(ctx, downloadURL)
}
