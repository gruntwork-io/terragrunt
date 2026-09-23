//go:build aws || awsgcp

// Package helpers provides helper functions for tests.
package helpers

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

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
