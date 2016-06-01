package config

import (
	"io/ioutil"
	"fmt"
	"github.com/hashicorp/hcl"
	"github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/gruntwork-io/terragrunt/remote"
)

const TERRAGRUNT_CONFIG_FILE = ".terragrunt"

// A common interface with all fields that could be in the .terragrunt config file.
type TerragruntConfig struct {
	DynamoDbLock *dynamodb.DynamoDbLock
	RemoteState  *remote.RemoteState
}

func ReadTerragruntConfig() (*TerragruntConfig, error) {
	return parseTerragruntConfigFile(TERRAGRUNT_CONFIG_FILE)
}

func parseTerragruntConfigFile(configPath string) (*TerragruntConfig, error) {
	bytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("Error reading Terragrunt config file %s (did you create one?): %s", configPath, err.Error())
	}

	config, err := parseTerragruntConfig(string(bytes))
	if err != nil {
		return nil, fmt.Errorf("Error parsing Terragrunt config file %s: %s", configPath, err.Error())
	}

	return config, nil
}

func parseTerragruntConfig(config string) (*TerragruntConfig, error) {
	terragruntConfig := &TerragruntConfig{}

	if err := hcl.Decode(terragruntConfig, config); err != nil {
		return nil, err
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