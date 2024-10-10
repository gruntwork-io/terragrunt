//go:build aws

// Package helpers provides helper functions for tests.
package helpers

import (
	"testing"
	"time"
)

// DeleteS3BucketWithRetry will attempt to delete the specified S3 bucket, retrying up to 3 times if there are errors to
// handle eventual consistency issues.
func DeleteS3BucketWithRetry(t *testing.T, awsRegion string, bucketName string) {
	t.Helper()

	for i := 0; i < 3; i++ {
		err := DeleteS3BucketE(t, awsRegion, bucketName)
		if err == nil {
			return
		}

		t.Logf("Error deleting s3 bucket %s. Sleeping for 10 seconds before retrying.", bucketName)
		time.Sleep(10 * time.Second) //nolint:mnd
	}

	t.Fatalf("Max retries attempting to delete s3 bucket %s in region %s", bucketName, awsRegion)
}
