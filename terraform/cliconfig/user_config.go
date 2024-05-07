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

	var methods []ProviderInstallationMethod

	for _, providerInstallation := range cfg.ProviderInstallation {
		for _, method := range providerInstallation.Methods {
			switch location := method.Location.(type) {
			case cliconfig.ProviderInstallationFilesystemMirror:
				methods = append(methods, NewProviderInstallationFilesystemMirror(string(location), method.Include, method.Exclude))
			case cliconfig.ProviderInstallationNetworkMirror:
				methods = append(methods, NewProviderInstallationNetworkMirror(string(location), method.Include, method.Exclude))
			default:
				methods = append(methods, NewProviderInstallationDirect(method.Include, method.Exclude))
			}
		}
	}

	return &Config{
		rawHCL:               rawHCL,
		PluginCacheDir:       cfg.PluginCacheDir,
		ProviderInstallation: &ProviderInstallation{Methods: methods},
	}, nil
}

func UserProviderDir() (string, error) {
	configDir, err := cliconfig.ConfigDir()
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return filepath.Join(configDir, "plugins"), nil
}
