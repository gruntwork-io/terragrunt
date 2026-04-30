//go:build aws || awsgcp

// Package helpers provides helper functions for tests.
package helpers

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

// DeleteS3BucketWithRetry will attempt to delete the specified S3 bucket, retrying up to 3 times if there are errors to
// handle eventual consistency issues.
func DeleteS3BucketWithRetry(t *testing.T, awsRegion string, bucketName string) {
	t.Helper()

	for range 3 {
		err := DeleteS3Bucket(t, awsRegion, bucketName)
		if err == nil {
			return
		}

		t.Logf("Error deleting s3 bucket %s. Sleeping for 10 seconds before retrying.", bucketName)
		time.Sleep(10 * time.Second) //nolint:mnd
	}

	t.Fatalf("Max retries attempting to delete s3 bucket %s in region %s", bucketName, awsRegion)
}

// GetS3BucketLoggingTarget returns the target bucket for access logging on the given S3 bucket.
func GetS3BucketLoggingTarget(t *testing.T, region, bucket string) string {
	t.Helper()

	client := CreateS3ClientForTest(t, region)

	resp, err := client.GetBucketLogging(t.Context(), &s3.GetBucketLoggingInput{
		Bucket: aws.String(bucket),
	})
	require.NoError(t, err)

	if resp.LoggingEnabled == nil {
		return ""
	}

	return aws.ToString(resp.LoggingEnabled.TargetBucket)
}

// GetS3BucketLoggingTargetPrefix returns the target prefix for access logging on the given S3 bucket.
func GetS3BucketLoggingTargetPrefix(t *testing.T, region, bucket string) string {
	t.Helper()

	client := CreateS3ClientForTest(t, region)

	resp, err := client.GetBucketLogging(t.Context(), &s3.GetBucketLoggingInput{
		Bucket: aws.String(bucket),
	})
	require.NoError(t, err)

	if resp.LoggingEnabled == nil {
		return ""
	}

	return aws.ToString(resp.LoggingEnabled.TargetPrefix)
}
