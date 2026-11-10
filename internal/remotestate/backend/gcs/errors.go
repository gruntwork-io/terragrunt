package gcs

import "fmt"

// MissingRequiredGCSRemoteStateConfig is the error for a required gcs remote_state key that is unset. Its value
// is the key name.
type MissingRequiredGCSRemoteStateConfig string

func (configName MissingRequiredGCSRemoteStateConfig) Error() string {
	return "Missing required GCS remote state configuration " + string(configName)
}

// MaxRetriesWaitingForGCSBucketExceeded is the error for a bucket that did not appear after creation, or
// disappear after deletion, within the retry limit. Its value is the bucket name.
type MaxRetriesWaitingForGCSBucketExceeded string

func (err MaxRetriesWaitingForGCSBucketExceeded) Error() string {
	return fmt.Sprintf(
		"Exceeded max retries (%d) waiting for bucket GCS bucket %s",
		maxRetriesWaitingForGcsBucket,
		string(err),
	)
}
