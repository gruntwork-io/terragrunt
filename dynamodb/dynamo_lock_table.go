package dynamodb

import (
	"time"
	"fmt"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
)

func createLockTableIfNecessary(tableName string, client *dynamodb.DynamoDB) error {
	tableExists, err := lockTableExistsAndIsActive(tableName, client)
	if err != nil {
		return err
	}

	if !tableExists {
		util.Logger.Printf("Lock table %s does not exist in DynamoDB. Will need to create it just this first time.", tableName)
		return createLockTable(tableName, client)
	}

	return nil
}

func lockTableExistsAndIsActive(tableName string, client *dynamodb.DynamoDB) (bool, error) {
	output, err := client.DescribeTable(&dynamodb.DescribeTableInput{TableName: aws.String(tableName)})
	if err != nil {
		if awsErr, isAwsErr := err.(awserr.Error); isAwsErr && awsErr.Code() == "ResourceNotFoundException" {
			return false, nil
		} else {
			return false, err
		}
	}

	return *output.Table.TableStatus == dynamodb.TableStatusActive, nil
}

func createLockTable(tableName string, client *dynamodb.DynamoDB) error {
	util.Logger.Printf("Creating table %s in DynamoDB", tableName)

	attributeDefinitions := []*dynamodb.AttributeDefinition{
		&dynamodb.AttributeDefinition{AttributeName: aws.String(ATTR_STATE_FILE_ID), AttributeType: aws.String(dynamodb.ScalarAttributeTypeS)},
	}

	keySchema := []*dynamodb.KeySchemaElement{
		&dynamodb.KeySchemaElement{AttributeName: aws.String(ATTR_STATE_FILE_ID), KeyType: aws.String(dynamodb.KeyTypeHash)},
	}

	_, err := client.CreateTable(&dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: attributeDefinitions,
		KeySchema: keySchema,
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{ReadCapacityUnits: aws.Int64(1), WriteCapacityUnits: aws.Int64(1)},
	})

	if err != nil {
		return err
	}

	return waitForTableToBeActive(tableName, client)
}

func waitForTableToBeActive(tableName string, client *dynamodb.DynamoDB) error {
	for i := 0; i < MAX_RETRIES_WAITING_FOR_TABLE_TO_BE_ACTIVE; i++ {
		tableReady, err := lockTableExistsAndIsActive(tableName, client)
		if err != nil {
			return err
		}

		if tableReady {
			util.Logger.Printf("Success! Table %s is now in active state.", tableName)
			return nil
		}

		util.Logger.Printf("Table %s is not yet in active state. Will check again after %s.", tableName, SLEEP_BETWEEN_TABLE_STATUS_CHECKS)
		time.Sleep(SLEEP_BETWEEN_TABLE_STATUS_CHECKS)
	}

	return fmt.Errorf("Table %s is still not in active state after %d retries. Exiting.", tableName, MAX_RETRIES_WAITING_FOR_TABLE_TO_BE_ACTIVE)
}

func removeItemFromLockTable(itemId string, tableName string, client *dynamodb.DynamoDB) error {
	_, err := client.DeleteItem(&dynamodb.DeleteItemInput{
		Key: createKeyFromItemId(itemId),
		TableName: aws.String(tableName),
	})

	return err
}

func writeItemToLockTable(itemId string, tableName string, client *dynamodb.DynamoDB) error {
	item, err := createItem(itemId)
	if err != nil {
		return err
	}

	_, err = client.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item: item,
		ConditionExpression: aws.String(fmt.Sprintf("attribute_not_exists(%s)", ATTR_STATE_FILE_ID)),
	})

	return err
}

func writeItemToLockTableUntilSuccess(itemId string, tableName string, client *dynamodb.DynamoDB, maxRetries int) error {
	for i := 0; i < maxRetries; i++ {
		util.Logger.Printf("Attempting to create lock item for state file %s in DynamoDB table %s", itemId, tableName)

		err := writeItemToLockTable(itemId, tableName, client)
		if err == nil {
			util.Logger.Printf("Lock acquired!")
			return nil
		}

		if isItemAlreadyExistsErr(err) {
			displayLockMetadata(itemId, tableName, client)
			util.Logger.Printf("Will try to acquire lock again in %s.", SLEEP_BETWEEN_TABLE_LOCK_ACQUIRE_ATTEMPTS)
			time.Sleep(SLEEP_BETWEEN_TABLE_LOCK_ACQUIRE_ATTEMPTS)
		} else {
			return err
		}
	}

	return fmt.Errorf("Unable to acquire lock for item %s after %d retries. Exiting.", itemId, maxRetries)
}

func isItemAlreadyExistsErr(err error) bool {
	awsErr, isAwsErr := err.(awserr.Error)
	return isAwsErr && awsErr.Code() == "ConditionalCheckFailedException"
}