package terraform

import (
	"os"
	"regexp"

	"github.com/genelet/determined/dethcl"
	"github.com/gruntwork-io/boilerplate/errors"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/terraform/command/cliconfig"
)

var (
	configParamNamePluginCacheDirReg = regexp.MustCompile(`(?mi)^.*plugin_cache_dir.*$`)
)

// ProviderInstallationMethod represents an installation method block inside a provider_installation block.
type ProviderInstallationMethod struct {
	Include []string `hcl:"include"`
	Exclude []string `hcl:"exclude"`
}

type ProviderInstallationFilesystemMirror struct {
	Location string `hcl:"path"`
	*ProviderInstallationMethod
}

func NewProviderInstallationFilesystemMirror(location string, include, exclude []string) *ProviderInstallationFilesystemMirror {
	return &ProviderInstallationFilesystemMirror{
		Location: location,
		ProviderInstallationMethod: &ProviderInstallationMethod{
			Include: include,
			Exclude: exclude,
		},
	}
}

type ProviderInstallationDirect struct {
	*ProviderInstallationMethod
}

func NewProviderInstallationDirect(include, exclude []string) *ProviderInstallationDirect {
	return &ProviderInstallationDirect{
		ProviderInstallationMethod: &ProviderInstallationMethod{
			Include: include,
			Exclude: exclude,
		},
	}
}

// ProviderInstallation is the structure of the "provider_installation" nested block within the CLI configuration.
type ProviderInstallation struct {
	FilesystemMirror *ProviderInstallationFilesystemMirror `hcl:"filesystem_mirror"`
	Direct           *ProviderInstallationDirect           `hcl:"direct"`
}

type Config struct {
	*cliconfig.Config `hcl:"-"`

	Hosts                map[string]*cliconfig.ConfigHost `hcl:"host"`
	ProviderInstallation []*ProviderInstallation          `hcl:"provider_installation"`
}

// LoadConfig return a new Config instance and loads the default/user terraform CLI config in order to retrieve `PluginCacheDir` value.
// The location of the default config is different for each OS https://developer.hashicorp.com/terraform/cli/config/config-file#locations
func LoadConfig() (*Config, error) {
	defaultCfg, diag := cliconfig.LoadConfig()
	if diag.HasErrors() {
		return nil, errors.WithStackTrace(diag.Err())
	}

	return &Config{
		Config: defaultCfg,
		Hosts:  make(map[string]*cliconfig.ConfigHost),
	}, nil
}

// AddHost adds a host (officially undocumented), https://github.com/hashicorp/terraform/issues/28309
// It gives us opportunity rewrite path to the remote registry and the most important thing is that it works smoothly with HTTP (without HTTPS)
//
//	host "registry.terraform.io" {
//		services = {
//			"providers.v1" = "http://localhost:5758/v1/providers/registry.terraform.io/",
//		}
//	}
func (cfg *Config) AddHost(name string, services map[string]any) {
	cfg.Hosts[name] = &cliconfig.ConfigHost{
		Services: services,
	}
}

// AddProviderInstallation adds an installation method, https://developer.hashicorp.com/terraform/cli/config/config-file#provider-installation
//
//	provider_installation {
//		filesystem_mirror {
//			path    = "/path/to/the/provider/cache"
//			include = ["example.com/*/*"]
//		}
//		direct {
//			exclude = ["example.com/*/*"]
//		}
//	}
func (cfg *Config) AddProviderInstallation(filesystemMethod *ProviderInstallationFilesystemMirror, directMethod *ProviderInstallationDirect) {
	providerInstallation := &ProviderInstallation{
		FilesystemMirror: filesystemMethod,
		Direct:           directMethod,
	}

	cfg.ProviderInstallation = append(cfg.ProviderInstallation, providerInstallation)
}

// Save marshalls and saves CLI config with the given config path.
// In order to not lose user/default settings, if the user/default CLI config file exists, read this config and place at the top our the config file.
func (cfg *Config) Save(configPath string) error {
	hclBytes, err := dethcl.Marshal(cfg)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	defaultCLIConfigFile, err := cliconfig.ConfigFile()
	if err != nil {
		return errors.WithStackTrace(err)
	}

	if util.FileExists(defaultCLIConfigFile) {
		defaultHCLBytes, err := os.ReadFile(defaultCLIConfigFile)
		if err != nil {
			return errors.WithStackTrace(err)
		}

		// if `PluginCacheDir` is empty, ensure that it will not be taken from the user/default CLI configuration.
		if cfg.PluginCacheDir == "" {
			defaultHCLBytes = configParamNamePluginCacheDirReg.ReplaceAll(defaultHCLBytes, []byte{})
		}

		hclBytes = append(defaultHCLBytes, hclBytes...)
	}

	if err := os.WriteFile(configPath, hclBytes, os.FileMode(0644)); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}
