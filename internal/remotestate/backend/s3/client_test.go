//go:build aws

package s3_test

import (
	"context"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultTestRegion is for simplicity, do all testing in the us-east-1 region
const defaultTestRegion = "us-east-1"

// CreateS3ClientForTest creates a DynamoDB client we can use at test time. If there are any errors creating the client, fail the test.
func CreateS3ClientForTest(t *testing.T) *s3backend.Client {
	t.Helper()

	mockOptions, err := options.NewTerragruntOptionsForTest("aws_test")
	require.NoError(t, err, "Error creating mockOptions")

	extS3Cfg := &s3backend.ExtendedRemoteStateConfigS3{
		RemoteStateConfigS3: s3backend.RemoteStateConfigS3{
			Region: defaultTestRegion,
		},
	}

	client, err := s3backend.NewClient(extS3Cfg, mockOptions)
	require.NoError(t, err, "Error creating S3 client")

	return client
}

func TestAwsCreateLockTableIfNecessaryTableDoesntAlreadyExist(t *testing.T) {
	t.Parallel()

	client := CreateS3ClientForTest(t)

	WithLockTable(t, client, func(tableName string, client *s3backend.Client) {
		AssertCanWriteToTable(t, tableName, client)
	})
}

func TestAwsCreateLockTableConcurrency(t *testing.T) {
	t.Parallel()

	client := CreateS3ClientForTest(t)
	tableName := UniqueTableNameForTest()

	defer CleanupTableForTest(t, tableName, client)

	// Use a WaitGroup to ensure the test doesn't exit before all goroutines finish.
	var waitGroup sync.WaitGroup

	// Launch a bunch of goroutines who will all try to create the same table at more or less the same time.
	// DynamoDB will, of course, only allow a single table to be created, but we still need to make sure none of
	// the goroutines report an error.
	for i := 0; i < 20; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			err := client.CreateLockTableIfNecessary(context.Background(), tableName, nil)
			assert.NoError(t, err, "Unexpected error: %v", err)
		}()
	}

	waitGroup.Wait()
}

func TestAwsWaitForTableToBeActiveTableDoesNotExist(t *testing.T) {
	t.Parallel()

	client := CreateS3ClientForTest(t)
	tableName := "terragrunt-table-does-not-exist"
	retries := 5

	err := client.WaitForTableToBeActiveWithRandomSleep(context.Background(), tableName, retries, 1*time.Millisecond, 500*time.Millisecond)

	errorMatchs := errors.IsError(err, s3backend.TableActiveRetriesExceeded{TableName: tableName, Retries: retries})
	assert.True(t, errorMatchs, "Unexpected error of type %s: %s", reflect.TypeOf(err), err)
}

func TestAwsCreateLockTableIfNecessaryTableAlreadyExists(t *testing.T) {
	t.Parallel()

	client := CreateS3ClientForTest(t)

	// Create the table the first time
	WithLockTable(t, client, func(tableName string, client *s3backend.Client) {
		AssertCanWriteToTable(t, tableName, client)

		// Try to create the table the second time and make sure you get no errors
		err := client.CreateLockTableIfNecessary(context.Background(), tableName, nil)
		require.NoError(t, err, "Unexpected error: %v", err)
	})
}

func TestAwsTableTagging(t *testing.T) {
	t.Parallel()

	client := CreateS3ClientForTest(t)
	tags := map[string]string{"team": "team A"}

	// Create the table the first time
	WithLockTableTagged(t, tags, client, func(tableName string, client *s3backend.Client) {
		AssertCanWriteToTable(t, tableName, client)

		assertTags(t, tags, tableName, client)

		// Try to create the table the second time and make sure you get no errors
		err := client.CreateLockTableIfNecessary(context.Background(), tableName, nil)
		require.NoError(t, err, "Unexpected error: %v", err)
	})
}

func assertTags(t *testing.T, expectedTags map[string]string, tableName string, client *s3backend.Client) {
	t.Helper()

	var description, err = client.DescribeTable(&dynamodb.DescribeTableInput{TableName: aws.String(tableName)})

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

func listTagsOfResourceWithRetry(t *testing.T, client *s3backend.Client, resourceArn *string) *dynamodb.ListTagsOfResourceOutput {
	t.Helper()

	const (
		delay   = 1 * time.Second
		retries = 5
	)

	for range retries {
		var tags, err = client.ListTagsOfResource(&dynamodb.ListTagsOfResourceInput{ResourceArn: resourceArn})
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

func UniqueTableNameForTest() string {
	return "terragrunt_test_" + util.UniqueID()
}

func CleanupTableForTest(t *testing.T, tableName string, client *s3backend.Client) {
	t.Helper()

	err := client.DeleteTable(context.Background(), tableName)
	require.NoError(t, err, "Unexpected error: %v", err)
}

func AssertCanWriteToTable(t *testing.T, tableName string, client *s3backend.Client) {
	t.Helper()

	item := CreateKeyFromItemID(util.UniqueID())

	_, err := client.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})

	require.NoError(t, err, "Unexpected error: %v", err)
}

func WithLockTable(t *testing.T, client *s3backend.Client, action func(tableName string, client *s3backend.Client)) {
	t.Helper()

	WithLockTableTagged(t, nil, client, action)
}

func WithLockTableTagged(t *testing.T, tags map[string]string, client *s3backend.Client, action func(tableName string, client *s3backend.Client)) {
	t.Helper()

	tableName := UniqueTableNameForTest()

	err := client.CreateLockTableIfNecessary(context.Background(), tableName, tags)
	require.NoError(t, err, "Unexpected error: %v", err)
	defer CleanupTableForTest(t, tableName, client)

	action(tableName, client)
}

func CreateKeyFromItemID(itemID string) map[string]*dynamodb.AttributeValue {
	return map[string]*dynamodb.AttributeValue{
		s3backend.AttrLockID: {S: aws.String(itemID)},
	}
}
