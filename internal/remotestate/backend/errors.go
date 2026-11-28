package backend

import (
	"fmt"
)

// BucketCreationNotAllowed is the error for a missing bucket, container, or storage account that
// bootstrap would have to create while [Options.FailIfBucketCreationRequired] is set. Its value is the
// name of the missing resource.
type BucketCreationNotAllowed string

func (bucketName BucketCreationNotAllowed) Error() string {
	return fmt.Sprintf("Creation of remote state bucket %s is not allowed", string(bucketName))
}

// BucketDoesNotExistError is the error that is returned when the bucket does not exist.
type BucketDoesNotExistError struct {
	bucketName string
}

// NewBucketDoesNotExistError creates a new `BucketDoesNotExistError` instance.
func NewBucketDoesNotExistError(bucketName string) *BucketDoesNotExistError {
	return &BucketDoesNotExistError{bucketName: bucketName}
}

// Error implements `error` interface.
func (err BucketDoesNotExistError) Error() string {
	return fmt.Sprintf("S3 bucket %s does not exist", err.bucketName)
}
