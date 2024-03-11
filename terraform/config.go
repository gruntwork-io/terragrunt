package terraform

import (
	"os"
	"regexp"

	"github.com/genelet/determined/dethcl"
	"github.com/gruntwork-io/boilerplate/errors"
	"github.com/hashicorp/terraform/command/cliconfig"
)

var (
	configParamNamePluginCacheDirReg = regexp.MustCompile(`(?mi)^.*plugin_cache_dir.*$`)
)

type ConfigHost struct {
	Services map[string]interface{} `hcl:"services"`
}

func NewConfigHost(services map[string]any) *ConfigHost {
	return &ConfigHost{
		Services: services,
	}
}

// ProviderInstallationMethod represents an installation method block inside
// a provider_installation block.
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

// ProviderInstallation is the structure of the "provider_installation"
// nested block within the CLI configuration.
type ProviderInstallation struct {
	FilesystemMirror *ProviderInstallationFilesystemMirror `hcl:"filesystem_mirror"`
	Direct           *ProviderInstallationDirect           `hcl:"direct"`
}

type Config struct {
	*cliconfig.Config `hcl:"-"`

	Hosts                map[string]*ConfigHost  `hcl:"host"`
	ProviderInstallation []*ProviderInstallation `hcl:"provider_installation"`
}

func LoadConfig() (*Config, error) {
	globalCfg, diag := cliconfig.LoadConfig()
	if diag.HasErrors() {
		return nil, errors.WithStackTrace(diag.Err())
	}

	return &Config{
		Config: globalCfg,
		Hosts:  make(map[string]*ConfigHost),
	}, nil
}

func (cfg *Config) AddHost(name string, host *ConfigHost) {
	cfg.Hosts[name] = host
}

func (cfg *Config) AddProviderInstallation(filesystemMethod *ProviderInstallationFilesystemMirror, directMethod *ProviderInstallationDirect) {
	providerInstallation := &ProviderInstallation{
		FilesystemMirror: filesystemMethod,
		Direct:           directMethod,
	}

	cfg.ProviderInstallation = append(cfg.ProviderInstallation, providerInstallation)
}

func (cfg *Config) SaveConfig(cliConfigFile string) error {
	globalCLIConfigFile, err := cliconfig.ConfigFile()
	if err != nil {
		return errors.WithStackTrace(err)
	}

	globalHCLBytes, err := os.ReadFile(globalCLIConfigFile)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	if cfg.PluginCacheDir == "" {
		globalHCLBytes = configParamNamePluginCacheDirReg.ReplaceAll(globalHCLBytes, []byte{})
	}

	localHCLBytes, err := dethcl.Marshal(cfg)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	hclBytes := append(globalHCLBytes, localHCLBytes...)

	if err := os.WriteFile(cliConfigFile, hclBytes, os.FileMode(0644)); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}
