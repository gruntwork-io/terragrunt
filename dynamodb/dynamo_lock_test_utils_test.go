package dynamodb_test

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	awsDynamodb "github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"
)

// For simplicity, do all testing in the us-east-1 region
const DEFAULT_TEST_REGION = "us-east-1"

// Create a DynamoDB client we can use at test time. If there are any errors creating the client, fail the test.
func createDynamoDbClientForTest(t *testing.T) *awsDynamodb.DynamoDB {
	mockOptions, err := options.NewTerragruntOptionsForTest("dynamo_lock_test_utils")
	if err != nil {
		t.Fatal(err)
	}

	sessionConfig := &aws_helper.AwsSessionConfig{
		Region: DEFAULT_TEST_REGION,
	}

	client, err := dynamodb.CreateDynamoDbClient(sessionConfig, mockOptions)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func uniqueTableNameForTest() string {
	return "terragrunt_test_" + util.UniqueId()
}

func cleanupTableForTest(t *testing.T, tableName string, client *awsDynamodb.DynamoDB) {
	err := dynamodb.DeleteTable(tableName, client)
	require.NoError(t, err, "Unexpected error: %v", err)
}

func assertCanWriteToTable(t *testing.T, tableName string, client *awsDynamodb.DynamoDB) {
	item := createKeyFromItemId(util.UniqueId())

	_, err := client.PutItem(&awsDynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})

	require.NoError(t, err, "Unexpected error: %v", err)
}

func withLockTable(t *testing.T, action func(tableName string, client *awsDynamodb.DynamoDB)) {
	withLockTableTagged(t, nil, action)
}

func withLockTableTagged(t *testing.T, tags map[string]string, action func(tableName string, client *awsDynamodb.DynamoDB)) {
	client := createDynamoDbClientForTest(t)
	tableName := uniqueTableNameForTest()

	mockOptions, err := options.NewTerragruntOptionsForTest("dynamo_lock_test_utils")
	if err != nil {
		t.Fatal(err)
	}

	err = dynamodb.CreateLockTableIfNecessary(tableName, tags, client, mockOptions)
	require.NoError(t, err, "Unexpected error: %v", err)
	defer cleanupTableForTest(t, tableName, client)

	action(tableName, client)
}

func createKeyFromItemId(itemId string) map[string]*awsDynamodb.AttributeValue {
	return map[string]*awsDynamodb.AttributeValue{
		dynamodb.ATTR_LOCK_ID: {S: aws.String(itemId)},
	}
}
