package config

import (
	"io/ioutil"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/locks"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/hashicorp/hcl"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

const DefaultTerragruntConfigPath = ".terragrunt"

// TerragruntConfig represents a parsed and expanded configuration
type TerragruntConfig struct {
	Lock        locks.Lock
	RemoteState *remote.RemoteState
}

// terragruntConfigFile represents the configuration supported in the .terragrunt file
type terragruntConfigFile struct {
	Lock        *LockConfig         `hcl:"lock,omitempty"`
	RemoteState *remote.RemoteState `hcl:"remote_state"`
}

// LockConfig represents generic configuration for Lock providers
type LockConfig struct {
	Backend string            `hcl:"backend"`
	Config  map[string]string `hcl:"config"`
}

// ReadTerragruntConfig the Terragrunt config file from its default location
func ReadTerragruntConfig(terragruntOptions *options.TerragruntOptions) (*TerragruntConfig, error) {
	util.Logger.Printf("Reading Terragrunt config file at %s", terragruntOptions.TerragruntConfigPath)
	return parseConfigFile(terragruntOptions.TerragruntConfigPath)
}

// Parse the Terragrunt config file at the given path
func parseConfigFile(configPath string) (*TerragruntConfig, error) {
	bytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error reading Terragrunt config file %s", configPath)
	}

	config, err := parseConfigString(string(bytes))
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error parsing Terragrunt config file %s", configPath)
	}

	return config, nil
}

// Parse the Terragrunt config contained in the given string
func parseConfigString(configSrc string) (*TerragruntConfig, error) {
	terragruntConfigFromFile := &terragruntConfigFile{}
	if err := hcl.Decode(terragruntConfigFromFile, configSrc); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	c := &TerragruntConfig{}

	if lockConfig := terragruntConfigFromFile.Lock; lockConfig != nil {
		lock, err := lookupLock(lockConfig.Backend, lockConfig.Config)
		if err != nil {
			return nil, err
		}

		c.Lock = lock
	}

	if terragruntConfigFromFile.RemoteState != nil {
		terragruntConfigFromFile.RemoteState.FillDefaults()
		if err := terragruntConfigFromFile.RemoteState.Validate(); err != nil {
			return nil, err
		}

		c.RemoteState = terragruntConfigFromFile.RemoteState
	}

	return c, nil
}
