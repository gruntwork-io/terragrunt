package config

import (
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

const defaultCatalogConfigFmt = `
include "root" {
  path = find_in_parent_folders("%s")
}
`

var rootConfigReg = regexp.MustCompile(fmt.Sprintf(`(?i)include[^\}]+%s[^\}]+}`, FuncNameFindInParentFolders))

type CatalogConfig struct {
	URLs []string `hcl:"urls,attr" cty:"urls"`
}

func (conf *CatalogConfig) String() string {
	return fmt.Sprintf("Catalog{URLs = %v}", conf.URLs)
}

func (config *CatalogConfig) normalize(cofnigPath string) {
	configDir := filepath.Dir(cofnigPath)

	// transform relative paths to absolute ones
	for i, url := range config.URLs {
		url := filepath.Join(configDir, url)

		if files.FileExists(url) {
			config.URLs[i] = url
		}
	}
}

// We want users to be able to browse to any folder in an `infra-live` repo, run `terragrunt catalog` (with no URL) arg.
// ReadCatalogConfig looks for the "nearest" `terragrunt.hcl` in the parent directories if the given `opts.TerragruntConfigPath` does not exist. Since our normal parsing `ParseConfigString` does not always work, as some `terragrunt.hcl` files are meant to be used from an `include` and/or they might use `find_in_parent_folders` and they only work from certain child folders, it parses this file to see if the config contains `include{...find_in_parent_folders()...}` block to determine if it is the root configuration. If it finds `terragrunt.hcl` that already has `include`, then read that configuration as is, oterwise generate a stub child `terragrunt.hcl` in memory with an `include` to pull in the one we found.
func ReadCatalogConfig(opts *options.TerragruntOptions) (*TerragruntConfig, error) {
	// We must first find the closest configuration from the current directory to ensure that it is not the root configuration,
	// otherwise when we try to pull it via the include block it gets an error "Only one level of includes is allowed".
	if !files.FileExists(opts.TerragruntConfigPath) {
		if configPath, err := FindInParentFolders([]string{opts.TerragruntConfigPath}, nil, opts); err != nil {
			return nil, err
		} else if configPath != "" {
			opts.TerragruntConfigPath = configPath
		}
	}

	configString, err := util.ReadFileAsString(opts.TerragruntConfigPath)
	if err != nil {
		return nil, err
	}

	// try to check if this is the root config, if yes then read the config as is.
	if rootConfigReg.MatchString(configString) {
		return ParseConfigString(configString, opts, nil, opts.TerragruntConfigPath, &EvalContextExtensions{})
	}

	configName := filepath.Base(opts.TerragruntConfigPath)
	configDir := filepath.Dir(opts.TerragruntConfigPath)

	// we have to imitate that the current directory is deeper in order for `find_in_parent_folders` can find the included configuration.
	opts.TerragruntConfigPath = filepath.Join(configDir, util.UniqueId(), configName)

	// try to load config via the include block
	config, err := ParseConfigString(fmt.Sprintf(defaultCatalogConfigFmt, configName), opts, nil, opts.TerragruntConfigPath, &EvalContextExtensions{})
	if err != nil {
		return nil, err
	}

	return config, nil

}
