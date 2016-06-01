package config

import (
	"testing"
	"github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/stretchr/testify/assert"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/errors"
	"reflect"
)

func TestParseTerragruntConfigDynamoLockMinimalConfig(t *testing.T) {
	t.Parallel()

	config :=
	`
	dynamoDbLock = {
	  stateFileId = "expected-state-file-id"
	}
	`

	terragruntConfig, err := parseTerragruntConfig(config)
	assert.Nil(t, err)

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.NotNil(t, terragruntConfig.DynamoDbLock)
	assert.Equal(t, "expected-state-file-id", terragruntConfig.DynamoDbLock.StateFileId)
	assert.Equal(t, dynamodb.DEFAULT_AWS_REGION, terragruntConfig.DynamoDbLock.AwsRegion)
	assert.Equal(t, dynamodb.DEFAULT_TABLE_NAME, terragruntConfig.DynamoDbLock.TableName)
	assert.Equal(t, dynamodb.DEFAULT_MAX_RETRIES_WAITING_FOR_LOCK, terragruntConfig.DynamoDbLock.MaxLockRetries)
}

func TestParseTerragruntConfigDynamoLockFullConfig(t *testing.T) {
	t.Parallel()

	config :=
	`
	dynamoDbLock = {
	  stateFileId = "expected-state-file-id"
	  awsRegion = "expected-region"
	  tableName = "expected-table-name"
	  maxLockRetries = 100
	}
	`

	terragruntConfig, err := parseTerragruntConfig(config)
	assert.Nil(t, err)

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.NotNil(t, terragruntConfig.DynamoDbLock)
	assert.Equal(t, "expected-state-file-id", terragruntConfig.DynamoDbLock.StateFileId)
	assert.Equal(t, "expected-region", terragruntConfig.DynamoDbLock.AwsRegion)
	assert.Equal(t, "expected-table-name", terragruntConfig.DynamoDbLock.TableName)
	assert.Equal(t, 100, terragruntConfig.DynamoDbLock.MaxLockRetries)
}

func TestParseTerragruntConfigDynamoLockMissingStateFileId(t *testing.T) {
	t.Parallel()

	config :=
	`
	dynamoDbLock = {
	}
	`

	_, err := parseTerragruntConfig(config)
	assert.True(t, errors.IsError(err, dynamodb.StateFileIdMissing), "Unexpected error of type %s: %s", reflect.TypeOf(err), err)
}

func TestParseTerragruntConfigRemoteStateMinimalConfig(t *testing.T) {
	t.Parallel()

	config :=
	`
	remoteState = {
	  backend = "s3"
	}
	`

	terragruntConfig, err := parseTerragruntConfig(config)
	assert.Nil(t, err)

	assert.Nil(t, terragruntConfig.DynamoDbLock)
	assert.NotNil(t, terragruntConfig.RemoteState)
	assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
	assert.Empty(t, terragruntConfig.RemoteState.BackendConfigs)
}

func TestParseTerragruntConfigRemoteStateMissingBackend(t *testing.T) {
	t.Parallel()

	config :=
	`
	remoteState = {
	}
	`

	_, err := parseTerragruntConfig(config)
	assert.True(t, errors.IsError(err, remote.RemoteBackendMissing), "Unexpected error of type %s: %s", reflect.TypeOf(err), err)
}


func TestParseTerragruntConfigRemoteStateFullConfig(t *testing.T) {
	t.Parallel()

	config :=
	`
	remoteState = {
	  backend = "s3"
	  backendConfigs = {
	    encrypted = "true"
	    bucket = "my-bucket"
	    key = "terraform.tfstate"
	    region = "us-east-1"
	  }
	}
	`

	terragruntConfig, err := parseTerragruntConfig(config)
	assert.Nil(t, err)

	assert.Nil(t, terragruntConfig.DynamoDbLock)
	assert.NotNil(t, terragruntConfig.RemoteState)
	assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
	assert.NotEmpty(t, terragruntConfig.RemoteState.BackendConfigs)
	assert.Equal(t, "true", terragruntConfig.RemoteState.BackendConfigs["encrypted"])
	assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.BackendConfigs["bucket"])
	assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.BackendConfigs["key"])
	assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.BackendConfigs["region"])
}

func TestParseTerragruntConfigRemoteStateAndDynamoDbFullConfig(t *testing.T) {
	t.Parallel()

	config :=
	`
	dynamoDbLock = {
	  stateFileId = "expected-state-file-id"
	  awsRegion = "expected-region"
	  tableName = "expected-table-name"
	  maxLockRetries = 100
	}

	remoteState = {
	  backend = "s3"
	  backendConfigs = {
	    encrypted = "true"
	    bucket = "my-bucket"
	    key = "terraform.tfstate"
	    region = "us-east-1"
	  }
	}
	`

	terragruntConfig, err := parseTerragruntConfig(config)
	assert.Nil(t, err)

	assert.NotNil(t, terragruntConfig.DynamoDbLock)
	assert.Equal(t, "expected-state-file-id", terragruntConfig.DynamoDbLock.StateFileId)
	assert.Equal(t, "expected-region", terragruntConfig.DynamoDbLock.AwsRegion)
	assert.Equal(t, "expected-table-name", terragruntConfig.DynamoDbLock.TableName)
	assert.Equal(t, 100, terragruntConfig.DynamoDbLock.MaxLockRetries)

	assert.NotNil(t, terragruntConfig.RemoteState)
	assert.Equal(t, "s3", terragruntConfig.RemoteState.Backend)
	assert.NotEmpty(t, terragruntConfig.RemoteState.BackendConfigs)
	assert.Equal(t, "true", terragruntConfig.RemoteState.BackendConfigs["encrypted"])
	assert.Equal(t, "my-bucket", terragruntConfig.RemoteState.BackendConfigs["bucket"])
	assert.Equal(t, "terraform.tfstate", terragruntConfig.RemoteState.BackendConfigs["key"])
	assert.Equal(t, "us-east-1", terragruntConfig.RemoteState.BackendConfigs["region"])
}

func TestParseTerragruntConfigEmptyConfig(t *testing.T) {
	t.Parallel()

	config := ``

	terragruntConfig, err := parseTerragruntConfig(config)
	assert.Nil(t, err)

	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.DynamoDbLock)
}