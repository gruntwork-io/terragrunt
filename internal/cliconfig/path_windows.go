//go:build windows
// +build windows

package cliconfig

import (
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	configFilename = "terragruntrc.json"
)

func ConfigDirs() ([]string, error) {
	dir, err := util.HomeDir()
	if err != nil {
		return nil, err
	}

	dirs := []string{dir}

	return dirs, nil
}
