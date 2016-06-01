package config

import (
	"io/ioutil"
	"github.com/hashicorp/hcl"
	"github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/errors"
)

const TERRAGRUNT_CONFIG_FILE = ".terragrunt"

// A common interface with all fields that could be in the .terragrunt config file.
type TerragruntConfig struct {
	DynamoDbLock *dynamodb.DynamoDbLock
	RemoteState  *remote.RemoteState
}

// Read the Terragrunt config file from its default location
func ReadTerragruntConfig() (*TerragruntConfig, error) {
	return parseTerragruntConfigFile(TERRAGRUNT_CONFIG_FILE)
}

// Parse the Terragrunt config file at the given path
func parseTerragruntConfigFile(configPath string) (*TerragruntConfig, error) {
	bytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error reading Terragrunt config file %s", configPath)
	}

	config, err := parseTerragruntConfig(string(bytes))
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error parsing Terragrunt config file %s", configPath)
	}

	return config, nil
}

// Parse the Terragrunt config contained in the given string
func parseTerragruntConfig(config string) (*TerragruntConfig, error) {
	terragruntConfig := &TerragruntConfig{}

	if err := hcl.Decode(terragruntConfig, config); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if terragruntConfig.DynamoDbLock != nil {
		terragruntConfig.DynamoDbLock.FillDefaults()
		if err := terragruntConfig.DynamoDbLock.Validate(); err != nil {
			return nil, err
		}
	}

	if terragruntConfig.RemoteState != nil {
		terragruntConfig.RemoteState.FillDefaults()
		if err := terragruntConfig.RemoteState.Validate(); err != nil {
			return nil, err
		}
	}

	return terragruntConfig, nil
}