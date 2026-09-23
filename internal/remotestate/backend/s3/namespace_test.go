package s3_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBucketNamespace(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		bucket   string
		region   string
		expected types.BucketNamespace
	}{
		{
			name:     "plain name",
			bucket:   "my-terraform-state",
			region:   "us-east-1",
			expected: "",
		},
		{
			name:     "suffix without the separating hyphen",
			bucket:   "my-state-japan",
			region:   "us-east-1",
			expected: "",
		},
		{
			name:     "account regional name",
			bucket:   "my-state-111122223333-us-east-1-an",
			region:   "us-east-1",
			expected: types.BucketNamespaceAccountRegional,
		},
		{
			name:     "account regional name in a region whose code has four parts",
			bucket:   "my-state-111122223333-us-gov-west-1-an",
			region:   "us-gov-west-1",
			expected: types.BucketNamespaceAccountRegional,
		},
		{
			name:     "no region",
			bucket:   "my-state-111122223333-an",
			region:   "us-east-1",
			expected: "",
		},
		{
			name:     "account ID of the wrong length",
			bucket:   "my-state-1111222233-us-east-1-an",
			region:   "us-east-1",
			expected: "",
		},
		{
			name:     "account ID that is not a number",
			bucket:   "my-state-caaabbbbcccc-us-east-1-an",
			region:   "us-east-1",
			expected: "",
		},
		{
			name:     "no prefix ahead of the account ID",
			bucket:   "111122223333-us-east-1-an",
			region:   "us-east-1",
			expected: "",
		},
		{
			name:     "prefix containing periods",
			bucket:   "my.state-111122223333-eu-central-1-an",
			region:   "eu-central-1",
			expected: types.BucketNamespaceAccountRegional,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			namespace, err := s3backend.BucketNamespace(tc.bucket, tc.region)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, namespace)
		})
	}
}

func TestBucketNamespaceRejectsAnotherRegion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		bucket string
		region string
	}{
		{
			name:   "region of another bucket",
			bucket: "my-state-111122223333-us-west-2-an",
			region: "us-east-1",
		},
		{
			name:   "trailing segment after the region",
			bucket: "my-state-111122223333-us-east-1-extra-an",
			region: "us-east-1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := s3backend.BucketNamespace(tc.bucket, tc.region)

			var invalidName s3backend.InvalidAccountRegionalBucketName
			require.ErrorAs(t, err, &invalidName)
			assert.Equal(t, tc.bucket, invalidName.Bucket)
			assert.Equal(t, tc.region, invalidName.Region)
		})
	}
}
