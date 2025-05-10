package cliconfig

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	gitDir = ".git"
)

func getRepoDirs(baseDir string) string {
	const maxPathWalking = 100

	for range maxPathWalking {
		gitDir := filepath.Join(baseDir, gitDir)
		if util.FileExists(gitDir) && util.IsDir(gitDir) {
			return baseDir
		}

		if parentDir := filepath.Dir(baseDir); parentDir != baseDir {
			baseDir = parentDir
		} else {
			break
		}
	}

	return ""
}

func DiscoveryPath(baseDir string) (string, error) {
	dirs := []string{
		baseDir,
	}

	if repoDir := getRepoDirs(baseDir); repoDir != "" {
		dirs = append(dirs, []string{
			repoDir,
			filepath.Join(repoDir, ".config"),
		}...)
	}

	configDirs, err := ConfigDirs()
	if err != nil {
		return "", errors.New(err)
	}

	dirs = append(dirs, configDirs...)

	for _, dir := range dirs {
		if !util.IsDir(dir) {
			continue
		}

		path := filepath.Join(dir, configFilename)

		if util.FileExists(path) {
			return path, nil
		}
	}

	return "", nil
}
