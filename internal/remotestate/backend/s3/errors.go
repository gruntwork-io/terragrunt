package s3

import "fmt"

// MissingRequiredS3RemoteStateConfig is the error for a required s3 remote_state key that is unset. Its value
// is the key name.
type MissingRequiredS3RemoteStateConfig string

func (configName MissingRequiredS3RemoteStateConfig) Error() string {
	return "Missing required S3 remote state configuration " + string(configName)
}

// MultipleTagsDeclarations is the error for tags declared in more than one block. Its value names the resource
// the tags are for.
type MultipleTagsDeclarations string

func (target MultipleTagsDeclarations) Error() string {
	return fmt.Sprintf(
		"Tags for %s declared multiple times. Please only declare tags in one block.",
		string(target),
	)
}

// MaxRetriesWaitingForS3BucketExceeded is the error for a bucket that did not appear after creation, or
// disappear after deletion, within the retry limit. Its value is the bucket name.
type MaxRetriesWaitingForS3BucketExceeded string

func (err MaxRetriesWaitingForS3BucketExceeded) Error() string {
	return fmt.Sprintf(
		"Exceeded max retries (%d) waiting for bucket S3 bucket %s",
		maxRetriesWaitingForS3Bucket,
		string(err),
	)
}

// InvalidAccessLoggingBucketEncryption is the error for an access logging bucket encryption algorithm that S3
// server access logging does not support.
type InvalidAccessLoggingBucketEncryption struct {
	BucketSSEAlgorithm string
}

func (err InvalidAccessLoggingBucketEncryption) Error() string {
	return fmt.Sprintf(
		"Encryption algorithm %s is not supported for access logging bucket. Please use a supported algorithm, like AES256",
		err.BucketSSEAlgorithm,
	)
}

// TableActiveRetriesExceeded is the error for a DynamoDB lock table that did not reach the ACTIVE status within
// Retries checks.
type TableActiveRetriesExceeded struct {
	TableName string
	Retries   int
}

func (err TableActiveRetriesExceeded) Error() string {
	return fmt.Sprintf(
		"Table %s failed to reach the 'active' state after %d retries.",
		err.TableName,
		err.Retries,
	)
}

// TableDoesNotExist is the error for a DynamoDB lock table that does not exist, wrapping the AWS error.
type TableDoesNotExist struct {
	Underlying error
	TableName  string
}

func (err TableDoesNotExist) Error() string {
	return fmt.Sprintf(
		"DynamoDB table %s does not exist! Original error from AWS: %v",
		err.TableName,
		err.Underlying,
	)
}

// TableDescriptionMissingError is returned when DescribeTable succeeds but has no table in the response.
type TableDescriptionMissingError struct {
	TableName string
}

func (err TableDescriptionMissingError) Error() string {
	return fmt.Sprintf(
		"DynamoDB DescribeTable returned no table description for table %s. "+
			"Check that dynamodb_endpoint points at a DynamoDB-compatible service.",
		err.TableName,
	)
}

// TableEncryptedRetriesExceeded is the error for a DynamoDB lock table whose server-side encryption was not
// confirmed enabled within Retries checks.
type TableEncryptedRetriesExceeded struct {
	TableName string
	Retries   int
}

func (err TableEncryptedRetriesExceeded) Error() string {
	return fmt.Sprintf(
		"Failed to confirm that DynamoDB table %s has encryption enabled after %d retries.",
		err.TableName,
		err.Retries,
	)
}

// InvalidAccountRegionalBucketName is the error for a bucket name in the account regional namespace form whose
// region suffix does not match Region, the region the bucket would be created in.
type InvalidAccountRegionalBucketName struct {
	Bucket string
	Region string
}

func (err InvalidAccountRegionalBucketName) Error() string {
	return fmt.Sprintf(
		"S3 bucket name %s is formatted for an account regional namespace, but not for region %s, where the bucket would be created. Such names take the form <prefix>-<account-id>-%s%s, e.g. my-state-111122223333-%s%s",
		err.Bucket,
		err.Region,
		err.Region,
		accountRegionalSuffix,
		err.Region,
		accountRegionalSuffix,
	)
}
