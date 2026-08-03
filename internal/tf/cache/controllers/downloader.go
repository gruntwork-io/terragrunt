package controllers

import (
	"crypto/rand"
	"net/http"
	"net/url"
	"path"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/labstack/echo/v4"
)

const (
	downloadPath = "/downloads"
)

type DownloaderController struct {
	*router.Router

	ProviderService      *services.ProviderService
	ProxyProviderHandler *handlers.ProxyProviderHandler

	segment string
}

// NewDownloaderController returns a controller whose download route is reachable
// only through a secret path segment, generated fresh for every server.
//
// OpenTofu/Terraform fetch a provider archive from an ordinary URL rather than a
// service endpoint, so they send no credentials with it and the path is the only
// channel that can carry a secret. The segment is independent of the cache server
// token so that a download URL surfacing in a log or a debug trace reveals nothing
// about the token guarding the rest of the API.
func NewDownloaderController(
	providerService *services.ProviderService,
	proxyProviderHandler *handlers.ProxyProviderHandler,
) *DownloaderController {
	return &DownloaderController{
		ProviderService:      providerService,
		ProxyProviderHandler: proxyProviderHandler,
		segment:              rand.Text(),
	}
}

// Segment returns the secret path segment that gates the download route.
func (controller *DownloaderController) Segment() string {
	return controller.segment
}

// Register implements router.Controller.Register
func (controller *DownloaderController) Register(router *router.Router) {
	controller.Router = router.Group(path.Join(downloadPath, controller.segment))

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
			controller.ProviderService.Logger().
				Debugf("Download cached provider %s", cache.Provider)

			return ctx.File(path)
		}
	}

	if err := controller.ProxyProviderHandler.Download(ctx, provider); err != nil {
		return err
	}

	return ctx.NoContent(http.StatusNotFound)
}
