package cliconfig

import (
	"path/filepath"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/hashicorp/terraform/command/cliconfig"
	"github.com/hashicorp/terraform/tfdiags"
)

// The user configuration is read as raw data and stored at the top of the saved configuration file.
// The location of the default config is different for each OS https://developer.hashicorp.com/terraform/cli/config/config-file#locations
func LoadUserConfig() (*Config, error) {
	return loadUserConfig(cliconfig.LoadConfig)
}

func loadUserConfig(
	loadConfigFn func() (*cliconfig.Config, tfdiags.Diagnostics),
) (*Config, error) {
	cfg, diag := loadConfigFn()
	if diag.HasErrors() {
		return nil, diag.Err()
	}

	var (
		methods            []ProviderInstallationMethod
		hosts              []ConfigHost
		credentials        []ConfigCredentials
		credentialsHelpers *ConfigCredentialsHelper
	)

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

	for name, host := range cfg.Hosts {
		services := make(map[string]string)
		if host != nil {
			for key, val := range host.Services {
				if val, ok := val.(string); ok {
					services[key] = val
				}
			}
		}

		host := ConfigHost{Name: name, Services: services}
		hosts = append(hosts, host)
	}

	for name, helper := range cfg.CredentialsHelpers {
		var args []string
		if helper != nil {
			args = helper.Args
		}

		credentialsHelpers = &ConfigCredentialsHelper{
			Name: name,
			Args: args,
		}
	}

	for name, credential := range cfg.Credentials {
		var token string

		if val, ok := credential["token"]; ok {
			if val, ok := val.(string); ok {
				token = val
			}
		}

		credential := ConfigCredentials{
			Name:  name,
			Token: token,
		}
		credentials = append(credentials, credential)
	}

	return &Config{
		DisableCheckpoint:          cfg.DisableCheckpoint,
		DisableCheckpointSignature: cfg.DisableCheckpointSignature,
		PluginCacheDir:             cfg.PluginCacheDir,
		Credentials:                credentials,
		CredentialsHelpers:         credentialsHelpers,
		ProviderInstallation:       &ProviderInstallation{Methods: methods},
		Hosts:                      hosts,
	}, nil
}

func UserProviderDir() (string, error) {
	configDir, err := cliconfig.ConfigDir()
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return filepath.Join(configDir, "plugins"), nil
}
