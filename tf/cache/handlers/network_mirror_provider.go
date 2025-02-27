package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/tf/cliconfig"
)

var _ ProviderHandler = new(NetworkMirrorProviderHandler)

type NetworkMirrorProviderHandler struct {
	*CommonProviderHandler

	client           *helpers.Client
	networkMirrorURL *url.URL
}

func NewNetworkMirrorProviderHandler(logger log.Logger, networkMirror *cliconfig.ProviderInstallationNetworkMirror, credsSource *cliconfig.CredentialsSource) (*NetworkMirrorProviderHandler, error) {
	networkMirrorURL, err := url.Parse(networkMirror.URL)
	if err != nil {
		return nil, errors.Errorf("failed to parse network mirror URL %q: %w", networkMirror.URL, err)
	}

	return &NetworkMirrorProviderHandler{
		CommonProviderHandler: NewCommonProviderHandler(logger, networkMirror.Include, networkMirror.Exclude),
		client:                helpers.NewClient(credsSource),
		networkMirrorURL:      networkMirrorURL,
	}, nil
}

func (handler *NetworkMirrorProviderHandler) String() string {
	return "network_mirror '" + handler.networkMirrorURL.String() + "'"
}

// GetVersions implements ProviderHandler.GetVersions
func (handler *NetworkMirrorProviderHandler) GetVersions(ctx context.Context, provider *models.Provider) (models.Versions, error) {
	var mirrorData struct {
		Versions map[string]struct{} `json:"versions"`
	}

	reqPath := path.Join(provider.RegistryName, provider.Namespace, provider.Name, "index.json")
	reqURL := fmt.Sprintf("%s/%s", strings.TrimRight(handler.networkMirrorURL.String(), "/"), reqPath)

	if err := handler.client.Do(ctx, http.MethodGet, reqURL, &mirrorData); err != nil {
		return nil, err
	}

	var versions = make(models.Versions, 0, len(mirrorData.Versions))

	for version := range mirrorData.Versions {
		versions = append(versions, &models.Version{
			Version:   version,
			Platforms: availablePlatforms,
		})
	}

	return versions, nil
}

// GetPlatform implements ProviderHandler.GetPlatform
func (handler *NetworkMirrorProviderHandler) GetPlatform(ctx context.Context, provider *models.Provider) (*models.ResponseBody, error) {
	var mirrorData struct {
		Archives map[string]struct {
			URL    string   `json:"url"`
			Hashes []string `json:"hashes"`
		} `json:"archives"`
	}

	reqPath := path.Join(provider.RegistryName, provider.Namespace, provider.Name, provider.Version+".json")
	reqURL := fmt.Sprintf("%s/%s", strings.TrimRight(handler.networkMirrorURL.String(), "/"), reqPath)

	if err := handler.client.Do(ctx, http.MethodGet, reqURL, &mirrorData); err != nil {
		return nil, err
	}

	var resp *models.ResponseBody

	if archive, ok := mirrorData.Archives[provider.Platform()]; ok {
		resp = (&models.ResponseBody{
			Filename:    filepath.Base(archive.URL),
			DownloadURL: archive.URL,
		}).ResolveRelativeReferences(handler.networkMirrorURL.ResolveReference(&url.URL{
			Path: path.Join(handler.networkMirrorURL.Path, provider.Address()),
		}))
	}

	return resp, nil
}
