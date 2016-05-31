package dynamodb

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/gruntwork-io/terragrunt/errors"
)

// A lock that uses AWS's DynamoDB to acquire and release locks
type DynamoDbLock struct {
	StateFileId 	string
	AwsRegion   	string
	TableName   	string
	MaxLockRetries	int
}

// Fill in default configuration values for this lock
func (dynamoLock *DynamoDbLock) FillDefaults() {
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

// Validate that this lock is configured correctly
func (dynamoDbLock *DynamoDbLock) Validate() error {
	if dynamoDbLock.StateFileId == "" {
		return errors.WithStackTrace(StateFileIdMissing)
	}

	return nil
}

// Acquire a lock by writing an entry to DynamoDB. If that write fails, it means someone else already has the lock, so
// retry until they release the lock.
func (dynamoDbLock DynamoDbLock) AcquireLock() error {
	util.Logger.Printf("Attempting to acquire lock for state file %s in DynamoDB", dynamoDbLock.StateFileId)

	client, err := createDynamoDbClient(dynamoDbLock.AwsRegion)
	if err != nil {
		return err
	}

	if err := createLockTableIfNecessary(dynamoDbLock.TableName, client); err != nil {
		return err
	}

	return writeItemToLockTableUntilSuccess(dynamoDbLock.StateFileId, dynamoDbLock.TableName, client, dynamoDbLock.MaxLockRetries, SLEEP_BETWEEN_TABLE_LOCK_ACQUIRE_ATTEMPTS)
}

// Release a lock by deleting an entry from DynamoDB.
func (dynamoDbLock DynamoDbLock) ReleaseLock() error {
	util.Logger.Printf("Attempting to release lock for state file %s in DynamoDB", dynamoDbLock.StateFileId)

	client, err := createDynamoDbClient(dynamoDbLock.AwsRegion)
	if err != nil {
		return err
	}

	if err := removeItemFromLockTable(dynamoDbLock.StateFileId, dynamoDbLock.TableName, client); err != nil {
		return err
	}

	util.Logger.Printf("Lock released!")
	return nil
}

// Print a string representation of this lock
func (dynamoLock DynamoDbLock) String() string {
	return fmt.Sprintf("DynamoDB lock for state file %s", dynamoLock.StateFileId)
}

// Create an authenticated client for DynamoDB
func createDynamoDbClient(awsRegion string) (*dynamodb.DynamoDB, error) {
	config := defaults.Get().Config.WithRegion(awsRegion)

	_, err := config.Credentials.Get()
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error finding AWS credentials (did you set the AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables?)")
	}

	return dynamodb.New(session.New(), config), nil
}

var StateFileIdMissing = fmt.Errorf("The dynamodb.stateFileId field cannot be empty")


