package gcs

import "fmt"

type MissingRequiredGCSRemoteStateConfig string

func (configName MissingRequiredGCSRemoteStateConfig) Error() string {
	return "Missing required GCS remote state configuration " + string(configName)
}

type MaxRetriesWaitingForGCSBucketExceeded string

func (err MaxRetriesWaitingForGCSBucketExceeded) Error() string {
	return fmt.Sprintf("Exceeded max retries (%d) waiting for bucket GCS bucket %s", maxRetriesWaitingForGcsBucket, string(err))
}
