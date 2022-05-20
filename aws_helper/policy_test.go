package aws_helper

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnmarshalActionList(t *testing.T) {
	t.Parallel()
	policy := `
		{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Sid": "ArrayList",
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
					"Resource": "*"
				}
			]
		}
	`
	bucketPolicy, err := UnmarshalPolicy(policy)
	assert.NoError(t, err)
	assert.NotNil(t, bucketPolicy)
	assert.Equal(t, 1, len(bucketPolicy.Statement))
	assert.NotNil(t, bucketPolicy.Statement[0].Action)

	switch actions := bucketPolicy.Statement[0].Action.(type) {
	case []interface{}:
		assert.Equal(t, 11, len(actions))
		break
	case string:
	default:
		fmt.Printf("Actions: %v \n", actions)
		assert.Fail(t, "Expected to be []string type for Action")
	}
}
