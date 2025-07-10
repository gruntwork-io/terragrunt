//go:build aws

package s3_test

import (
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
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

	l := logger.CreateLogger()

	client, err := s3backend.NewClient(t.Context(), l, extS3Cfg, mockOptions)
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

	l := logger.CreateLogger()

	// Launch a bunch of goroutines who will all try to create the same table at more or less the same time.
	// DynamoDB will, of course, only allow a single table to be created, but we still need to make sure none of
	// the goroutines report an error.
	for i := 0; i < 20; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			err := client.CreateLockTableIfNecessary(t.Context(), l, tableName, nil)
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

	l := logger.CreateLogger()

	err := client.WaitForTableToBeActiveWithRandomSleep(t.Context(), l, tableName, retries, 1*time.Millisecond, 500*time.Millisecond)

	errorMatchs := errors.IsError(err, s3backend.TableActiveRetriesExceeded{TableName: tableName, Retries: retries})
	assert.True(t, errorMatchs, "Unexpected error of type %s: %s", reflect.TypeOf(err), err)
}

func TestAwsCreateLockTableIfNecessaryTableAlreadyExists(t *testing.T) {
	t.Parallel()

	client := CreateS3ClientForTest(t)

	// Create the table the first time
	WithLockTable(t, client, func(tableName string, client *s3backend.Client) {
		AssertCanWriteToTable(t, tableName, client)

		l := logger.CreateLogger()

		// Try to create the table the second time and make sure you get no errors
		err := client.CreateLockTableIfNecessary(t.Context(), l, tableName, nil)
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

		l := logger.CreateLogger()

		// Try to create the table the second time and make sure you get no errors
		err := client.CreateLockTableIfNecessary(t.Context(), l, tableName, nil)
		require.NoError(t, err, "Unexpected error: %v", err)
	})
}

func assertTags(t *testing.T, expectedTags map[string]string, tableName string, client *s3backend.Client) {
	t.Helper()

	// Access the dynamodb client directly from the S3 client
	dynamoClient := client.GetDynamoDBClient()

	var description, err = dynamoClient.DescribeTable(t.Context(), &dynamodb.DescribeTableInput{TableName: aws.String(tableName)})

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

	// Access the dynamodb client directly from the S3 client
	dynamoClient := client.GetDynamoDBClient()

	for range retries {
		var tags, err = dynamoClient.ListTagsOfResource(t.Context(), &dynamodb.ListTagsOfResourceInput{ResourceArn: resourceArn})
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

	l := logger.CreateLogger()

	err := client.DeleteTable(t.Context(), l, tableName)
	require.NoError(t, err, "Unexpected error: %v", err)
}

func AssertCanWriteToTable(t *testing.T, tableName string, client *s3backend.Client) {
	t.Helper()

	item := CreateKeyFromItemID(util.UniqueID())

	// Access the dynamodb client directly from the S3 client
	dynamoClient := client.GetDynamoDBClient()

	_, err := dynamoClient.PutItem(t.Context(), &dynamodb.PutItemInput{
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
	defer CleanupTableForTest(t, tableName, client)

	l := logger.CreateLogger()

	err := client.CreateLockTableIfNecessary(t.Context(), l, tableName, tags)
	require.NoError(t, err, "Unexpected error: %v", err)

	action(tableName, client)
}

func CreateKeyFromItemID(itemID string) map[string]dynamodbtypes.AttributeValue {
	return map[string]dynamodbtypes.AttributeValue{
		"LockID": &dynamodbtypes.AttributeValueMemberS{Value: itemID},
	}
}
