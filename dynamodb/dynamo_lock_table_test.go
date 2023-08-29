package dynamodb

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

func TestCreateLockTableIfNecessaryTableDoesntAlreadyExist(t *testing.T) {
	t.Parallel()

	withLockTable(t, func(tableName string, client *dynamodb.DynamoDB) {
		assertCanWriteToTable(t, tableName, client)
	})
}

func TestCreateLockTableConcurrency(t *testing.T) {
	t.Parallel()

	mockOptions, err := options.NewTerragruntOptionsForTest("dynamo_lock_test_utils")
	if err != nil {
		t.Fatal(err)
	}

	client := createDynamoDbClientForTest(t)
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
			err := CreateLockTableIfNecessary(tableName, nil, client, mockOptions)
			assert.Nil(t, err, "Unexpected error: %v", err)
		}()
	}

	waitGroup.Wait()
}

func TestWaitForTableToBeActiveTableDoesNotExist(t *testing.T) {
	t.Parallel()

	mockOptions, err := options.NewTerragruntOptionsForTest("dynamo_lock_test_utils")
	if err != nil {
		t.Fatal(err)
	}

	client := createDynamoDbClientForTest(t)
	tableName := "terragrunt-table-does-not-exist"
	retries := 5

	err = waitForTableToBeActiveWithRandomSleep(tableName, client, retries, 1*time.Millisecond, 500*time.Millisecond, mockOptions)

	assert.True(t, errors.IsError(err, TableActiveRetriesExceeded{TableName: tableName, Retries: retries}), "Unexpected error of type %s: %s", reflect.TypeOf(err), err)
}

func TestCreateLockTableIfNecessaryTableAlreadyExists(t *testing.T) {
	t.Parallel()

	mockOptions, err := options.NewTerragruntOptionsForTest("dynamo_lock_test_utils")
	if err != nil {
		t.Fatal(err)
	}

	// Create the table the first time
	withLockTable(t, func(tableName string, client *dynamodb.DynamoDB) {
		assertCanWriteToTable(t, tableName, client)

		// Try to create the table the second time and make sure you get no errors
		err = CreateLockTableIfNecessary(tableName, nil, client, mockOptions)
		assert.Nil(t, err, "Unexpected error: %v", err)
	})
}

func TestTableTagging(t *testing.T) {
	t.Parallel()

	mockOptions, err := options.NewTerragruntOptionsForTest("dynamo_lock_test_utils")
	if err != nil {
		t.Fatal(err)
	}

	tags := map[string]string{"team": "team A"}

	// Create the table the first time
	withLockTableTagged(t, tags, func(tableName string, client *dynamodb.DynamoDB) {
		assertCanWriteToTable(t, tableName, client)

		assertTags(tags, tableName, client, t)

		// Try to create the table the second time and make sure you get no errors
		err = CreateLockTableIfNecessary(tableName, nil, client, mockOptions)
		assert.Nil(t, err, "Unexpected error: %v", err)
	})
}

func assertTags(expectedTags map[string]string, tableName string, client *dynamodb.DynamoDB, t *testing.T) {
	var description, err = client.DescribeTable(&dynamodb.DescribeTableInput{TableName: aws.String(tableName)})

	if err != nil {
		t.Fatal(err)
	}

	var tags, err2 = client.ListTagsOfResource(&dynamodb.ListTagsOfResourceInput{ResourceArn: description.Table.TableArn})

	if err2 != nil {
		t.Fatal(err2)
	}

	var actualTags = make(map[string]string)

	for _, element := range tags.Tags {
		actualTags[*element.Key] = *element.Value
	}

	assert.Equal(t, expectedTags, actualTags, "Did not find expected tags on dynamo table.")
}
