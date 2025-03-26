package backend

import (
	"fmt"
)

type BucketCreationNotAllowed string

func (bucketName BucketCreationNotAllowed) Error() string {
	return fmt.Sprintf("Creation of remote state bucket %s is not allowed", string(bucketName))
}
