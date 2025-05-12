package cliconfig

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	gitDir = ".git"
)

// DiscoveryPath tries to find the configuration file in the following directories:
// 1. In the specified `baseDir` (TG working directory).
// 2. In the repository root directory.
// 3. In `.config` located in the repository root directory.
// 4. In config dirs, depending on the OS, this may be `HOME` or `XDG_CONFIG_HOME` directory.
func DiscoveryPath(baseDir string) (string, error) {
	dirs := []string{
		baseDir,
	}

	if repoDir := getRepoDir(baseDir); repoDir != "" {
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

		path := filepath.Join(dir, DefaultConfigFilename)

		if util.FileExists(path) {
			return path, nil
		}
	}

	return "", nil
}

func getRepoDir(baseDir string) string {
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
