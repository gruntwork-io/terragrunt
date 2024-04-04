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
	// matches the line starting with `plugin_cache_dir =`
	configParamPluginCacheDirReg = regexp.MustCompile(`(?mi)^\s*plugin_cache_dir\s*=\s*(.*?)\s*$`)
)

type Configer interface {
	// In order to not lose the user default config, the loaded user config will be added at the top of the saved config file.
	// The location of the default config is different for each OS https://developer.hashicorp.com/terraform/cli/config/config-file#locations
	LoadConfig() (*Config, error)
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

// ProviderInstallationMethod represents an installation method block inside a provider_installation block.
type ProviderInstallationMethod struct {
	Include []string `hcl:"include"`
	Exclude []string `hcl:"exclude"`
}

type ProviderInstallationFilesystemMirror struct {
	Location string `hcl:"path"`
	*ProviderInstallationMethod
}

// Config provides methods to create a terraform [CLI config file](https://developer.hashicorp.com/terraform/cli/config/config-file).
// The main purpose of which is to create a local config that will inherit the default user CLI config and adding new settings that will force Terraform to make requests through a Terragrunt Cache server and use the provider cache directory.
type Config struct {
	rawHCL []byte

	PluginCacheDir       string                           `hcl:"plugin_cache_dir"`
	Hosts                map[string]*cliconfig.ConfigHost `hcl:"host"`
	ProviderInstallation []*ProviderInstallation          `hcl:"provider_installation"`
}

// In order to not lose the user default config, the loaded user config will be added at the top of the saved config file.
// The location of the default config is different for each OS https://developer.hashicorp.com/terraform/cli/config/config-file#locations
type userConfiger struct{}

// In order to not lose the user default config, the loaded user config will be added at the top of the saved config file.
// The location of the default config is different for each OS https://developer.hashicorp.com/terraform/cli/config/config-file#locations
func (*userConfiger) LoadConfig() (*Config, error) {
	var rawHCL []byte

	configFile, err := cliconfig.ConfigFile()
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if util.FileExists(configFile) {
		rawHCL, err = os.ReadFile(configFile)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		// Since there is a `PluginCacheDir` in our Config structure, remove it from the raw HCL config to prevent repetition in the saved configuration file.
		rawHCL = configParamPluginCacheDirReg.ReplaceAll(rawHCL, []byte{})
	}

	cfg, diag := cliconfig.LoadConfig()
	if diag.HasErrors() {
		return nil, diag.Err()
	}

	return &Config{
		rawHCL:         rawHCL,
		PluginCacheDir: cfg.PluginCacheDir,
		Hosts:          make(map[string]*cliconfig.ConfigHost),
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
func (cfg *Config) Save(configPath string) error {
	newHCL := cfg.rawHCL

	// if `PluginCacheDir` is empty, ensure that it will not be taken from the user/default CLI configuration.
	if cfg.PluginCacheDir == "" {
		newHCL = configParamPluginCacheDirReg.ReplaceAll(newHCL, []byte{})
	}

	currentHCL, err := dethcl.Marshal(cfg)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	newHCL = append(newHCL, currentHCL...)

	if err := os.WriteFile(configPath, newHCL, os.FileMode(0644)); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}
