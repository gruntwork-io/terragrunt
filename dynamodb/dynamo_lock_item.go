package dynamodb

import (
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/locks"
	"time"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/aws/aws-sdk-go/aws"
	"fmt"
)

func createKeyFromItemId(itemId string) map[string]*dynamodb.AttributeValue {
	return map[string]*dynamodb.AttributeValue {
		ATTR_STATE_FILE_ID: &dynamodb.AttributeValue{S: aws.String(itemId)},
	}
}

func displayLockMetadata(itemId string, tableName string, client *dynamodb.DynamoDB) {
	lockMetadata, err := getLockMetadata(itemId, tableName, client)
	if err != nil {
		util.Logger.Printf("Someone already has a lock on state file %s in table %s in DynamoDB! However, failed to fetch metadata for the lock (perhaps the lock has since been released?): %s", itemId, tableName, err.Error())
	} else {
		util.Logger.Printf("Someone already has a lock on state file %s! %s@%s acquired the lock on %s.", itemId, lockMetadata.Username, lockMetadata.IpAddress, lockMetadata.DateCreated.String())
	}
}

func getLockMetadata(itemId string, tableName string, client *dynamodb.DynamoDB) (*locks.LockMetadata, error) {
	output, err := client.GetItem(&dynamodb.GetItemInput{
		Key: createKeyFromItemId(itemId),
		ConsistentRead: aws.Bool(true),
		TableName: aws.String(tableName),
	})

	if err != nil {
		return nil, err
	}

	return toLockMetadata(itemId, output.Item)
}

func toLockMetadata(itemId string, item map[string]*dynamodb.AttributeValue) (*locks.LockMetadata, error) {
	username, err := getAttribute(item, ATTR_USERNAME)
	if err != nil {
		return nil, err
	}

	ipAddress, err := getAttribute(item, ATTR_IP)
	if err != nil {
		return nil, err
	}

	dateCreatedStr, err := getAttribute(item, ATTR_CREATION_DATE)
	if err != nil {
		return nil, err
	}

	dateCreated, err := time.Parse(locks.DEFAULT_TIME_FORMAT, dateCreatedStr)
	if err != nil {
		return nil, err
	}

	return &locks.LockMetadata{
		StateFileId: itemId,
		Username: username,
		IpAddress: ipAddress,
		DateCreated: dateCreated,
	}, nil
}

func getAttribute(item map[string]*dynamodb.AttributeValue, attribute string) (string, error) {
	value, exists := item[attribute]
	if !exists {
		return "", fmt.Errorf("Could not find attribute %s in item!", attribute)
	}

	return *value.S, nil
}

func createItem(itemId string) (map[string]*dynamodb.AttributeValue, error) {
	lockMetadata, err := locks.CreateLockMetadata(itemId)
	if err != nil {
		return nil, err
	}

	return map[string]*dynamodb.AttributeValue{
		ATTR_STATE_FILE_ID: &dynamodb.AttributeValue{S: aws.String(itemId)},
		ATTR_USERNAME: &dynamodb.AttributeValue{S: aws.String(lockMetadata.Username)},
		ATTR_IP: &dynamodb.AttributeValue{S: aws.String(lockMetadata.IpAddress)},
		ATTR_CREATION_DATE: &dynamodb.AttributeValue{S: aws.String(lockMetadata.DateCreated.String())},
	}, nil
}
