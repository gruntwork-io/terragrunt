//go:build aws

package awshelper_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/awshelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const simplePolicy = `
		{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Sid": "StringValues",
					"Effect": "Allow",
					"Action": "s3:*",
					"Resource": "*"
				}
			]
		}
	`

const arraysPolicy = `
		{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Sid": "Lists",
					"Effect": "Allow",
					"Action": [
						"s3:ListStorageLensConfigurations",
						"s3:ListAccessPointsForObjectLambda",
						"s3:ListBucketMultipartUploads",
						"s3:ListAllMyBuckets",
						"s3:DescribeJob",
						"s3:ListAccessPoints",
						"s3:ListJobs",
						"s3:ListBucketVersions",
						"s3:ListBucket",
						"s3:ListMultiRegionAccessPoints",
						"s3:ListMultipartUploadParts"
					],
					"Resource": [
						"arn:aws:s3:::*",
						"arn:aws:s3:*:666:job/*"
					]
				}
			]
		}
	`

func TestAwsUnmarshalStringActionResource(t *testing.T) {
	t.Parallel()

	bucketPolicy, err := awshelper.UnmarshalPolicy(simplePolicy)
	require.NoError(t, err)
	assert.NotNil(t, bucketPolicy)
	assert.Len(t, bucketPolicy.Statement, 1)
	assert.NotNil(t, bucketPolicy.Statement[0].Action)
	assert.NotNil(t, bucketPolicy.Statement[0].Resource)

	switch action := bucketPolicy.Statement[0].Action.(type) {
	case string:
		assert.Equal(t, "s3:*", action)
	default:
		assert.Fail(t, "Expected string type for Action")
	}

	switch resource := bucketPolicy.Statement[0].Resource.(type) {
	case string:
		assert.Equal(t, "*", resource)
	default:
		assert.Fail(t, "Expected string type for Resource")
	}

	out, err := awshelper.MarshalPolicy(bucketPolicy)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "null")
}

func TestAwsUnmarshalActionResourceList(t *testing.T) {
	t.Parallel()
	bucketPolicy, err := awshelper.UnmarshalPolicy(arraysPolicy)
	require.NoError(t, err)
	assert.NotNil(t, bucketPolicy)
	assert.Len(t, bucketPolicy.Statement, 1)
	assert.NotNil(t, bucketPolicy.Statement[0].Action)
	assert.NotNil(t, bucketPolicy.Statement[0].Resource)

	switch actions := bucketPolicy.Statement[0].Action.(type) {
	case []interface{}:
		assert.Len(t, actions, 11)
		assert.Contains(t, actions, "s3:ListJobs")
	default:
		assert.Fail(t, "Expected []string type for Action")
	}

	switch resource := bucketPolicy.Statement[0].Resource.(type) {
	case []interface{}:
		assert.Len(t, resource, 2)
		assert.Contains(t, resource, "arn:aws:s3:*:666:job/*")
	default:
		assert.Fail(t, "Expected []string type for Resource")
	}

	out, err := awshelper.MarshalPolicy(bucketPolicy)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "null")
}
