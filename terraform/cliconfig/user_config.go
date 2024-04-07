package cliconfig

import (
	"os"
	"path/filepath"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/terraform/command/cliconfig"
	"github.com/hashicorp/terraform/tfdiags"
)

// The user configuration is read as raw data and stored at the top of the saved configuration file.
// The location of the default config is different for each OS https://developer.hashicorp.com/terraform/cli/config/config-file#locations
func LoadUserConfig() (*Config, error) {
	return loadUserConfig(cliconfig.ConfigFile, cliconfig.LoadConfig)
}

func loadUserConfig(
	configFileFn func() (string, error),
	loadConfigFn func() (*cliconfig.Config, tfdiags.Diagnostics),
) (*Config, error) {
	var rawHCL []byte

	configFile, err := configFileFn()
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if util.FileExists(configFile) {
		rawHCL, err = os.ReadFile(configFile)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
	}

	cfg, diag := loadConfigFn()
	if diag.HasErrors() {
		return nil, diag.Err()
	}

	return &Config{
		rawHCL:         rawHCL,
		PluginCacheDir: cfg.PluginCacheDir,
	}, nil
}

func UserProviderDir() (string, error) {
	configDir, err := cliconfig.ConfigDir()
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return filepath.Join(configDir, "plugins"), nil
}
