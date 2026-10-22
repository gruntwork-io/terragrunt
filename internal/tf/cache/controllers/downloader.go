package controllers

import (
	"crypto/rand"
	"errors"
	"net/http"
	"net/url"
	"path"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
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

	// Wildcard route so multi-segment S3/GCS bucket paths match correctly.
	controller.GET("/{remote_host}/{rest...}", controller.downloadProviderAction)
}

func (controller *DownloaderController) downloadProviderAction(w router.ResponseWriter, r *http.Request) error {
	var (
		remoteHost = r.PathValue("remote_host")
		remotePath = r.PathValue("rest")
	)

	// Forward query string from the inbound request so signed URLs (e.g. S3 pre-signed) keep their auth parameters.
	downloadURL := url.URL{
		Scheme:   "https",
		Host:     remoteHost,
		Path:     "/" + remotePath,
		RawQuery: r.URL.RawQuery,
	}

	provider := &models.Provider{
		ResponseBody: &models.ResponseBody{
			DownloadURL: downloadURL.String(),
		},
	}

	if cache := controller.ProviderService.GetProviderCache(provider); cache != nil {
		if archivePath := cache.ArchivePath(); archivePath != "" {
			controller.ProviderService.Logger().
				Debugf("Download cached provider %s", cache.Provider)

			return serveFile(w, r, controller.ProviderService.FS(), archivePath)
		}
	}

	return controller.ProxyProviderHandler.Download(w, r, provider)
}

// serveFile answers with the file at name on fsys, or a 404 when it cannot be
// opened.
func serveFile(w router.ResponseWriter, r *http.Request, fsys vfs.FS, name string) (err error) {
	f, err := fsys.Open(name)
	if err != nil {
		return router.NewHTTPError(http.StatusNotFound)
	}

	defer func() {
		err = errors.Join(err, f.Close())
	}()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	http.ServeContent(w, r, info.Name(), info.ModTime(), f)

	return nil
}
