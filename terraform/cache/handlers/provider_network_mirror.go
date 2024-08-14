package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/terraform/cache/helpers"
	"github.com/gruntwork-io/terragrunt/terraform/cache/models"
	"github.com/gruntwork-io/terragrunt/terraform/cache/router"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
	"github.com/gruntwork-io/terragrunt/terraform/cliconfig"
	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/labstack/echo/v4"
)

type ProviderNetworkMirrorHandler struct {
	*CommonProviderHandler

	*http.Client
	providerService             *services.ProviderService
	cacheProviderHTTPStatusCode int
	networkMirrorURL            *url.URL
	credsSource                 *cliconfig.CredentialsSource
}

func NewProviderNetworkMirrorHandler(providerService *services.ProviderService, cacheProviderHTTPStatusCode int, networkMirror *cliconfig.ProviderInstallationNetworkMirror, credsSource *cliconfig.CredentialsSource) (ProviderHandler, error) {
	networkMirrorURL, err := url.Parse(networkMirror.URL)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return &ProviderNetworkMirrorHandler{
		CommonProviderHandler:       NewCommonProviderHandler(networkMirror.Include, networkMirror.Exclude),
		Client:                      &http.Client{},
		providerService:             providerService,
		cacheProviderHTTPStatusCode: cacheProviderHTTPStatusCode,
		networkMirrorURL:            networkMirrorURL,
		credsSource:                 credsSource,
	}, nil
}

func (handler *ProviderNetworkMirrorHandler) String() string {
	return "network mirror handler "
}

// GetVersions implements ProviderHandler.GetVersions
func (handler *ProviderNetworkMirrorHandler) GetVersions(ctx echo.Context, provider *models.Provider) error {
	var mirrorData struct {
		Versions map[string]struct{} `json:"versions"`
	}

	reqPath := path.Join(provider.RegistryName, provider.Namespace, provider.Name, "index.json")
	if err := handler.do(ctx, http.MethodGet, reqPath, &mirrorData); err != nil {
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
func (handler *ProviderNetworkMirrorHandler) GetPlatform(ctx echo.Context, provider *models.Provider, downloaderController router.Controller, cacheRequestID string) error {
	if cacheRequestID == "" {
		// it is impossible to return all platform properties from the network mirror, return 404 status
		return ctx.NoContent(http.StatusNotFound)
	}

	var mirrorData struct {
		Archives map[string]struct {
			URL    string   `json:"url"`
			Hashes []string `json:"hashes"`
		} `json:"archives"`
	}

	reqPath := path.Join(provider.RegistryName, provider.Namespace, provider.Name, provider.Version+".json")
	if err := handler.do(ctx, http.MethodGet, reqPath, &mirrorData); err != nil {
		return err
	}

	if archive, ok := mirrorData.Archives[provider.Platform()]; ok {
		provider.ResponseBody = (&models.ResponseBody{
			Filename:    filepath.Base(archive.URL),
			DownloadURL: archive.URL,
		}).ResolveRelativeReferences(handler.networkMirrorURL.ResolveReference(&url.URL{
			Path: path.Join(handler.networkMirrorURL.Path, provider.Address()),
		}))
	} else {
		return ctx.NoContent(http.StatusNotFound)
	}

	// start caching and return 423 status
	handler.providerService.CacheProvider(ctx.Request().Context(), cacheRequestID, provider)
	return ctx.NoContent(handler.cacheProviderHTTPStatusCode)
}

// Download implements ProviderHandler.Download
func (handler *ProviderNetworkMirrorHandler) Download(ctx echo.Context, provider *models.Provider) error {
	return ctx.NoContent(http.StatusNotImplemented)
}

func (handler *ProviderNetworkMirrorHandler) do(ctx echo.Context, method, reqPath string, value any) error {
	reqURL := fmt.Sprintf("%s/%s", strings.TrimRight(handler.networkMirrorURL.String(), "/"), reqPath)

	req, err := http.NewRequestWithContext(ctx.Request().Context(), method, reqURL, nil)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	if handler.credsSource != nil {
		hostname := svchost.Hostname(req.URL.Hostname())
		if creds := handler.credsSource.ForHost(hostname); creds != nil {
			creds.PrepareRequest(req)
		}
	}

	resp, err := handler.Client.Do(req)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	return helpers.DecodeJSONBody(resp, value)
}
