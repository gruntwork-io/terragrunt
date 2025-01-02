// Package cliconfig provides methods to create an OpenTofu/Terraform CLI configuration file.
package cliconfig

import (
	"os"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclwrite"
	svchost "github.com/hashicorp/terraform-svchost"
)

// ConfigHost is the structure of the "host" nested block within the CLI configuration, which can be used to override the default service host discovery behavior for a particular hostname.
type ConfigHost struct {
	Name     string            `hcl:",label"`
	Services map[string]string `hcl:"services,attr"`
}

// ConfigCredentials is the structure of the "credentials" nested block within the CLI configuration.
type ConfigCredentials struct {
	Name  string `hcl:",label"`
	Token string `hcl:"token"`
}

// ConfigCredentialsHelper is the structure of the "credentials_helper" nested block within the CLI configuration.
type ConfigCredentialsHelper struct {
	Name string   `hcl:",label"`
	Args []string `hcl:"args"`
}

// Config provides methods to create a terraform [CLI config file](https://developer.hashicorp.com/terraform/cli/config/config-file).
// The main purpose of which is to create a local config that will inherit the default user CLI config and adding new sections to force Terraform to send requests through the Terragrunt Cache server and use the provider cache directory.
type Config struct {
	DisableCheckpoint          bool `hcl:"disable_checkpoint"`
	DisableCheckpointSignature bool `hcl:"disable_checkpoint_signature"`

	Credentials        []ConfigCredentials      `hcl:"credentials,block"`
	CredentialsHelpers *ConfigCredentialsHelper `hcl:"credentials_helper,block"`

	PluginCacheDir       string                `hcl:"plugin_cache_dir"`
	Hosts                []ConfigHost          `hcl:"host,block"`
	ProviderInstallation *ProviderInstallation `hcl:"provider_installation,block"`
}

func (cfg *Config) Clone() *Config {
	var (
		providerInstallation *ProviderInstallation
		hosts                []ConfigHost
	)

	hosts = append(hosts, cfg.Hosts...)

	if cfg.ProviderInstallation != nil {
		providerInstallation = &ProviderInstallation{
			Methods: cfg.ProviderInstallation.Methods,
		}
	}

	return &Config{
		PluginCacheDir:             cfg.PluginCacheDir,
		DisableCheckpoint:          cfg.DisableCheckpoint,
		DisableCheckpointSignature: cfg.DisableCheckpointSignature,
		Credentials:                cfg.Credentials,
		CredentialsHelpers:         cfg.CredentialsHelpers,
		Hosts:                      hosts,
		ProviderInstallation:       providerInstallation,
	}
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

// AddProviderInstallationMethods merges new installation methods with the current ones, https://developer.hashicorp.com/terraform/cli/config/config-file#provider-installation
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

func (cfg *Config) AddProviderInstallationMethods(newMethods ...ProviderInstallationMethod) {
	if cfg.ProviderInstallation == nil {
		cfg.ProviderInstallation = &ProviderInstallation{}
	}

	cfg.ProviderInstallation.Methods = cfg.ProviderInstallation.Methods.Merge(newMethods...)
}

// Save marshalls and saves CLI config with the given config path.
func (cfg *Config) Save(configPath string) error {
	file := hclwrite.NewEmptyFile()
	gohcl.EncodeIntoBody(cfg, file.Body())

	const ownerWriteGlobalReadPerms = 0644
	if err := os.WriteFile(configPath, file.Bytes(), os.FileMode(ownerWriteGlobalReadPerms)); err != nil {
		return errors.New(err)
	}

	return nil
}

// CredentialsSource creates and returns a service credentials source whose behavior depends on which "credentials" if are present in the receiving config.
func (cfg *Config) CredentialsSource() *CredentialsSource {
	configured := make(map[svchost.Hostname]string)

	for _, creds := range cfg.Credentials {
		host, err := svchost.ForComparison(creds.Name)
		if err != nil {
			// We expect the config was already validated by the time we get here, so we'll just ignore invalid hostnames.
			continue
		}

		configured[host] = creds.Token
	}

	return &CredentialsSource{
		configured: configured,
	}
}
