package aws_helper

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestUnmarshalStringActionResource(t *testing.T) {
	t.Parallel()

	bucketPolicy, err := UnmarshalPolicy(simplePolicy)
	assert.NoError(t, err)
	assert.NotNil(t, bucketPolicy)
	assert.Equal(t, 1, len(bucketPolicy.Statement))
	assert.NotNil(t, bucketPolicy.Statement[0].Action)
	assert.NotNil(t, bucketPolicy.Statement[0].Resource)

	switch action := bucketPolicy.Statement[0].Action.(type) {
	case string:
		assert.Equal(t, "s3:*", action)
		break
	default:
		assert.Fail(t, "Expected string type for Action")
	}

	switch resource := bucketPolicy.Statement[0].Resource.(type) {
	case string:
		assert.Equal(t, "*", resource)
		break
	default:
		assert.Fail(t, "Expected string type for Resource")
	}
}

func TestUnmarshalActionResourceList(t *testing.T) {
	t.Parallel()
	bucketPolicy, err := UnmarshalPolicy(arraysPolicy)
	assert.NoError(t, err)
	assert.NotNil(t, bucketPolicy)
	assert.Equal(t, 1, len(bucketPolicy.Statement))
	assert.NotNil(t, bucketPolicy.Statement[0].Action)
	assert.NotNil(t, bucketPolicy.Statement[0].Resource)

	switch actions := bucketPolicy.Statement[0].Action.(type) {
	case []interface{}:
		assert.Equal(t, 11, len(actions))
		assert.Contains(t, actions, "s3:ListJobs")
		break
	default:
		assert.Fail(t, "Expected []string type for Action")
	}

	switch resource := bucketPolicy.Statement[0].Resource.(type) {
	case []interface{}:
		assert.Equal(t, 2, len(resource))
		assert.Contains(t, resource, "arn:aws:s3:*:666:job/*")
		break
	default:
		assert.Fail(t, "Expected []string type for Resource")
	}
}
