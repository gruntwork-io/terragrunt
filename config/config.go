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
	Lock        locks.Lock
	RemoteState *remote.RemoteState
}

// terragruntConfigFile represents the configuration supported in the .terragrunt file
type terragruntConfigFile struct {
	Parent      *ParentConfig       `hcl:"parent,omitempty"`
	Lock        *LockConfig         `hcl:"lock,omitempty"`
	RemoteState *remote.RemoteState `hcl:"remote_state,omitempty"`
}

// ParentConfig represents the configuration settings for a parent .terragrunt file that you can inherit settings from
type ParentConfig struct {
	Path string `hcl:"path"`
}

// LockConfig represents generic configuration for Lock providers
type LockConfig struct {
	Backend string            `hcl:"backend"`
	Config  map[string]string `hcl:"config"`
}

// ReadTerragruntConfig the Terragrunt config file from its default location
func ReadTerragruntConfig(terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
	util.Logger.Printf("Reading Terragrunt config file at %s", terragruntOptions.TerragruntConfigPath)
	return parseConfigFile(terragruntOptions.TerragruntConfigPath, terragruntOptions, nil)
}

// Parse the Terragrunt config file at the given path. If parent is specified, then treat this config is a parent
// config of some other file when resolving relative paths.
func parseConfigFile(configPath string, terragruntOptions *options.TerragruntOptions, parent *ParentConfig) (*TerragruntConfig, error) {
	configString, err := util.ReadFileAsString(configPath)
	if err != nil {
		return nil, err
	}

	config, err := parseConfigString(configString, terragruntOptions, parent)
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error parsing Terragrunt config file %s", configPath)
	}

	return config, nil
}

// Parse the Terragrunt config contained in the given string.
func parseConfigString(configString string, terragruntOptions *options.TerragruntOptions, parent *ParentConfig) (*TerragruntConfig, error) {
	// HCL does not natively process interpolations (${...}), and we don't want to write our own HCL parser, so for
	// now, we'll do the parsing in two passes. The first pass reads in the config file without processing any
	// interpolations. This is mostly to make the (un-interpolated) variables available programmatically,
	// especially the parent path, which we need to process other interpolations.
	terragruntConfigFromFileFirstPass := &terragruntConfigFile{}
	if err := hcl.Decode(terragruntConfigFromFileFirstPass, configString); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	// Now we process the interpolations in the string
	resolvedConfigString, err := ResolveTerragruntConfigString(configString, parent, terragruntOptions)
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

	if parent != nil && terragruntConfigFromFileFirstPass.Parent != nil {
		return nil, errors.WithStackTrace(TooManyLevelsOfInheritance(terragruntOptions.TerragruntConfigPath))
	}

	parentConfig, err := parseParentConfig(terragruntConfigFromFileFirstPass.Parent, terragruntOptions)
	if err != nil {
		return nil, err
	}

	return mergeConfigIntoParentConfig(config, parentConfig)
}

// Merge the given config into its parent config. Anything specified in the current config will override the contents
// of the parent config. If the parent config is nil, just returns the current config.
func mergeConfigIntoParentConfig(config *TerragruntConfig, parentConfig *TerragruntConfig) (*TerragruntConfig, error) {
	if parentConfig == nil {
		return config, nil
	}

	if config.Lock != nil {
		parentConfig.Lock = config.Lock
	}

	if config.RemoteState != nil {
		parentConfig.RemoteState = config.RemoteState
	}


	return parentConfig, nil
}

// Parse the config of the given parent, if one is specified
func parseParentConfig(parentConfig *ParentConfig, terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
	if parentConfig == nil {
		return nil, nil
	}

	parentPath := parentConfig.Path
	if parentPath == "" {
		return nil, errors.WithStackTrace(ParentConfigMissingPath(terragruntOptions.TerragruntConfigPath))
	}

	resolvedParentPath, err := ResolveTerragruntConfigString(parentPath, nil, terragruntOptions)
	if err != nil {
		return nil, err
	}

	if !filepath.IsAbs(resolvedParentPath) {
		resolvedParentPath = filepath.Join(filepath.Dir(terragruntOptions.TerragruntConfigPath), resolvedParentPath)
	}

	return parseConfigFile(resolvedParentPath, terragruntOptions, parentConfig)
}

// Convert the contents of a fully resolved .terragrunt file to a TerragruntConfig object
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

	return terragruntConfig, nil
}

// Custom error types

type ParentConfigMissingPath string
func (err ParentConfigMissingPath) Error() string {
	return fmt.Sprintf("The parent configuration in %s must specify a 'path' parameter", string(err))
}

type TooManyLevelsOfInheritance string
func (err TooManyLevelsOfInheritance) Error() string {
	return fmt.Sprintf("%s inherits from a %s file that inherits from yet another %s file. Only one level of parent inheritance is allowed.", string(err), DefaultTerragruntConfigPath, DefaultTerragruntConfigPath)
}