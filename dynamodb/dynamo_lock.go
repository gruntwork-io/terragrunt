package dynamodb

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/util"
)

type DynamoLock struct {
	StateFileId 	string
	AwsRegion   	string
	TableName   	string
	MaxLockRetries	int
}

func (dynamoLock DynamoLock) AcquireLock() error {
	util.Logger.Printf("Attempting to acquire lock for state file %s in DynamoDB", dynamoLock.StateFileId)

	client, err := createDynamoDbClient(dynamoLock.AwsRegion)
	if err != nil {
		return err
	}

	if err := createLockTableIfNecessary(dynamoLock.TableName, client); err != nil {
		return err
	}

	return writeItemToLockTableUntilSuccess(dynamoLock.StateFileId, dynamoLock.TableName, client, dynamoLock.MaxLockRetries)
}

func (dynamoLock DynamoLock) ReleaseLock() error {
	util.Logger.Printf("Attempting to release lock for state file %s in DynamoDB", dynamoLock.StateFileId)

	client, err := createDynamoDbClient(dynamoLock.AwsRegion)
	if err != nil {
		return err
	}

	if err := removeItemFromLockTable(dynamoLock.StateFileId, dynamoLock.TableName, client); err != nil {
		return err
	}

	util.Logger.Printf("Lock released!")
	return nil
}

func (dynamoLock DynamoLock) String() string {
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