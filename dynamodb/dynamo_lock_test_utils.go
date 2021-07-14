package dynamodb

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
)

// For simplicity, do all testing in the us-east-1 region
const DEFAULT_TEST_REGION = "us-east-1"

// Create a DynamoDB client we can use at test time. If there are any errors creating the client, fail the test.
func createDynamoDbClientForTest(t *testing.T) *dynamodb.DynamoDB {
	mockOptions, err := options.NewTerragruntOptionsForTest("dynamo_lock_test_utils")
	if err != nil {
		t.Fatal(err)
	}

	sessionConfig := &aws_helper.AwsSessionConfig{
		Region: DEFAULT_TEST_REGION,
	}

	client, err := CreateDynamoDbClient(sessionConfig, mockOptions)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func uniqueTableNameForTest() string {
	return fmt.Sprintf("terragrunt_test_%s", util.UniqueId())
}

func cleanupTableForTest(t *testing.T, tableName string, client *dynamodb.DynamoDB) {
	err := DeleteTable(tableName, client)
	assert.Nil(t, err, "Unexpected error: %v", err)
}

func assertCanWriteToTable(t *testing.T, tableName string, client *dynamodb.DynamoDB) {
	item := createKeyFromItemId(util.UniqueId())

	_, err := client.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})

	assert.Nil(t, err, "Unexpected error: %v", err)
}

func withLockTable(t *testing.T, action func(tableName string, client *dynamodb.DynamoDB)) {
	withLockTableTagged(t, nil, action)
}

func withLockTableTagged(t *testing.T, tags map[string]string, action func(tableName string, client *dynamodb.DynamoDB)) {
	client := createDynamoDbClientForTest(t)
	tableName := uniqueTableNameForTest()

	mockOptions, err := options.NewTerragruntOptionsForTest("dynamo_lock_test_utils")
	if err != nil {
		t.Fatal(err)
	}

	err = CreateLockTableIfNecessary(tableName, tags, client, mockOptions)
	assert.Nil(t, err, "Unexpected error: %v", err)
	defer cleanupTableForTest(t, tableName, client)

	action(tableName, client)
}

func createKeyFromItemId(itemId string) map[string]*dynamodb.AttributeValue {
	return map[string]*dynamodb.AttributeValue{
		ATTR_LOCK_ID: &dynamodb.AttributeValue{S: aws.String(itemId)},
	}
}
