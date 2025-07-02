package s3

import (
	"github.com/aws/aws-sdk-go-v2/aws"
)

type Retryer struct {
	aws.Retryer
}

// AWS SDK v2 doesn't expose the same retry helper functions as v1
// The retry logic is handled internally by the SDK
// This is a simplified retryer that delegates to the underlying AWS retryer
func (retryer Retryer) IsErrorRetryable(err error) bool {
	return retryer.Retryer.IsErrorRetryable(err)
}
