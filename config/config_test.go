package config

import (
	"testing"
	"github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/stretchr/testify/assert"
)

func TestGetLockForDynamoDbMinimalConfig(t *testing.T) {
	t.Parallel()

	config :=
	`
	lockType = "dynamodb"
	stateFileId = "expected-state-file-id"
	`

	lock, err := getLockForConfig(config)

	assert.Nil(t, err)
	assert.IsType(t, dynamodb.DynamoLock{}, lock)

	dynamoLock := lock.(dynamodb.DynamoLock)
	assert.Equal(t, "expected-state-file-id", dynamoLock.StateFileId)
	assert.Equal(t, DEFAULT_AWS_REGION, dynamoLock.AwsRegion)
	assert.Equal(t, DEFAULT_TABLE_NAME, dynamoLock.TableName)
}

func TestGetLockForDynamoDbFullConfig(t *testing.T) {
	t.Parallel()

	config :=
	`
	lockType = "dynamodb"
	stateFileId = "expected-state-file-id"

	dynamoLock = {
	  awsRegion = "expected-region"
	  tableName = "expected-table-name"
	}
	`

	lock, err := getLockForConfig(config)

	assert.Nil(t, err)
	assert.IsType(t, dynamodb.DynamoLock{}, lock)

	dynamoLock := lock.(dynamodb.DynamoLock)
	assert.Equal(t, "expected-state-file-id", dynamoLock.StateFileId)
	assert.Equal(t, "expected-region", dynamoLock.AwsRegion)
	assert.Equal(t, "expected-table-name", dynamoLock.TableName)
}

func TestGetLockForEmptyConfig(t *testing.T) {
	t.Parallel()

	config := ``

	_, err := getLockForConfig(config)
	assert.NotNil(t, err)
}

func TestGetLockForConfigWithNoStateFileId(t *testing.T) {
	t.Parallel()

	config :=
	`
	lockType = "dynamodb"
	`

	_, err := getLockForConfig(config)
	assert.NotNil(t, err)
}

func TestGetLockForConfigWithNoLockType(t *testing.T) {
	t.Parallel()

	config :=
	`
	stateFileId = "foo"
	`

	_, err := getLockForConfig(config)
	assert.NotNil(t, err)
}

func TestGetLockForConfigWithInvalidLockType(t *testing.T) {
	t.Parallel()

	config :=
	`
	lockType = "not-a-valid-lock-type"
	stateFileId = "foo"
	`

	_, err := getLockForConfig(config)
	assert.NotNil(t, err)
}