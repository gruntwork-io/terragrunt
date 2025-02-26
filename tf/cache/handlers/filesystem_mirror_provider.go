package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/util"
)

var _ ProviderHandler = new(FilesystemMirrorProviderHandler)

type FilesystemMirrorProviderHandler struct {
	*CommonProviderHandler

	filesystemMirrorPath string
}

func NewFilesystemMirrorProviderHandler(logger log.Logger, method *cliconfig.ProviderInstallationFilesystemMirror) *FilesystemMirrorProviderHandler {
	return &FilesystemMirrorProviderHandler{
		CommonProviderHandler: NewCommonProviderHandler(logger, method.Include, method.Exclude),
		filesystemMirrorPath:  method.Path,
	}
}

func (handler *FilesystemMirrorProviderHandler) String() string {
	return "filesystem_mirror '" + handler.filesystemMirrorPath + "'"
}

// GetVersions implements ProviderHandler.GetVersions
func (handler *FilesystemMirrorProviderHandler) GetVersions(_ context.Context, provider *models.Provider) (models.Versions, error) {
	var mirrorData struct {
		Versions map[string]struct{} `json:"versions"`
	}

	filename := filepath.Join(provider.RegistryName, provider.Namespace, provider.Name, "index.json")
	if err := handler.readMirrorData(filename, &mirrorData); err != nil {
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
func (handler *FilesystemMirrorProviderHandler) GetPlatform(_ context.Context, provider *models.Provider) (*models.ResponseBody, error) {
	var mirrorData struct {
		Archives map[string]struct {
			URL    string   `json:"url"`
			Hashes []string `json:"hashes"`
		} `json:"archives"`
	}

	filename := filepath.Join(provider.RegistryName, provider.Namespace, provider.Name, provider.Version+".json")
	if err := handler.readMirrorData(filename, &mirrorData); err != nil {
		return nil, err
	}

	var resp *models.ResponseBody

	if archive, ok := mirrorData.Archives[provider.Platform()]; ok {
		// check if the URL contains http scheme, it may just be a filename and we need to build the URL
		if !strings.Contains(archive.URL, "://") {
			archive.URL = filepath.Join(handler.filesystemMirrorPath, provider.RegistryName, provider.Namespace, provider.Name, archive.URL)
		}

		resp = &models.ResponseBody{
			Filename:    filepath.Base(archive.URL),
			DownloadURL: archive.URL,
		}
	}

	return resp, nil
}

func (handler *FilesystemMirrorProviderHandler) readMirrorData(filename string, value any) error {
	filename = filepath.Join(handler.filesystemMirrorPath, filename)

	if !util.FileExists(filename) {
		return nil
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return errors.New(err)
	}

	if err := json.Unmarshal(data, value); err != nil {
		return errors.New(err)
	}

	return nil
}
