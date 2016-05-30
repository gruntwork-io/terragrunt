package config

import (
	"io/ioutil"
	"fmt"
	"strings"
	"github.com/hashicorp/hcl"
	"github.com/gruntwork-io/terragrunt/locks"
	"github.com/gruntwork-io/terragrunt/dynamodb"
)

const TERRAGRUNT_CONFIG_FILE = ".terragrunt"
const DEFAULT_TABLE_NAME = "terragrunt_locks"
const DEFAULT_AWS_REGION = "us-east-1"
const DEFAULT_MAX_RETRIES_WAITING_FOR_LOCK = 360

// A common interface with all fields that could be in the config file. We keep this generic to be able to support
// different lock types in the future.
type LockConfig struct {
	// Common fields
	LockType 	string
	StateFileId 	string

	// Embedded fields from any supported lock types
	dynamodb.DynamoLock
}

// This method returns the lock impelemntation specified in the config file. Currently, only DynamoDB is supported, but
// we may add other lock mechanisms in the future.
func (lockConfig *LockConfig) GetLockForConfig() (locks.Lock, error) {
	switch strings.ToLower(lockConfig.LockType) {
	case "dynamodb": return lockConfig.DynamoLock, nil
	default: return nil, fmt.Errorf("Unrecognized lock type: %s", lockConfig.LockType)
	}
}

func GetLockForConfig() (locks.Lock, error) {
	return getLockForConfigFile(TERRAGRUNT_CONFIG_FILE)
}

func getLockForConfigFile(configPath string) (locks.Lock, error) {
	bytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("Error reading Terragrunt config file %s (did you create one?): %s", configPath, err.Error())
	}

	lock, err := getLockForConfig(string(bytes))
	if err != nil {
		return nil, fmt.Errorf("Error parsing Terragrunt config file %s: %s", configPath, err.Error())
	}
	return lock, nil
}

func getLockForConfig(config string) (locks.Lock, error) {
	lockConfig := &LockConfig{}
	if err := hcl.Decode(lockConfig, config); err != nil {
		return nil, err
	}

	fillDefaults(lockConfig)
	if err := validateConfig(lockConfig); err != nil {
		return nil, err
	}

	return lockConfig.GetLockForConfig()
}

func fillDefaults(config *LockConfig) {
	config.DynamoLock.StateFileId = config.StateFileId

	if config.TableName == "" {
		config.TableName = DEFAULT_TABLE_NAME
	}

	if config.AwsRegion == "" {
		config.AwsRegion = DEFAULT_AWS_REGION
	}

	if config.MaxLockRetries == 0 {
		config.MaxLockRetries = DEFAULT_MAX_RETRIES_WAITING_FOR_LOCK
	}
}

func validateConfig(config *LockConfig) error {
	if config.LockType == "" {
		return fmt.Errorf("The lockType field cannot be empty")
	}

	if config.StateFileId == "" {
		return fmt.Errorf("The stateFileId field cannot be empty")
	}

	return nil
}

