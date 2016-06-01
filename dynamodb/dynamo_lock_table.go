package dynamodb

import (
	"time"
	"fmt"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/gruntwork-io/terragrunt/errors"
)

// Create the lock table in DynamoDB if it doesn't already exist
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

// Return true if the lock table exists in DynamoDB and is in "active" state
func lockTableExistsAndIsActive(tableName string, client *dynamodb.DynamoDB) (bool, error) {
	output, err := client.DescribeTable(&dynamodb.DescribeTableInput{TableName: aws.String(tableName)})
	if err != nil {
		if awsErr, isAwsErr := err.(awserr.Error); isAwsErr && awsErr.Code() == "ResourceNotFoundException" {
			return false, nil
		} else {
			return false, errors.WithStackTrace(err)
		}
	}

	return *output.Table.TableStatus == dynamodb.TableStatusActive, nil
}

// Create a lock table in DynamoDB and wait until it is in "active" state. If the table already exists, merely wait
// until it is in "active" state.
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
		if isTableAlreadyBeingCreatedError(err) {
			util.Logger.Printf("Looks like someone created table %s at the same time. Will wait for it to be in active state.", tableName)
		} else {
			return errors.WithStackTrace(err)
		}
	}

	return waitForTableToBeActive(tableName, client, MAX_RETRIES_WAITING_FOR_TABLE_TO_BE_ACTIVE, SLEEP_BETWEEN_TABLE_STATUS_CHECKS)
}

// Return true if the given error is the error message returned by AWS when the resource already exists
func isTableAlreadyBeingCreatedError(err error) bool {
	awsErr, isAwsErr := err.(awserr.Error)
	return isAwsErr && awsErr.Code() == "ResourceInUseException"
}

// Wait for the given DynamoDB table to be in the "active" state. If it's not in "active" state, sleep for the
// specified amount of time, and try again, up to a maximum of maxRetries retries.
func waitForTableToBeActive(tableName string, client *dynamodb.DynamoDB, maxRetries int, sleepBetweenRetries time.Duration) error {
	for i := 0; i < maxRetries; i++ {
		tableReady, err := lockTableExistsAndIsActive(tableName, client)
		if err != nil {
			return err
		}

		if tableReady {
			util.Logger.Printf("Success! Table %s is now in active state.", tableName)
			return nil
		}

		util.Logger.Printf("Table %s is not yet in active state. Will check again after %s.", tableName, sleepBetweenRetries)
		time.Sleep(sleepBetweenRetries)
	}

	return errors.WithStackTrace(TableActiveRetriesExceeded{TableName: tableName, Retries: maxRetries})
}

// Remove the given item from the DynamoDB lock table
func removeItemFromLockTable(itemId string, tableName string, client *dynamodb.DynamoDB) error {
	// TODO: should we check that the entry has our own metadata and not someone else's?

	_, err := client.DeleteItem(&dynamodb.DeleteItemInput{
		Key: createKeyFromItemId(itemId),
		TableName: aws.String(tableName),
	})

	return errors.WithStackTrace(err)
}

// Write the given item to the DynamoDB lock table. If the given item already exists, return an error.
func writeItemToLockTable(itemId string, tableName string, client *dynamodb.DynamoDB) error {
	item, err := createItem(itemId)
	if err != nil {
		return err
	}

	// Conditional writes in DynamoDB should be strongly consistent: http://stackoverflow.com/a/23371813/483528
	// https://r.32k.io/locking-with-dynamodb
	_, err = client.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item: item,
		ConditionExpression: aws.String(fmt.Sprintf("attribute_not_exists(%s)", ATTR_STATE_FILE_ID)),
	})

	return errors.WithStackTrace(err)
}

// Try to write the given item to the DynamoDB lock table. If the item already exists, that means someone already has
// the lock, so display their metadata, sleep for the given amount of time, and try again, up to a maximum of
// maxRetries retries.
func writeItemToLockTableUntilSuccess(itemId string, tableName string, client *dynamodb.DynamoDB, maxRetries int, sleepBetweenRetries time.Duration) error {
	for i := 0; i < maxRetries; i++ {
		util.Logger.Printf("Attempting to create lock item for state file %s in DynamoDB table %s", itemId, tableName)

		err := writeItemToLockTable(itemId, tableName, client)
		if err == nil {
			util.Logger.Printf("Lock acquired!")
			return nil
		}

		if isItemAlreadyExistsErr(err) {
			displayLockMetadata(itemId, tableName, client)
			util.Logger.Printf("Will try to acquire lock again in %s.", sleepBetweenRetries)
			time.Sleep(sleepBetweenRetries)
		} else {
			return err
		}
	}

	return errors.WithStackTrace(AcquireLockRetriesExceeded{ItemId: itemId, Retries: maxRetries})
}

// Return true if the given error is the error returned by AWS when a conditional check fails. This is usually
// indicates an item you tried to create already exists.
func isItemAlreadyExistsErr(err error) bool {
	unwrappedErr := errors.Unwrap(err)
	awsErr, isAwsErr := unwrappedErr.(awserr.Error)
	return isAwsErr && awsErr.Code() == "ConditionalCheckFailedException"
}

type TableActiveRetriesExceeded struct {
	TableName string
	Retries   int
}

func (err TableActiveRetriesExceeded) Error() string {
	return fmt.Sprintf("Table %s is still not in active state after %d retries.", err.TableName, err.Retries)
}

type AcquireLockRetriesExceeded struct {
	ItemId  string
	Retries int
}

func (err AcquireLockRetriesExceeded) Error() string {
	return fmt.Sprintf("Unable to acquire lock for item %s after %d retries.", err.ItemId, err.Retries)
}
