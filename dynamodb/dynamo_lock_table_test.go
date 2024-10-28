//go:build aws

package dynamodb_test

import (
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	awsDynamodb "github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/dynamodb"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAwsCreateLockTableIfNecessaryTableDoesntAlreadyExist(t *testing.T) {
	t.Parallel()

	withLockTable(t, func(tableName string, client *awsDynamodb.DynamoDB) {
		assertCanWriteToTable(t, tableName, client)
	})
}

func TestAwsCreateLockTableConcurrency(t *testing.T) {
	t.Parallel()

	mockOptions, err := options.NewTerragruntOptionsForTest("dynamo_lock_test_utils")
	if err != nil {
		t.Fatal(err)
	}

	client := createDynamoDBClientForTest(t)
	tableName := uniqueTableNameForTest()

	defer cleanupTableForTest(t, tableName, client)

	// Use a WaitGroup to ensure the test doesn't exit before all goroutines finish.
	var waitGroup sync.WaitGroup

	// Launch a bunch of goroutines who will all try to create the same table at more or less the same time.
	// DynamoDB will, of course, only allow a single table to be created, but we still need to make sure none of
	// the goroutines report an error.
	for i := 0; i < 20; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			err := dynamodb.CreateLockTableIfNecessary(tableName, nil, client, mockOptions)
			assert.NoError(t, err, "Unexpected error: %v", err)
		}()
	}

	waitGroup.Wait()
}

func TestAwsWaitForTableToBeActiveTableDoesNotExist(t *testing.T) {
	t.Parallel()

	mockOptions, err := options.NewTerragruntOptionsForTest("dynamo_lock_test_utils")
	if err != nil {
		t.Fatal(err)
	}

	client := createDynamoDBClientForTest(t)
	tableName := "terragrunt-table-does-not-exist"
	retries := 5

	err = dynamodb.WaitForTableToBeActiveWithRandomSleep(tableName, client, retries, 1*time.Millisecond, 500*time.Millisecond, mockOptions)

	assert.True(t, errors.IsError(err, dynamodb.TableActiveRetriesExceeded{TableName: tableName, Retries: retries}), "Unexpected error of type %s: %s", reflect.TypeOf(err), err)
}

func TestAwsCreateLockTableIfNecessaryTableAlreadyExists(t *testing.T) {
	t.Parallel()

	mockOptions, err := options.NewTerragruntOptionsForTest("dynamo_lock_test_utils")
	if err != nil {
		t.Fatal(err)
	}

	// Create the table the first time
	withLockTable(t, func(tableName string, client *awsDynamodb.DynamoDB) {
		assertCanWriteToTable(t, tableName, client)

		// Try to create the table the second time and make sure you get no errors
		err = dynamodb.CreateLockTableIfNecessary(tableName, nil, client, mockOptions)
		require.NoError(t, err, "Unexpected error: %v", err)
	})
}

func TestAwsTableTagging(t *testing.T) {
	t.Parallel()

	mockOptions, err := options.NewTerragruntOptionsForTest("dynamo_lock_test_utils")
	if err != nil {
		t.Fatal(err)
	}

	tags := map[string]string{"team": "team A"}

	// Create the table the first time
	withLockTableTagged(t, tags, func(tableName string, client *awsDynamodb.DynamoDB) {
		assertCanWriteToTable(t, tableName, client)

		assertTags(t, tags, tableName, client)

		// Try to create the table the second time and make sure you get no errors
		err = dynamodb.CreateLockTableIfNecessary(tableName, nil, client, mockOptions)
		require.NoError(t, err, "Unexpected error: %v", err)
	})
}

func assertTags(t *testing.T, expectedTags map[string]string, tableName string, client *awsDynamodb.DynamoDB) {
	t.Helper()

	var description, err = client.DescribeTable(&awsDynamodb.DescribeTableInput{TableName: aws.String(tableName)})

	if err != nil {
		require.NoError(t, err, "Unexpected error: %v", err)
	}

	var tags = listTagsOfResourceWithRetry(t, client, description.Table.TableArn)

	var actualTags = make(map[string]string)

	for _, element := range tags.Tags {
		actualTags[*element.Key] = *element.Value
	}

	assert.Equal(t, expectedTags, actualTags, "Did not find expected tags on dynamo table.")
}

func listTagsOfResourceWithRetry(t *testing.T, client *awsDynamodb.DynamoDB, resourceArn *string) *awsDynamodb.ListTagsOfResourceOutput {
	t.Helper()

	const (
		delay   = 1 * time.Second
		retries = 5
	)

	for range retries {
		var tags, err = client.ListTagsOfResource(&awsDynamodb.ListTagsOfResourceInput{ResourceArn: resourceArn})
		if err != nil {
			require.NoError(t, err, "Unexpected error: %v", err)
		}

		if len(tags.Tags) > 0 {
			return tags
		}

		time.Sleep(delay)
	}

	require.Failf(t, "Could not list tags of resource after %s retries.", strconv.Itoa(retries))
	return nil
}
