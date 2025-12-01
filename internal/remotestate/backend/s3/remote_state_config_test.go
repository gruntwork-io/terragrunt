package s3_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
)

func TestConfig_CreateS3LoggingInput(t *testing.T) {
	t.Parallel()

	testCases := []struct { //nolint: govet
		name          string
		config        s3backend.Config
		loggingInput  s3.PutBucketLoggingInput
		shouldBeEqual bool
	}{
		{
			"equal-default-prefix-no-partition-date-source",
			s3backend.Config{
				"bucket":                    "source-bucket",
				"accesslogging_bucket_name": "logging-bucket",
			},
			s3.PutBucketLoggingInput{
				Bucket: aws.String("source-bucket"),
				BucketLoggingStatus: &s3types.BucketLoggingStatus{
					LoggingEnabled: &s3types.LoggingEnabled{
						TargetBucket: aws.String("logging-bucket"),
						TargetPrefix: aws.String(s3backend.DefaultS3BucketAccessLoggingTargetPrefix),
					},
				},
			},
			true,
		},
		{
			"equal-no-prefix-no-partition-date-source",
			s3backend.Config{
				"bucket":                      "source-bucket",
				"accesslogging_bucket_name":   "logging-bucket",
				"accesslogging_target_prefix": "",
			},
			s3.PutBucketLoggingInput{
				Bucket: aws.String("source-bucket"),
				BucketLoggingStatus: &s3types.BucketLoggingStatus{
					LoggingEnabled: &s3types.LoggingEnabled{
						TargetBucket: aws.String("logging-bucket"),
					},
				},
			},
			true,
		},
		{
			"equal-custom-prefix-no-partition-date-source",
			s3backend.Config{
				"bucket":                      "source-bucket",
				"accesslogging_bucket_name":   "logging-bucket",
				"accesslogging_target_prefix": "custom-prefix/",
			},
			s3.PutBucketLoggingInput{
				Bucket: aws.String("source-bucket"),
				BucketLoggingStatus: &s3types.BucketLoggingStatus{
					LoggingEnabled: &s3types.LoggingEnabled{
						TargetBucket: aws.String("logging-bucket"),
						TargetPrefix: aws.String("custom-prefix/"),
					},
				},
			},
			true,
		},
		{
			"equal-custom-prefix-custom-partition-date-source",
			s3backend.Config{
				"bucket":                    "source-bucket",
				"accesslogging_bucket_name": "logging-bucket",
				"accesslogging_target_object_partition_date_source": "EventTime",
				"accesslogging_target_prefix":                       "custom-prefix/",
			},
			s3.PutBucketLoggingInput{
				Bucket: aws.String("source-bucket"),
				BucketLoggingStatus: &s3types.BucketLoggingStatus{
					LoggingEnabled: &s3types.LoggingEnabled{
						TargetBucket: aws.String("logging-bucket"),
						TargetPrefix: aws.String("custom-prefix/"),
						TargetObjectKeyFormat: &s3types.TargetObjectKeyFormat{
							PartitionedPrefix: &s3types.PartitionedPrefix{
								PartitionDateSource: s3types.PartitionDateSource("EventTime"),
							},
						},
					},
				},
			},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			extS3Cfg, err := tc.config.Normalize(log.Default()).ParseExtendedS3Config()
			require.NoError(t, err, "Unexpected error parsing config for test: %v", err)

			createdLoggingInput := extS3Cfg.CreateS3LoggingInput()

			actual := reflect.DeepEqual(createdLoggingInput, tc.loggingInput)
			if !assert.Equal(t, tc.shouldBeEqual, actual) {
				t.Errorf("s3.PutBucketLoggingInput mismatch:\ncreated: %+v\nexpected: %+v", createdLoggingInput, tc.loggingInput)
			}
		})
	}
}

func TestConfig_ForcePathStyleClientSession(t *testing.T) {
	t.Parallel()

	testCases := []struct { //nolint: govet
		name     string
		config   s3backend.Config
		expected bool
	}{
		{
			"path-style-true",
			s3backend.Config{"force_path_style": true},
			true,
		},
		{
			"path-style-false",
			s3backend.Config{"force_path_style": false},
			false,
		},
		{
			"path-style-non-existent",
			s3backend.Config{},
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			extS3Cfg, err := tc.config.Normalize(log.Default()).ParseExtendedS3Config()
			require.NoError(t, err, "Unexpected error parsing config for test: %v", err)

			awsSessionConfig := extS3Cfg.GetAwsSessionConfig()

			actual := awsSessionConfig.S3ForcePathStyle
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestConfig_CustomStateEndpoints(t *testing.T) {
	t.Parallel()

	testCases := []struct { //nolint: govet
		name     string
		config   s3backend.Config
		expected *awshelper.AwsSessionConfig
	}{
		{
			name:   "using pre 1.6.x settings only",
			config: s3backend.Config{"endpoint": "foo", "dynamodb_endpoint": "bar"},
			expected: &awshelper.AwsSessionConfig{
				CustomS3Endpoint:       "foo",
				CustomDynamoDBEndpoint: "bar",
			},
		},
		{
			name: "using 1.6+ settings",
			config: s3backend.Config{
				"endpoint": "foo", "dynamodb_endpoint": "bar",
				"endpoints": s3backend.Config{
					"s3":       "fooBar",
					"dynamodb": "barFoo",
				},
			},
			expected: &awshelper.AwsSessionConfig{
				CustomS3Endpoint:       "fooBar",
				CustomDynamoDBEndpoint: "barFoo",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			extS3Cfg, err := tc.config.Normalize(log.Default()).ParseExtendedS3Config()
			require.NoError(t, err, "Unexpected error parsing config for test: %v", err)

			actual := extS3Cfg.GetAwsSessionConfig()
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestConfig_GetAwsSessionConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct { //nolint: govet
		name   string
		config s3backend.Config
	}{
		{
			"all-values",
			s3backend.Config{"region": "foo", "endpoint": "bar", "profile": "baz", "role_arn": "arn::it", "shared_credentials_file": "my-file", "force_path_style": true},
		},
		{
			"no-values",
			s3backend.Config{},
		},
		{
			"extra-values",
			s3backend.Config{"something": "unexpected", "region": "foo", "endpoint": "bar", "dynamodb_endpoint": "foobar", "profile": "baz", "role_arn": "arn::it", "shared_credentials_file": "my-file", "force_path_style": false},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			extS3Cfg, err := tc.config.Normalize(log.Default()).ParseExtendedS3Config()
			require.NoError(t, err, "Unexpected error parsing config for test: %v", err)

			expected := &awshelper.AwsSessionConfig{
				Region:                  extS3Cfg.RemoteStateConfigS3.Region,
				CustomS3Endpoint:        extS3Cfg.RemoteStateConfigS3.Endpoint,
				CustomDynamoDBEndpoint:  extS3Cfg.RemoteStateConfigS3.DynamoDBEndpoint,
				Profile:                 extS3Cfg.RemoteStateConfigS3.Profile,
				RoleArn:                 extS3Cfg.RemoteStateConfigS3.RoleArn,
				CredsFilename:           extS3Cfg.RemoteStateConfigS3.CredsFilename,
				S3ForcePathStyle:        extS3Cfg.RemoteStateConfigS3.S3ForcePathStyle,
				DisableComputeChecksums: extS3Cfg.DisableAWSClientChecksums,
			}

			actual := extS3Cfg.GetAwsSessionConfig()
			assert.Equal(t, expected, actual)
		})
	}
}

func TestConfig_GetAwsSessionConfigWithAssumeRole(t *testing.T) {
	t.Parallel()

	testCases := []struct { //nolint: govet
		name   string
		config s3backend.Config
	}{
		{
			"all-values",
			s3backend.Config{"role_arn": "arn::it", "external_id": "123", "session_name": "foobar", "tags": map[string]string{"foo": "bar"}},
		},
		{
			"no-tags",
			s3backend.Config{"role_arn": "arn::it", "external_id": "123", "session_name": "foobar"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config := s3backend.Config{"assume_role": tc.config}

			extS3Cfg, err := config.Normalize(log.Default()).ParseExtendedS3Config()
			require.NoError(t, err, "Unexpected error parsing config for test: %v", err)

			expected := &awshelper.AwsSessionConfig{
				RoleArn:     extS3Cfg.RemoteStateConfigS3.AssumeRole.RoleArn,
				ExternalID:  extS3Cfg.RemoteStateConfigS3.AssumeRole.ExternalID,
				SessionName: extS3Cfg.RemoteStateConfigS3.AssumeRole.SessionName,
				Tags:        extS3Cfg.RemoteStateConfigS3.AssumeRole.Tags,
			}

			actual := extS3Cfg.GetAwsSessionConfig()
			assert.Equal(t, expected, actual)
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		extConfig      *s3backend.ExtendedRemoteStateConfigS3
		expectedErr    error
		expectedOutput string
	}{
		{
			name:        "no-region",
			extConfig:   &s3backend.ExtendedRemoteStateConfigS3{},
			expectedErr: s3backend.MissingRequiredS3RemoteStateConfig("region"),
		},
		{
			name: "no-bucket",
			extConfig: &s3backend.ExtendedRemoteStateConfigS3{
				RemoteStateConfigS3: s3backend.RemoteStateConfigS3{
					Region: "us-west-2",
				},
			},
			expectedErr: s3backend.MissingRequiredS3RemoteStateConfig("bucket"),
		},
		{
			name: "no-key",
			extConfig: &s3backend.ExtendedRemoteStateConfigS3{
				RemoteStateConfigS3: s3backend.RemoteStateConfigS3{
					Region: "us-west-2",
					Bucket: "state-bucket",
				},
			},
			expectedErr: s3backend.MissingRequiredS3RemoteStateConfig("key"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			buf := &bytes.Buffer{}
			logger := logrus.New()
			logger.SetLevel(logrus.DebugLevel)
			logger.SetOutput(buf)

			err := tc.extConfig.Validate()
			require.ErrorIs(t, err, tc.expectedErr)

			assert.Contains(t, buf.String(), tc.expectedOutput)
		})
	}
}
