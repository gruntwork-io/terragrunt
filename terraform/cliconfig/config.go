package cliconfig

import (
	"os"
	"regexp"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

var (
	// matches the line starting with `plugin_cache_dir =`
	configParamPluginCacheDirReg = regexp.MustCompile(`(?mi)^\s*plugin_cache_dir\s*=\s*(.*?)\s*$`)
)

// ConfigHost is the structure of the "host" nested block within the CLI configuration, which can be used to override the default service host discovery behavior for a particular hostname.
type ConfigHost struct {
	Name     string            `hcl:",label"`
	Services map[string]string `hcl:"services,attr"`
}

// Config provides methods to create a terraform [CLI config file](https://developer.hashicorp.com/terraform/cli/config/config-file).
// The main purpose of which is to create a local config that will inherit the default user CLI config and adding new sections to force Terraform to send requests through the Terragrunt Cache server and use the provider cache directory.
type Config struct {
	rawHCL []byte

	PluginCacheDir       string                `hcl:"plugin_cache_dir"`
	Hosts                []ConfigHost          `hcl:"host,block"`
	ProviderInstallation *ProviderInstallation `hcl:"provider_installation,block"`
}

// AddHost adds a host (officially undocumented), https://github.com/hashicorp/terraform/issues/28309
// It gives us opportunity rewrite path to the remote registry and the most important thing is that it works smoothly with HTTP (without HTTPS)
//
//	host "registry.terraform.io" {
//		services = {
//			"providers.v1" = "http://localhost:5758/v1/providers/registry.terraform.io/",
//		}
//	}
func (cfg *Config) AddHost(name string, services map[string]string) {
	cfg.Hosts = append(cfg.Hosts, ConfigHost{
		Name:     name,
		Services: services,
	})
}

// SetProviderInstallation sets an installation method, https://developer.hashicorp.com/terraform/cli/config/config-file#provider-installation
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
func (cfg *Config) SetProviderInstallation(filesystemMethod *ProviderInstallationFilesystemMirror, directMethod *ProviderInstallationDirect) {
	if filesystemMethod == nil && directMethod == nil {
		return
	}
	providerInstallation := &ProviderInstallation{
		FilesystemMirror: filesystemMethod,
		Direct:           directMethod,
	}
	cfg.ProviderInstallation = providerInstallation
}

// Save marshalls and saves CLI config with the given config path.
func (cfg *Config) Save(configPath string) error {
	rawHCL := cfg.rawHCL
	// Since `Config` structure already has `plugin_cache_dir`, remove it from the raw HCL config to prevent repeating in the saved file.
	rawHCL = configParamPluginCacheDirReg.ReplaceAll(rawHCL, []byte{})

	file := hclwrite.NewEmptyFile()

	gohcl.EncodeIntoBody(cfg, file.Body())
	newHCL := append(rawHCL, file.Bytes()...)

	if err := os.WriteFile(configPath, newHCL, os.FileMode(0644)); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}
