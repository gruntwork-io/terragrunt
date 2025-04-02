package backend

import (
	"fmt"
)

type BucketCreationNotAllowed string

func (bucketName BucketCreationNotAllowed) Error() string {
	return fmt.Sprintf("Creation of remote state bucket %s is not allowed", string(bucketName))
}

type BucketDoesNotExistError struct {
	bucketName string
}

func NewBucketDoesNotExistError(bucketName string) *BucketDoesNotExistError {
	return &BucketDoesNotExistError{bucketName: bucketName}
}

func (err BucketDoesNotExistError) Error() string {
	return fmt.Sprintf("S3 bucket %s does not exist", err.bucketName)
}
