//go:build !windows
// +build !windows

package cliconfig

import (
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/util"
)

const (
	configFilename = ".terragruntrc.json"
	configXDGDir   = "terragrunt"
)

func ConfigDirs() ([]string, error) {
	dir, err := util.HomeDir()
	if err != nil {
		return nil, err
	}

	dirs := []string{dir}

	if xdgDir := os.Getenv("XDG_CONFIG_HOME"); xdgDir != "" {
		dirs = append(dirs, filepath.Join(xdgDir, configXDGDir))
	}

	return dirs, nil
}
