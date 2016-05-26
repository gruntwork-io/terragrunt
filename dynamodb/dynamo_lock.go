package dynamodb

import "fmt"

type DynamoLock struct {
	StateFileId 	string
	AwsRegion   	string
	TableName   	string
}

func (dynamoLock DynamoLock) AcquireLock() error {
	// Create TableName if it doesn't exist
	// Conditionally write item to DynamoDB that contains StateFileId, username, IP, and timestamp, and only
	// succeeds if that StateFileId isn't already there
	// If you fail, keep retrying every 30 seconds until CTRL+C
	return fmt.Errorf("AcquireLock not yet implemented for DynamoDB")
}

func (dynamoLock DynamoLock) ReleaseLock() error {
	// Delete item StateFileId from DynamoDB
	return fmt.Errorf("ReleaseLock not yet implemented for DynamoDB")
}

func (dynamoLock DynamoLock) String() string {
	return fmt.Sprintf("DynamoDB lock for state file %s", dynamoLock.StateFileId)
}