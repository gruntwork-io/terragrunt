//go:build aws

package dynamodb_test

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	awsDynamodb "github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/awshelper"
	"github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"
)

// For simplicity, do all testing in the us-east-1 region
const defaultTestRegion = "us-east-1"

// Create a DynamoDB client we can use at test time. If there are any errors creating the client, fail the test.
func createDynamoDBClientForTest(t *testing.T) *awsDynamodb.DynamoDB {
	t.Helper()

	mockOptions, err := options.NewTerragruntOptionsForTest("dynamo_lock_test_utils")
	if err != nil {
		t.Fatal(err)
	}

	sessionConfig := &awshelper.AwsSessionConfig{
		Region: defaultTestRegion,
	}

	client, err := dynamodb.CreateDynamoDBClient(sessionConfig, mockOptions)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func uniqueTableNameForTest() string {
	return "terragrunt_test_" + util.UniqueID()
}

func cleanupTableForTest(t *testing.T, tableName string, client *awsDynamodb.DynamoDB) {
	t.Helper()

	err := dynamodb.DeleteTable(tableName, client)
	require.NoError(t, err, "Unexpected error: %v", err)
}

func assertCanWriteToTable(t *testing.T, tableName string, client *awsDynamodb.DynamoDB) {
	t.Helper()

	item := createKeyFromItemID(util.UniqueID())

	_, err := client.PutItem(&awsDynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})

	require.NoError(t, err, "Unexpected error: %v", err)
}

func withLockTable(t *testing.T, action func(tableName string, client *awsDynamodb.DynamoDB)) {
	t.Helper()

	withLockTableTagged(t, nil, action)
}

func withLockTableTagged(t *testing.T, tags map[string]string, action func(tableName string, client *awsDynamodb.DynamoDB)) {
	t.Helper()

	client := createDynamoDBClientForTest(t)
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

func createKeyFromItemID(itemID string) map[string]*awsDynamodb.AttributeValue {
	return map[string]*awsDynamodb.AttributeValue{
		dynamodb.AttrLockID: {S: aws.String(itemID)},
	}
}
