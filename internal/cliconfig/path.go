package cliconfig

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

func DiscoveryPath(paths ...string) (string, error) {
	for _, path := range paths {
		path, err := filepath.Abs(path)
		if err != nil {
			return "", errors.New(err)
		}

		if !util.FileExists(path) {
			return "", NewNotFoundError(path)
		}

		if util.IsDir(path) {
			path = filepath.Join(path, configFilename)
		}

		if util.FileExists(path) {
			return path, nil
		}
	}

	return "", nil
}
