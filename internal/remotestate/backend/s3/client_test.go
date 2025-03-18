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
	awsDynamodb "github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAwsCreateLockTableIfNecessaryTableDoesntAlreadyExist(t *testing.T) {
	t.Parallel()

	client := helpers.CreateS3ClientForTest(t, helpers.DefaultTestRegion)

	helpers.WithLockTable(t, client, func(tableName string, client *s3backend.Client) {
		helpers.AssertCanWriteToTable(t, tableName, client)
	})
}

func TestAwsCreateLockTableConcurrency(t *testing.T) {
	t.Parallel()

	client := helpers.CreateS3ClientForTest(t, helpers.DefaultTestRegion)
	tableName := helpers.UniqueTableNameForTest()

	defer helpers.CleanupTableForTest(t, tableName, client)

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

	client := helpers.CreateS3ClientForTest(t, helpers.DefaultTestRegion)
	tableName := "terragrunt-table-does-not-exist"
	retries := 5

	err := client.WaitForTableToBeActiveWithRandomSleep(context.Background(), tableName, retries, 1*time.Millisecond, 500*time.Millisecond)

	errorMatchs := errors.IsError(err, s3backend.TableActiveRetriesExceeded{TableName: tableName, Retries: retries})
	assert.True(t, errorMatchs, "Unexpected error of type %s: %s", reflect.TypeOf(err), err)
}

func TestAwsCreateLockTableIfNecessaryTableAlreadyExists(t *testing.T) {
	t.Parallel()

	client := helpers.CreateS3ClientForTest(t, helpers.DefaultTestRegion)

	// Create the table the first time
	helpers.WithLockTable(t, client, func(tableName string, client *s3backend.Client) {
		helpers.AssertCanWriteToTable(t, tableName, client)

		// Try to create the table the second time and make sure you get no errors
		err := client.CreateLockTableIfNecessary(context.Background(), tableName, nil)
		require.NoError(t, err, "Unexpected error: %v", err)
	})
}

func TestAwsTableTagging(t *testing.T) {
	t.Parallel()

	client := helpers.CreateS3ClientForTest(t, helpers.DefaultTestRegion)
	tags := map[string]string{"team": "team A"}

	// Create the table the first time
	helpers.WithLockTableTagged(t, tags, client, func(tableName string, client *s3backend.Client) {
		helpers.AssertCanWriteToTable(t, tableName, client)

		assertTags(t, tags, tableName, client)

		// Try to create the table the second time and make sure you get no errors
		err := client.CreateLockTableIfNecessary(context.Background(), tableName, nil)
		require.NoError(t, err, "Unexpected error: %v", err)
	})
}

func assertTags(t *testing.T, expectedTags map[string]string, tableName string, client *s3backend.Client) {
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

func listTagsOfResourceWithRetry(t *testing.T, client *s3backend.Client, resourceArn *string) *awsDynamodb.ListTagsOfResourceOutput {
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
