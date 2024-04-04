package cliconfig

import (
	"os"
	"regexp"

	"github.com/genelet/determined/dethcl"
	"github.com/gruntwork-io/boilerplate/errors"
	"github.com/hashicorp/terraform/command/cliconfig"
)

var (
	// matches the line starting with `plugin_cache_dir =`
	configParamPluginCacheDirReg = regexp.MustCompile(`(?mi)^\s*plugin_cache_dir\s*=\s*(.*?)\s*$`)
)

// Config provides methods to create a terraform [CLI config file](https://developer.hashicorp.com/terraform/cli/config/config-file).
// The main purpose of which is to create a local config that will inherit the default user CLI config and adding new sections to force Terraform to send requests through the Terragrunt Cache server and use the provider cache directory.
type Config struct {
	rawHCL []byte

	PluginCacheDir       string                           `hcl:"plugin_cache_dir"`
	Hosts                map[string]*cliconfig.ConfigHost `hcl:"host"`
	ProviderInstallation []*ProviderInstallation          `hcl:"provider_installation"`
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
	inheritHCL := cfg.rawHCL
	// Since `Config` structure already has `plugin_cache_dir`, remove it from the raw HCL config to prevent repetition in the saved file.
	inheritHCL = configParamPluginCacheDirReg.ReplaceAll(inheritHCL, []byte{})

	newHCL, err := dethcl.Marshal(cfg)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	newHCL = append(inheritHCL, newHCL...)

	if err := os.WriteFile(configPath, newHCL, os.FileMode(0644)); err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}
