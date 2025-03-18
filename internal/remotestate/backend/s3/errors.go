package s3

import "fmt"

type MissingRequiredS3RemoteStateConfig string

func (configName MissingRequiredS3RemoteStateConfig) Error() string {
	return "Missing required S3 remote state configuration " + string(configName)
}

type MultipleTagsDeclarations string

func (target MultipleTagsDeclarations) Error() string {
	return fmt.Sprintf("Tags for %s got declared multiple times. Please do only declare in one block.", string(target))
}

type MaxRetriesWaitingForS3BucketExceeded string

func (err MaxRetriesWaitingForS3BucketExceeded) Error() string {
	return fmt.Sprintf("Exceeded max retries (%d) waiting for bucket S3 bucket %s", maxRetriesWaitingForS3Bucket, string(err))
}

type MaxRetriesWaitingForS3ACLExceeded string

func (err MaxRetriesWaitingForS3ACLExceeded) Error() string {
	return fmt.Sprintf("Exceeded max retries waiting for bucket S3 bucket %s to have proper ACL for access logging", string(err))
}

type InvalidAccessLoggingBucketEncryption struct {
	BucketSSEAlgorithm string
}

func (err InvalidAccessLoggingBucketEncryption) Error() string {
	return fmt.Sprintf("Encryption algorithm %s is not supported for access logging bucket. Please use AES256", err.BucketSSEAlgorithm)
}

type TableActiveRetriesExceeded struct {
	TableName string
	Retries   int
}

func (err TableActiveRetriesExceeded) Error() string {
	return fmt.Sprintf("Table %s is still not in active state after %d retries.", err.TableName, err.Retries)
}

type TableDoesNotExist struct {
	TableName  string
	Underlying error
}

func (err TableDoesNotExist) Error() string {
	return fmt.Sprintf("Table %s does not exist in DynamoDB! Original error from AWS: %v", err.TableName, err.Underlying)
}

type TableEncryptedRetriesExceeded struct {
	TableName string
	Retries   int
}

func (err TableEncryptedRetriesExceeded) Error() string {
	return fmt.Sprintf("Table %s still does not have encryption enabled after %d retries.", err.TableName, err.Retries)
}
