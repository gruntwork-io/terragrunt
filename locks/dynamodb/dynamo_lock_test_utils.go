package dynamodb

import (
	"time"
	"bytes"
	"math/rand"
	"testing"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/stretchr/testify/assert"
	"github.com/aws/aws-sdk-go/aws"
	"fmt"
	"github.com/gruntwork-io/terragrunt/options"
	"sync"
)

// For simplicity, do all testing in the us-east-1 region
const DEFAULT_TEST_REGION = "us-east-1"

var mockOptions = options.NewTerragruntOptionsForTest("dynamo_lock_test_utils")

var cleanupTableLock sync.Mutex

// Returns a unique (ish) id we can use to name resources so they don't conflict with each other. Uses base 62 to
// generate a 6 character string that's unlikely to collide with the handful of tests we run in parallel. Based on code
// here: http://stackoverflow.com/a/9543797/483528
func uniqueId() string {
	const BASE_62_CHARS = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	const UNIQUE_ID_LENGTH = 6 // Should be good for 62^6 = 56+ billion combinations

	var out bytes.Buffer

	random := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < UNIQUE_ID_LENGTH; i++ {
		out.WriteByte(BASE_62_CHARS[random.Intn(len(BASE_62_CHARS))])
	}

	return out.String()
}

// Create a DynamoDB client we can use at test time. If there are any errors creating the client, fail the test.
func createDynamoDbClientForTest(t *testing.T) *dynamodb.DynamoDB {
	client, err := createDynamoDbClient(DEFAULT_TEST_REGION)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func uniqueTableNameForTest() string {
	return fmt.Sprintf("terragrunt_test_%s", uniqueId())
}

func cleanupTable(t *testing.T, tableName string, client *dynamodb.DynamoDB) {

	// DynamoDB only allows 10 table creates/deletes simultaneously. Since our tests run in parallel, we may end
	// up doing more than 10, and we get the error:
	//
	// Subscriber limit exceeded: Only 10 tables can be created, updated, or deleted simultaneously
	//
	// This is a quick hack to ensure we are only ever deleting one table at a time, we should be enough to keep us
	// below the simultaneous limit.
	cleanupTableLock.Lock()
	defer cleanupTableLock.Unlock()

	_, err := client.DeleteTable(&dynamodb.DeleteTableInput{TableName: aws.String(tableName)})
	assert.Nil(t, err, "Unexpected error: %v", err)
}

func assertCanWriteToTable(t *testing.T, tableName string, client *dynamodb.DynamoDB) {
	item := createKeyFromItemId(uniqueId())

	_, err := client.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item: item,
	})

	assert.Nil(t, err, "Unexpected error: %v", err)
}

func assertItemExistsInTable(t *testing.T, itemId string, tableName string, client *dynamodb.DynamoDB) {
	output, err := client.GetItem(&dynamodb.GetItemInput{
		ConsistentRead: aws.Bool(true),
		Key: createKeyFromItemId(itemId),
		TableName: aws.String(tableName),
	})

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.NotEmpty(t, output.Item, "Did not expect item with id %s in table %s to be empty", itemId, tableName)
}

func assertItemNotExistsInTable(t *testing.T, itemId string, tableName string, client *dynamodb.DynamoDB) {
	output, err := client.GetItem(&dynamodb.GetItemInput{
		ConsistentRead: aws.Bool(true),
		Key: createKeyFromItemId(itemId),
		TableName: aws.String(tableName),
	})

	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Empty(t, output.Item, "Did not expect item with id %s in table %s to be empty", itemId, tableName)
}

func withLockTable(t *testing.T, action func(tableName string, client *dynamodb.DynamoDB)) {
	client := createDynamoDbClientForTest(t)
	tableName := uniqueTableNameForTest()

	err := createLockTableIfNecessary(tableName, client, mockOptions)
	assert.Nil(t, err, "Unexpected error: %v", err)
	defer cleanupTable(t, tableName, client)

	action(tableName, client)
}

func withLockTableProvisionedUnits(t *testing.T, readCapacityUnits int, writeCapacityUnits int, action func(tableName string, client *dynamodb.DynamoDB)) {
	client := createDynamoDbClientForTest(t)
	tableName := uniqueTableNameForTest()

	err := createLockTable(tableName, readCapacityUnits, writeCapacityUnits, client, mockOptions)
	assert.Nil(t, err, "Unexpected error: %v", err)
	defer cleanupTable(t, tableName, client)

	action(tableName, client)
}