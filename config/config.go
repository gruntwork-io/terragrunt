package config

import (
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/locks"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/hashicorp/hcl"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"fmt"
	"path/filepath"
)

const DefaultTerragruntConfigPath = ".terragrunt"

// TerragruntConfig represents a parsed and expanded configuration
type TerragruntConfig struct {
	Terraform    *TerraformConfig
	Lock         locks.Lock
	RemoteState  *remote.RemoteState
	Dependencies *ModuleDependencies
}
func (conf *TerragruntConfig) String() string {
	return fmt.Sprintf("TerragruntConfig{Terraform = %v, Lock = %v, RemoteState = %v, Dependencies = %v}", conf.Terraform, conf.Lock, conf.RemoteState, conf.Dependencies)
}

// terragruntConfigFile represents the configuration supported in the terraform.tfvars file
type terragruntConfigFile struct {
	Terraform    *TerraformConfig    `hcl:"terraform,omitempty"`
	Include      *IncludeConfig      `hcl:"include,omitempty"`
	Lock         *LockConfig         `hcl:"lock,omitempty"`
	RemoteState  *remote.RemoteState `hcl:"remote_state,omitempty"`
	Dependencies *ModuleDependencies `hcl:"dependencies,omitempty"`
}

// IncludeConfig represents the configuration settings for a parent terraform.tfvars file that you can "include" in a
// child terraform.tfvars file
type IncludeConfig struct {
	Path string `hcl:"path"`
}

// LockConfig represents generic configuration for Lock providers
type LockConfig struct {
	Backend string            `hcl:"backend"`
	Config  map[string]string `hcl:"config"`
}

// ModuleDependencies represents the paths to other Terraform modules that must be applied before the current module
// can be applied
type ModuleDependencies struct {
	Paths []string `hcl:"paths"`
}
func (deps *ModuleDependencies) String() string {
	return fmt.Sprintf("ModuleDependencies{Paths = %v}", deps.Paths)
}

// TerraformConfig specifies where to find the Terraform configuration files
type TerraformConfig struct {
	Source string `hcl:"source"`
}
func (conf *TerraformConfig) String() string {
	return fmt.Sprintf("TerraformConfig{Source = %v}", conf.Source)
}

// Read the Terragrunt config file from its default location
func ReadTerragruntConfig(terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
	terragruntOptions.Logger.Printf("Reading Terragrunt config file at %s", terragruntOptions.TerragruntConfigPath)
	return ParseConfigFile(terragruntOptions.TerragruntConfigPath, terragruntOptions, nil)
}

// Parse the Terragrunt config file at the given path. If the include parameter is not nil, then treat this as a config
// included in some other config file when resolving relative paths.
func ParseConfigFile(configPath string, terragruntOptions *options.TerragruntOptions, include *IncludeConfig) (*TerragruntConfig, error) {
	configString, err := util.ReadFileAsString(configPath)

	if err != nil {
		return nil, err
	}

	config, err := parseConfigString(configString, terragruntOptions, include)
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error parsing Terragrunt config file %s", configPath)
	}

	return config, nil
}

// Parse the Terragrunt config contained in the given string.
func parseConfigString(configString string, terragruntOptions *options.TerragruntOptions, include *IncludeConfig) (*TerragruntConfig, error) {
	// HCL does not natively process interpolations (${...}), and we don't want to write our own HCL parser, so for
	// now, we'll do the parsing in two passes. The first pass reads in the config file without processing any
	// interpolations. This is mostly to make the (un-interpolated) variables available programmatically,
	// especially the parent path, which we need to process other interpolations.
	terragruntConfigFromFileFirstPass := &terragruntConfigFile{}
	if err := hcl.Decode(terragruntConfigFromFileFirstPass, configString); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	// Now we process the interpolations in the string
	resolvedConfigString, err := ResolveTerragruntConfigString(configString, include, terragruntOptions)
	if err != nil {
		return nil, err
	}

	// Now we do a second pass at parsing, but this time on the string with all the interpolations already
	// resolved
	terragruntConfigFromFileSecondPass := &terragruntConfigFile{}
	if err := hcl.Decode(terragruntConfigFromFileSecondPass, resolvedConfigString); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	config, err := convertToTerragruntConfig(terragruntConfigFromFileSecondPass, terragruntOptions)
	if err != nil {
		return nil, err
	}

	if include != nil && terragruntConfigFromFileFirstPass.Include != nil {
		return nil, errors.WithStackTrace(TooManyLevelsOfInheritance(terragruntOptions.TerragruntConfigPath))
	}

	includedConfig, err := parseIncludedConfig(terragruntConfigFromFileFirstPass.Include, terragruntOptions)
	if err != nil {
		return nil, err
	}

	return mergeConfigWithIncludedConfig(config, includedConfig)
}

// Merge the given config with an included config. Anything specified in the current config will override the contents
// of the included config. If the included config is nil, just return the current config.
func mergeConfigWithIncludedConfig(config *TerragruntConfig, includedConfig *TerragruntConfig) (*TerragruntConfig, error) {
	if includedConfig == nil {
		return config, nil
	}

	if config.Lock != nil {
		includedConfig.Lock = config.Lock
	}

	if config.RemoteState != nil {
		includedConfig.RemoteState = config.RemoteState
	}

	if config.Terraform != nil {
		includedConfig.Terraform = config.Terraform
	}

	if config.Dependencies != nil {
		includedConfig.Dependencies = config.Dependencies
	}

	return includedConfig, nil
}

// Parse the config of the given include, if one is specified
func parseIncludedConfig(includedConfig *IncludeConfig, terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
	if includedConfig == nil {
		return nil, nil
	}
	if includedConfig.Path == "" {
		return nil, errors.WithStackTrace(IncludedConfigMissingPath(terragruntOptions.TerragruntConfigPath))
	}

	resolvedIncludePath, err := ResolveTerragruntConfigString(includedConfig.Path, nil, terragruntOptions)
	if err != nil {
		return nil, err
	}

	if !filepath.IsAbs(resolvedIncludePath) {
		resolvedIncludePath = util.JoinPath(filepath.Dir(terragruntOptions.TerragruntConfigPath), resolvedIncludePath)
	}

	return ParseConfigFile(resolvedIncludePath, terragruntOptions, includedConfig)
}

// Convert the contents of a fully resolved terraform.tfvars file to a TerragruntConfig object
func convertToTerragruntConfig(terragruntConfigFromFile *terragruntConfigFile, terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
	terragruntConfig := &TerragruntConfig{}

	if terragruntConfigFromFile.Lock != nil {
		lock, err := lookupLock(terragruntConfigFromFile.Lock.Backend, terragruntConfigFromFile.Lock.Config)
		if err != nil {
			return nil, err
		}

		terragruntConfig.Lock = lock
	}

	if terragruntConfigFromFile.RemoteState != nil {
		terragruntConfigFromFile.RemoteState.FillDefaults()
		if err := terragruntConfigFromFile.RemoteState.Validate(); err != nil {
			return nil, err
		}

		terragruntConfig.RemoteState = terragruntConfigFromFile.RemoteState
	}

	terragruntConfig.Terraform = terragruntConfigFromFile.Terraform
	terragruntConfig.Dependencies = terragruntConfigFromFile.Dependencies

	return terragruntConfig, nil
}

// Custom error types

type IncludedConfigMissingPath string
func (err IncludedConfigMissingPath) Error() string {
	return fmt.Sprintf("The include configuration in %s must specify a 'path' parameter", string(err))
}

type TooManyLevelsOfInheritance string
func (err TooManyLevelsOfInheritance) Error() string {
	return fmt.Sprintf("%s includes %s, which itself includes %s. Only one level of includes is allowed.", string(err), DefaultTerragruntConfigPath, DefaultTerragruntConfigPath)
}