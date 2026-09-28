package s3_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
)

func TestAccessLogDeliveryPolicy(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		partition  string
		accountID  string
		logsBucket string
		expected   string
	}{
		{
			name:       "commercial-partition",
			partition:  "aws",
			accountID:  "123456789012",
			logsBucket: "my-logs-bucket",
			expected: `{
				"Version": "2012-10-17",
				"Statement": [{
					"Sid": "AccessLogDelivery",
					"Effect": "Allow",
					"Principal": {"Service": "logging.s3.amazonaws.com"},
					"Action": "s3:PutObject",
					"Resource": "arn:aws:s3:::my-logs-bucket/*",
					"Condition": {"StringEquals": {"aws:SourceAccount": "123456789012"}}
				}]
			}`,
		},
		{
			name:       "govcloud-partition",
			partition:  "aws-us-gov",
			accountID:  "210987654321",
			logsBucket: "gov-logs-bucket",
			expected: `{
				"Version": "2012-10-17",
				"Statement": [{
					"Sid": "AccessLogDelivery",
					"Effect": "Allow",
					"Principal": {"Service": "logging.s3.amazonaws.com"},
					"Action": "s3:PutObject",
					"Resource": "arn:aws-us-gov:s3:::gov-logs-bucket/*",
					"Condition": {"StringEquals": {"aws:SourceAccount": "210987654321"}}
				}]
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := awshelper.MarshalPolicy(
				s3backend.AccessLogDeliveryPolicy(tc.partition, tc.accountID, tc.logsBucket),
			)
			require.NoError(t, err)
			assert.JSONEq(t, tc.expected, string(got))
		})
	}
}

func TestDecideAccessLogDelivery(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		config   s3backend.ExtendedRemoteStateConfigS3
		origin   s3backend.LogsBucketOrigin
		expected s3backend.AccessLogDeliveryAction
	}{
		{
			name:     "created-bucket-is-granted",
			origin:   s3backend.LogsBucketCreated,
			config:   s3backend.ExtendedRemoteStateConfigS3{},
			expected: s3backend.GrantAccessLogDelivery,
		},
		{
			name:     "preexisting-bucket-is-left-alone",
			origin:   s3backend.LogsBucketPreexisting,
			config:   s3backend.ExtendedRemoteStateConfigS3{},
			expected: s3backend.LeavePreexistingLogsBucketPolicy,
		},
		{
			name:     "skip-policy-opts-out-of-a-created-bucket",
			origin:   s3backend.LogsBucketCreated,
			config:   s3backend.ExtendedRemoteStateConfigS3{SkipAccessLoggingBucketPolicy: true},
			expected: s3backend.SkipAccessLogDeliveryByConfig,
		},
		{
			name:     "preexisting-bucket-wins-over-skip-policy",
			origin:   s3backend.LogsBucketPreexisting,
			config:   s3backend.ExtendedRemoteStateConfigS3{SkipAccessLoggingBucketPolicy: true},
			expected: s3backend.LeavePreexistingLogsBucketPolicy,
		},
		{
			// The deprecated attribute opted out of a bucket ACL. Honoring it here would deny the
			// policy to the configs that set it precisely because their bucket rejected the ACL.
			name:     "deprecated-acl-attribute-does-not-opt-out",
			origin:   s3backend.LogsBucketCreated,
			config:   s3backend.ExtendedRemoteStateConfigS3{SkipAccessLoggingBucketACL: true},
			expected: s3backend.GrantAccessLogDelivery,
		},
		{
			name:   "deprecated-attribute-does-not-cancel-the-current-one",
			origin: s3backend.LogsBucketCreated,
			config: s3backend.ExtendedRemoteStateConfigS3{
				SkipAccessLoggingBucketACL:    true,
				SkipAccessLoggingBucketPolicy: true,
			},
			expected: s3backend.SkipAccessLogDeliveryByConfig,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, s3backend.DecideAccessLogDelivery(tc.origin, &tc.config))
		})
	}
}
