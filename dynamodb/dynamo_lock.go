package dynamodb

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/util"
)

type DynamoDbLock struct {
	StateFileId 	string
	AwsRegion   	string
	TableName   	string
	MaxLockRetries	int
}

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

func (dynamoDbLock *DynamoDbLock) Validate() error {
	if dynamoDbLock.StateFileId == "" {
		return fmt.Errorf("The dynamodb.lockType field cannot be empty")
	}

	return nil
}

func (dynamoDbLock DynamoDbLock) AcquireLock() error {
	util.Logger.Printf("Attempting to acquire lock for state file %s in DynamoDB", dynamoDbLock.StateFileId)

	client, err := createDynamoDbClient(dynamoDbLock.AwsRegion)
	if err != nil {
		return err
	}

	if err := createLockTableIfNecessary(dynamoDbLock.TableName, client); err != nil {
		return err
	}

	return writeItemToLockTableUntilSuccess(dynamoDbLock.StateFileId, dynamoDbLock.TableName, client, dynamoDbLock.MaxLockRetries)
}

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

func (dynamoLock DynamoDbLock) String() string {
	return fmt.Sprintf("DynamoDB lock for state file %s", dynamoLock.StateFileId)
}

func createDynamoDbClient(awsRegion string) (*dynamodb.DynamoDB, error) {
	config := defaults.Get().Config.WithRegion(awsRegion)

	_, err := config.Credentials.Get()
	if err != nil {
		return nil, fmt.Errorf("Error finding AWS credentials (did you set the AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables?): %s", err)
	}

	return dynamodb.New(session.New(), config), nil
}