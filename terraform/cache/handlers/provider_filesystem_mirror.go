package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/terraform/cache/models"
	"github.com/gruntwork-io/terragrunt/terraform/cache/router"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
	"github.com/gruntwork-io/terragrunt/terraform/cliconfig"
	"github.com/labstack/echo/v4"
)

type ProviderFilesystemMirrorHandler struct {
	*CommonProviderHandler

	providerService             *services.ProviderService
	cacheProviderHTTPStatusCode int
	filesystemMirrorPath        string
}

func NewProviderFilesystemMirrorHandler(providerService *services.ProviderService, cacheProviderHTTPStatusCode int, method *cliconfig.ProviderInstallationFilesystemMirror) ProviderHandler {
	return &ProviderFilesystemMirrorHandler{
		CommonProviderHandler:       NewCommonProviderHandler(method.Include, method.Exclude),
		providerService:             providerService,
		cacheProviderHTTPStatusCode: cacheProviderHTTPStatusCode,
		filesystemMirrorPath:        method.Path,
	}
}

func (handler *ProviderFilesystemMirrorHandler) String() string {
	return "filesystem mirror handler "
}

// GetVersions implements ProviderHandler.GetVersions
func (handler *ProviderFilesystemMirrorHandler) GetVersions(ctx echo.Context, provider *models.Provider) error {
	var mirrorData struct {
		Versions map[string]struct{} `json:"versions"`
	}

	filename := filepath.Join(provider.RegistryName, provider.Namespace, provider.Name, "index.json")
	if err := handler.readMirrorData(filename, &mirrorData); err != nil {
		return err
	}

	versions := struct {
		ID       string           `json:"id"`
		Versions []models.Version `json:"versions"`
	}{
		ID: provider.Address(),
	}

	for version := range mirrorData.Versions {
		versions.Versions = append(versions.Versions, models.Version{
			Version:   version,
			Platforms: availablePlatforms,
		})
	}

	return ctx.JSON(http.StatusOK, versions)
}

// GetPlatform implements ProviderHandler.GetPlatform
func (handler *ProviderFilesystemMirrorHandler) GetPlatform(ctx echo.Context, provider *models.Provider, downloaderController router.Controller, cacheRequestID string) error {
	if cacheRequestID == "" {
		// it is impossible to return all platform properties from the filesystem mirror, return 404 status
		return ctx.NoContent(http.StatusNotFound)
	}

	var mirrorData struct {
		Archives map[string]struct {
			URL    string   `json:"url"`
			Hashes []string `json:"hashes"`
		} `json:"archives"`
	}

	filename := filepath.Join(provider.RegistryName, provider.Namespace, provider.Name, provider.Version+".json")
	if err := handler.readMirrorData(filename, &mirrorData); err != nil {
		return err
	}

	if archive, ok := mirrorData.Archives[provider.Platform()]; ok {
		// check if the URL contains http scheme, it may just be a filename and we need to build the URL
		if !strings.Contains(archive.URL, "://") {
			archive.URL = filepath.Join(handler.filesystemMirrorPath, provider.RegistryName, provider.Namespace, provider.Name, archive.URL)
		}

		provider.ResponseBody = &models.ResponseBody{
			Filename:    filepath.Base(archive.URL),
			DownloadURL: archive.URL,
		}
	} else {
		return ctx.NoContent(http.StatusNotFound)
	}

	// start caching and return 423 status
	handler.providerService.CacheProvider(ctx.Request().Context(), cacheRequestID, provider)
	return ctx.NoContent(handler.cacheProviderHTTPStatusCode)
}

// Download implements ProviderHandler.Download
func (handler *ProviderFilesystemMirrorHandler) Download(ctx echo.Context, provider *models.Provider) error {
	return ctx.NoContent(http.StatusNotImplemented)
}

func (handler *ProviderFilesystemMirrorHandler) readMirrorData(filename string, value any) error {
	filename = filepath.Join(handler.filesystemMirrorPath, filename)

	data, err := os.ReadFile(filename)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	if err := json.Unmarshal(data, value); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}
