package dynamodb

import (
	"bytes"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
	"time"
)

// For simplicity, do all testing in the us-east-1 region
const DEFAULT_TEST_REGION = "us-east-1"

func init() {
	rand.Seed(time.Now().UnixNano())
}

var mockOptions, _ = options.NewTerragruntOptionsForTest("dynamo_lock_test_utils")

// Returns a unique (ish) id we can use to name resources so they don't conflict with each other. Uses base 62 to
// generate a 6 character string that's unlikely to collide with the handful of tests we run in parallel. Based on code
// here: http://stackoverflow.com/a/9543797/483528
func uniqueId() string {
	const BASE_62_CHARS = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	const UNIQUE_ID_LENGTH = 6 // Should be good for 62^6 = 56+ billion combinations

	var out bytes.Buffer

	for i := 0; i < UNIQUE_ID_LENGTH; i++ {
		out.WriteByte(BASE_62_CHARS[rand.Intn(len(BASE_62_CHARS))])
	}

	return out.String()
}

// Create a DynamoDB client we can use at test time. If there are any errors creating the client, fail the test.
func createDynamoDbClientForTest(t *testing.T) *dynamodb.DynamoDB {
	client, err := CreateDynamoDbClient(DEFAULT_TEST_REGION, "", "", mockOptions)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func uniqueTableNameForTest() string {
	return fmt.Sprintf("terragrunt_test_%s", uniqueId())
}

func cleanupTableForTest(t *testing.T, tableName string, client *dynamodb.DynamoDB) {
	err := DeleteTable(tableName, client)
	assert.Nil(t, err, "Unexpected error: %v", err)
}

func assertCanWriteToTable(t *testing.T, tableName string, client *dynamodb.DynamoDB) {
	item := createKeyFromItemId(uniqueId())

	_, err := client.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})

	assert.Nil(t, err, "Unexpected error: %v", err)
}

func withLockTable(t *testing.T, action func(tableName string, client *dynamodb.DynamoDB)) {
	client := createDynamoDbClientForTest(t)
	tableName := uniqueTableNameForTest()

	err := CreateLockTableIfNecessary(tableName, client, mockOptions)
	assert.Nil(t, err, "Unexpected error: %v", err)
	defer cleanupTableForTest(t, tableName, client)

	action(tableName, client)
}

func createKeyFromItemId(itemId string) map[string]*dynamodb.AttributeValue {
	return map[string]*dynamodb.AttributeValue{
		ATTR_LOCK_ID: &dynamodb.AttributeValue{S: aws.String(itemId)},
	}
}
