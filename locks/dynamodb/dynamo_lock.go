package dynamodb

import (
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/locks"
	"github.com/gruntwork-io/terragrunt/options"
)

// A lock that uses AWS's DynamoDB to acquire and release locks
type DynamoDbLock struct {
	StateFileId    string
	AwsRegion      string
	TableName      string
	MaxLockRetries int
}

// New is the factory function for DynamoDbLock
func New(conf map[string]string) (locks.Lock, error) {
	lock := &DynamoDbLock{
		StateFileId:    conf["state_file_id"],
		AwsRegion:      conf["aws_region"],
		TableName:      conf["table_name"],
		MaxLockRetries: 0,
	}

	if lock.StateFileId == "" {
		return nil, errors.WithStackTrace(StateFileIdMissing)
	}

	if confMaxRetries := conf["max_lock_retries"]; confMaxRetries != "" {
		maxRetries, err := strconv.Atoi(confMaxRetries)
		if err != nil {
			return nil, errors.WithStackTrace(&InvalidMaxLockRetriesValue{err})
		}
		lock.MaxLockRetries = maxRetries
	}

	lock.fillDefaults()
	return lock, nil
}

// Fill in default configuration values for this lock
func (dynamoLock *DynamoDbLock) fillDefaults() {
	if dynamoLock.AwsRegion == "" {
		dynamoLock.AwsRegion = DEFAULT_AWS_REGION
	}

	if dynamoLock.TableName == "" {
		dynamoLock.TableName = DEFAULT_TABLE_NAME
	}

	if dynamoLock.MaxLockRetries == 0 {
		dynamoLock.MaxLockRetries = DEFAULT_MAX_RETRIES_WAITING_FOR_LOCK
	}
}

// Acquire a lock by writing an entry to DynamoDB. If that write fails, it means someone else already has the lock, so
// retry until they release the lock.
func (dynamoDbLock DynamoDbLock) AcquireLock(terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Printf("Attempting to acquire lock for state file %s in DynamoDB", dynamoDbLock.StateFileId)

	client, err := createDynamoDbClient(dynamoDbLock.AwsRegion)
	if err != nil {
		return err
	}

	if err := CreateLockTableIfNecessary(dynamoDbLock.TableName, client, terragruntOptions); err != nil {
		return err
	}

	return writeItemToLockTableUntilSuccess(dynamoDbLock.StateFileId, dynamoDbLock.TableName, client, dynamoDbLock.MaxLockRetries, SLEEP_BETWEEN_TABLE_LOCK_ACQUIRE_ATTEMPTS, terragruntOptions)
}

// Release a lock by deleting an entry from DynamoDB.
func (dynamoDbLock DynamoDbLock) ReleaseLock(terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Printf("Attempting to release lock for state file %s in DynamoDB", dynamoDbLock.StateFileId)

	client, err := createDynamoDbClient(dynamoDbLock.AwsRegion)
	if err != nil {
		return err
	}

	if err := removeItemFromLockTable(dynamoDbLock.StateFileId, dynamoDbLock.TableName, client); err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("Lock released!")
	return nil
}

// Print a string representation of this lock
func (dynamoLock DynamoDbLock) String() string {
	return fmt.Sprintf("DynamoDB lock for state file %s", dynamoLock.StateFileId)
}

// Create an authenticated client for DynamoDB
func createDynamoDbClient(awsRegion string) (*dynamodb.DynamoDB, error) {
	config, err := aws_helper.CreateAwsConfig(awsRegion)
	if err != nil {
		return nil, err
	}

	return dynamodb.New(session.New(), config), nil
}

var StateFileIdMissing = fmt.Errorf("state_file_id cannot be empty")

type InvalidMaxLockRetriesValue struct {
	ValidationErr error
}

func (err *InvalidMaxLockRetriesValue) Error() string {
	return fmt.Sprintf("unable to parse config value max_lock_retries: %s", err.ValidationErr)
}
