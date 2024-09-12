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
	downloadPath = "/downloads"
)

type DownloaderController struct {
	*router.Router

	ProviderHandlers []handlers.ProviderHandler
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

	for _, handler := range controller.ProviderHandlers {
		if handler.CanHandleProvider(provider) {
			if err := handler.Download(ctx, provider); err == nil {
				break
			}
		}
	}

	return ctx.NoContent(http.StatusNotFound)
}
