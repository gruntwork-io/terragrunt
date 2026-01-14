package cliconfig

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/hashicorp/terraform/command/cliconfig"
	"github.com/hashicorp/terraform/tfdiags"
)

// LoadUserConfig loads the user configuration is read as raw data and stored at the top of the saved configuration file.
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
		installationMethods = getUserProviderInstallationMethods(cfg)
		hosts               = getUserHosts(cfg)
		credentialsHelpers  = getUserCredentialsHelpers(cfg)
		credentials         = getUserCredentials(cfg)
	)

	return &Config{
		DisableCheckpoint:          cfg.DisableCheckpoint,
		DisableCheckpointSignature: cfg.DisableCheckpointSignature,
		PluginCacheDir:             cfg.PluginCacheDir,
		Credentials:                credentials,
		CredentialsHelpers:         credentialsHelpers,
		ProviderInstallation:       &ProviderInstallation{Methods: installationMethods},
		Hosts:                      hosts,
	}, nil
}

func UserProviderDir() (string, error) {
	configDir, err := cliconfig.ConfigDir()
	if err != nil {
		return "", errors.New(err)
	}

	return filepath.Join(configDir, "plugins"), nil
}

func getUserCredentials(cfg *cliconfig.Config) []ConfigCredentials {
	var credentials = make([]ConfigCredentials, 0, len(cfg.Credentials))

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

	return credentials
}

func getUserCredentialsHelpers(cfg *cliconfig.Config) *ConfigCredentialsHelper {
	var credentialsHelpers *ConfigCredentialsHelper

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

	return credentialsHelpers
}

func getUserHosts(cfg *cliconfig.Config) []ConfigHost {
	var hosts = make([]ConfigHost, 0, len(cfg.Hosts))

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

	return hosts
}

func getUserProviderInstallationMethods(cfg *cliconfig.Config) []ProviderInstallationMethod {
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

	return methods
}
