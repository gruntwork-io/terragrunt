//go:build aws

// Package helpers provides helper functions for tests.
package helpers

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/s3"
	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"
)

// DefaultTestRegion is for simplicity, do all testing in the us-east-1 region
const DefaultTestRegion = "us-east-1"

// DeleteS3BucketWithRetry will attempt to delete the specified S3 bucket, retrying up to 3 times if there are errors to
// handle eventual consistency issues.
func DeleteS3BucketWithRetry(t *testing.T, awsRegion string, bucketName string) {
	t.Helper()

	for i := 0; i < 3; i++ {
		err := DeleteS3Bucket(t, awsRegion, bucketName)
		if err == nil {
			return
		}

		t.Logf("Error deleting s3 bucket %s. Sleeping for 10 seconds before retrying.", bucketName)
		time.Sleep(10 * time.Second) //nolint:mnd
	}

	t.Fatalf("Max retries attempting to delete s3 bucket %s in region %s", bucketName, awsRegion)
}

// DeleteS3Bucket deletes the specified S3 bucket potentially with error to clean up after a test.
func DeleteS3Bucket(t *testing.T, awsRegion string, bucketName string, opts ...options.TerragruntOptionsFunc) error {
	t.Helper()

	client := CreateS3ClientForTest(t, DefaultTestRegion)

	t.Logf("Deleting test s3 bucket %s", bucketName)

	out, err := client.ListObjectVersions(&s3.ListObjectVersionsInput{Bucket: aws.String(bucketName)})
	if err != nil {
		t.Logf("Failed to list object versions in s3 bucket %s: %v", bucketName, err)
		return err
	}

	objectIdentifiers := []*s3.ObjectIdentifier{}
	for _, version := range out.Versions {
		objectIdentifiers = append(objectIdentifiers, &s3.ObjectIdentifier{
			Key:       version.Key,
			VersionId: version.VersionId,
		})
	}

	if len(objectIdentifiers) > 0 {
		deleteInput := &s3.DeleteObjectsInput{
			Bucket: aws.String(bucketName),
			Delete: &s3.Delete{Objects: objectIdentifiers},
		}
		if _, err := client.DeleteObjects(deleteInput); err != nil {
			t.Logf("Error deleting all versions of all objects in bucket %s: %v", bucketName, err)
			return err
		}
	}

	if _, err := client.DeleteBucket(&s3.DeleteBucketInput{Bucket: aws.String(bucketName)}); err != nil {
		t.Logf("Failed to delete S3 bucket %s: %v", bucketName, err)
		return err
	}

	return nil
}

// CreateS3ClientForTest creates a DynamoDB client we can use at test time. If there are any errors creating the client, fail the test.
func CreateS3ClientForTest(t *testing.T, awsRegion string) *s3backend.Client {
	t.Helper()

	mockOptions, err := options.NewTerragruntOptionsForTest("aws_test")
	if err != nil {
		t.Fatal(err)
	}

	extS3Cfg := &s3backend.ExtendedRemoteStateConfigS3{
		RemoteStateConfigS3: s3backend.RemoteStateConfigS3{
			Region: awsRegion,
		},
	}

	client, err := s3backend.NewClient(extS3Cfg, mockOptions)
	require.NoError(t, err, "Error creating S3 client")

	return client
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
