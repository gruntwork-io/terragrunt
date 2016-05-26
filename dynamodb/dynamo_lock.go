package dynamodb

import (
	"fmt"
	"time"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/gruntwork-io/terragrunt/locks"
)

type DynamoLock struct {
	StateFileId 	string
	AwsRegion   	string
	TableName   	string
}

const ATTR_STATE_FILE_ID = "StateFileId"
const ATTR_USERNAME = "Username"
const ATTR_IP = "Ip"
const ATTR_CREATION_DATE = "CreationDate"

const SLEEP_BETWEEN_TABLE_STATUS_CHECKS = 10 * time.Second
const SLEEP_BETWEEN_TABLE_LOCK_ACQUIRE_ATTEMPTS = 10 * time.Second

func (dynamoLock DynamoLock) AcquireLock() error {
	util.Logger.Printf("Attempting to acquire lock for state file %s in DynamoDB", dynamoLock.StateFileId)

	client, err := createDynamoDbClient(dynamoLock.AwsRegion)
	if err != nil {
		return err
	}

	if err := createLockTableIfNecessary(dynamoLock.TableName, client); err != nil {
		return err
	}

	return writeItemToLockTableUntilSuccess(dynamoLock.StateFileId, dynamoLock.TableName, client)
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

func createLockTableIfNecessary(tableName string, client *dynamodb.DynamoDB) error {
	tableExists, err := lockTableExistsAndIsActive(tableName, client)
	if err != nil {
		return err
	}

	if !tableExists {
		util.Logger.Printf("Lock table %s does not exist in DynamoDB. Will need to create it just this first time.")
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
	for {
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
}

func removeItemFromLockTable(itemId string, tableName string, client *dynamodb.DynamoDB) error {
	_, err := client.DeleteItem(&dynamodb.DeleteItemInput{
		Key: createKeyFromItemId(itemId),
		TableName: aws.String(tableName),
	})

	return err
}

func createKeyFromItemId(itemId string) map[string]*dynamodb.AttributeValue {
	return map[string]*dynamodb.AttributeValue {
		ATTR_STATE_FILE_ID: &dynamodb.AttributeValue{S: aws.String(itemId)},
	}
}

func writeItemToLockTableUntilSuccess(itemId string, tableName string, client *dynamodb.DynamoDB) error {
	item, err := createItem(itemId)
	if err != nil {
		return err
	}

	for {
		util.Logger.Printf("Attempting to create lock item for state file %s in DynamoDB table %s", itemId, tableName)

		_, err = client.PutItem(&dynamodb.PutItemInput{
			TableName: aws.String(tableName),
			Item: item,
			ConditionExpression: aws.String(fmt.Sprintf("attribute_not_exists(%s)", ATTR_STATE_FILE_ID)),
		})

		if err == nil {
			util.Logger.Printf("Lock acquired!")
			return nil
		}

		if awsErr, isAwsErr := err.(awserr.Error); isAwsErr && awsErr.Code() == "ConditionalCheckFailedException" {
			displayLockMetadata(itemId, tableName, client)
			util.Logger.Printf("Will try to acquire lock again in %s.", SLEEP_BETWEEN_TABLE_LOCK_ACQUIRE_ATTEMPTS)
			time.Sleep(SLEEP_BETWEEN_TABLE_LOCK_ACQUIRE_ATTEMPTS)
		} else {
			return err
		}
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

	return toLockMetadata(output.Item)
}

func toLockMetadata(item map[string]*dynamodb.AttributeValue) (*locks.LockMetadata, error) {
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
	lockMetadata, err := locks.CreateLockMetadata()
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


func createDynamoDbClient(awsRegion string) (*dynamodb.DynamoDB, error) {
	config := defaults.Get().Config.WithRegion(awsRegion)

	_, err := config.Credentials.Get()
	if err != nil {
		return nil, fmt.Errorf("Error finding AWS credentials (did you set the AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables?): %s", err)
	}

	return dynamodb.New(session.New(), config), nil
}