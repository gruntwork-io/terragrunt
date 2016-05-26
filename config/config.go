package config

import (
	"io/ioutil"
	"fmt"
	"strings"
	"github.com/hashicorp/hcl"
	"github.com/gruntwork-io/terragrunt/locks"
	"github.com/gruntwork-io/terragrunt/git"
	"github.com/gruntwork-io/terragrunt/dynamodb"
)

const TERRAGRUNT_CONFIG_FILE = ".terragrunt"
const DEFAULT_REMOTE_NAME = "origin"
const DEFAULT_TABLE_NAME = "terragrunt_locks"
const DEFAULT_BRANCH_NAME = "terragrunt_locks"
const DEFAULT_AWS_REGION = "us-east-1"

// A common interface with all fields that could be in the config file
type LockConfig struct {
	// Common fields
	LockType 	string
	StateFileId 	string

	// Embedded fields from all lock types
	git.GitLock
	dynamodb.DynamoLock
}

func (lockConfig *LockConfig) GetLockForConfig() (locks.Lock, error) {
	switch strings.ToLower(lockConfig.LockType) {
	case "git": return lockConfig.GitLock, nil
	case "dynamodb": return lockConfig.DynamoLock, nil
	default: return nil, fmt.Errorf("Unrecognized lock type: %s", lockConfig.LockType)
	}
}

func GetLockForConfig() (locks.Lock, error) {
	return getLockForConfig(TERRAGRUNT_CONFIG_FILE)
}

func getLockForConfig(configPath string) (locks.Lock, error) {
	bytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("Error reading Terragrunt config file %s (did you create one?): %s", configPath, err.Error())
	}

	config := &LockConfig{}
	if err := hcl.Decode(config, string(bytes)); err != nil {
		return nil, fmt.Errorf("Error parsing Terragrunt config file %s: %s", configPath, err.Error())
	}

	fillDefaults(config)
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("Error validating Terragrunt config file %s: %s", configPath, err.Error())
	}

	return config.GetLockForConfig()
}

func fillDefaults(config *LockConfig) {
	config.GitLock.StateFileId = config.StateFileId
	config.DynamoLock.StateFileId = config.StateFileId

	if config.RemoteName == "" {
		config.RemoteName = DEFAULT_REMOTE_NAME
	}

	if config.TableName == "" {
		config.TableName = DEFAULT_TABLE_NAME
	}

	if config.LockBranch == "" {
		config.LockBranch = DEFAULT_BRANCH_NAME
	}

	if config.AwsRegion == "" {
		config.AwsRegion = DEFAULT_AWS_REGION
	}
}

func validateConfig(config *LockConfig) error {
	if config.StateFileId == "" {
		return fmt.Errorf("The stateFileId field cannot be empty")
	}

	return nil
}

