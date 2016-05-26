package dynamodb

import "fmt"

type DynamoLock struct {
	StateFileId 	string
	AwsRegion   	string
	TableName   	string
}

func (dynamoLock DynamoLock) AcquireLock() error {
	return fmt.Errorf("AcquireLock not yet implemented for DynamoDB")
}

func (dynamoLock DynamoLock) ReleaseLock() error {
	return fmt.Errorf("ReleaseLock not yet implemented for DynamoDB")
}

func (dynamoLock DynamoLock) String() string {
	return fmt.Sprintf("DynamoDB lock for state file %s", dynamoLock.StateFileId)
}