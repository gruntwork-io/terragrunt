//go:build aws

package remote_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/gruntwork-io/terragrunt/awshelper"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAwsCreateS3LoggingInput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		config        map[string]interface{}
		loggingInput  s3.PutBucketLoggingInput
		shouldBeEqual bool
	}{
		{
			"equal-default-prefix-no-partition-date-source",
			map[string]interface{}{
				"bucket":                    "source-bucket",
				"accesslogging_bucket_name": "logging-bucket",
			},
			s3.PutBucketLoggingInput{
				Bucket: aws.String("source-bucket"),
				BucketLoggingStatus: &s3.BucketLoggingStatus{
					LoggingEnabled: &s3.LoggingEnabled{
						TargetBucket: aws.String("logging-bucket"),
						TargetPrefix: aws.String(remote.DefaultS3BucketAccessLoggingTargetPrefix),
					},
				},
			},
			true,
		},
		{
			"equal-no-prefix-no-partition-date-source",
			map[string]interface{}{
				"bucket":                      "source-bucket",
				"accesslogging_bucket_name":   "logging-bucket",
				"accesslogging_target_prefix": "",
			},
			s3.PutBucketLoggingInput{
				Bucket: aws.String("source-bucket"),
				BucketLoggingStatus: &s3.BucketLoggingStatus{
					LoggingEnabled: &s3.LoggingEnabled{
						TargetBucket: aws.String("logging-bucket"),
					},
				},
			},
			true,
		},
		{
			"equal-custom-prefix-no-partition-date-source",
			map[string]interface{}{
				"bucket":                      "source-bucket",
				"accesslogging_bucket_name":   "logging-bucket",
				"accesslogging_target_prefix": "custom-prefix/",
			},
			s3.PutBucketLoggingInput{
				Bucket: aws.String("source-bucket"),
				BucketLoggingStatus: &s3.BucketLoggingStatus{
					LoggingEnabled: &s3.LoggingEnabled{
						TargetBucket: aws.String("logging-bucket"),
						TargetPrefix: aws.String("custom-prefix/"),
					},
				},
			},
			true,
		},
		{
			"equal-custom-prefix-custom-partition-date-source",
			map[string]interface{}{
				"bucket":                    "source-bucket",
				"accesslogging_bucket_name": "logging-bucket",
				"accesslogging_target_object_partition_date_source": "EventTime",
				"accesslogging_target_prefix":                       "custom-prefix/",
			},
			s3.PutBucketLoggingInput{
				Bucket: aws.String("source-bucket"),
				BucketLoggingStatus: &s3.BucketLoggingStatus{
					LoggingEnabled: &s3.LoggingEnabled{
						TargetBucket: aws.String("logging-bucket"),
						TargetPrefix: aws.String("custom-prefix/"),
						TargetObjectKeyFormat: &s3.TargetObjectKeyFormat{
							PartitionedPrefix: &s3.PartitionedPrefix{
								PartitionDateSource: aws.String("EventTime"),
							},
						},
					},
				},
			},
			true,
		},
	}

	for _, testCase := range testCases {
		// Save the testCase in local scope so all the t.Run calls don't end up with the last item in the list
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			extendedS3Config, _ := remote.ParseExtendedS3Config(testCase.config)
			createdLoggingInput := extendedS3Config.CreateS3LoggingInput()
			actual := reflect.DeepEqual(createdLoggingInput, testCase.loggingInput)
			if !assert.Equal(t, testCase.shouldBeEqual, actual) {
				t.Errorf("s3.PutBucketLoggingInput mismatch:\ncreated: %+v\nexpected: %+v", createdLoggingInput, testCase.loggingInput)
			}
		})
	}
}

func TestAwsConfigValuesEqual(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("remote_state_test")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	tc := []struct {
		name          string
		config        map[string]interface{}
		backend       *remote.TerraformBackend
		shouldBeEqual bool
	}{
		{
			"equal-both-empty",
			map[string]interface{}{},
			&remote.TerraformBackend{Type: "s3", Config: map[string]interface{}{}},
			true,
		},
		{
			"equal-empty-and-nil",
			map[string]interface{}{},
			nil,
			true,
		},
		{
			"equal-empty-and-nil-backend-config",
			map[string]interface{}{},
			&remote.TerraformBackend{Type: "s3"},
			true,
		},
		{
			"equal-one-key",
			map[string]interface{}{"foo": "bar"},
			&remote.TerraformBackend{Type: "s3", Config: map[string]interface{}{"foo": "bar"}},
			true,
		},
		{
			"equal-multiple-keys",
			map[string]interface{}{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true},
			&remote.TerraformBackend{Type: "s3", Config: map[string]interface{}{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true}},
			true,
		},
		{
			"equal-encrypt-bool-handling",
			map[string]interface{}{"encrypt": true},
			&remote.TerraformBackend{Type: "s3", Config: map[string]interface{}{"encrypt": "true"}},
			true,
		},
		{
			"equal-general-bool-handling",
			map[string]interface{}{"something": true, "encrypt": true},
			&remote.TerraformBackend{Type: "s3", Config: map[string]interface{}{"something": "true", "encrypt": "true"}},
			true,
		},
		{
			"equal-ignore-s3-tags",
			map[string]interface{}{"foo": "bar", "s3_bucket_tags": []map[string]string{{"foo": "bar"}}},
			&remote.TerraformBackend{Type: "s3", Config: map[string]interface{}{"foo": "bar"}},
			true,
		},
		{
			"equal-ignore-dynamodb-tags",
			map[string]interface{}{"dynamodb_table_tags": []map[string]string{{"foo": "bar"}}},
			&remote.TerraformBackend{Type: "s3", Config: map[string]interface{}{}},
			true,
		},
		{
			"equal-ignore-accesslogging-options",
			map[string]interface{}{
				"accesslogging_bucket_tags":                         []map[string]string{{"foo": "bar"}},
				"accesslogging_target_object_partition_date_source": "EventTime",
				"accesslogging_target_prefix":                       "test/",
				"skip_accesslogging_bucket_acl":                     false,
				"skip_accesslogging_bucket_enforced_tls":            false,
				"skip_accesslogging_bucket_public_access_blocking":  false,
				"skip_accesslogging_bucket_ssencryption":            false,
			},
			&remote.TerraformBackend{Type: "s3", Config: map[string]interface{}{}},
			true,
		},
		{
			"unequal-wrong-backend",
			map[string]interface{}{"foo": "bar"},
			&remote.TerraformBackend{Type: "wrong", Config: map[string]interface{}{"foo": "bar"}},
			false,
		},
		{
			"unequal-values",
			map[string]interface{}{"foo": "bar"},
			&remote.TerraformBackend{Type: "s3", Config: map[string]interface{}{"foo": "different"}},
			false,
		},
		{
			"unequal-non-empty-config-nil",
			map[string]interface{}{"foo": "bar"},
			nil,
			false,
		},
		{
			"unequal-general-bool-handling",
			map[string]interface{}{"something": true},
			&remote.TerraformBackend{Type: "s3", Config: map[string]interface{}{"something": "false"}},
			false,
		},
		{
			"equal-null-ignored",
			map[string]interface{}{"something": "foo"},
			&remote.TerraformBackend{Type: "s3", Config: map[string]interface{}{"something": "foo", "ignored-because-null": nil}},
			true,
		},
	}

	for _, tt := range tc {
		// Save the tt in local scope so all the t.Run calls don't end up with the last item in the list
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actual := remote.ConfigValuesEqual(tt.config, tt.backend, terragruntOptions)
			assert.Equal(t, tt.shouldBeEqual, actual)
		})
	}
}

func TestAwsForcePathStyleClientSession(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("s3_client_test")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	testCases := []struct {
		name     string
		config   map[string]interface{}
		expected bool
	}{
		{
			"path-style-true",
			map[string]interface{}{"force_path_style": true},
			true,
		},
		{
			"path-style-false",
			map[string]interface{}{"force_path_style": false},
			false,
		},
		{
			"path-style-non-existent",
			map[string]interface{}{},
			false,
		},
	}

	for _, testCase := range testCases {
		// The following is necessary to make sure testCase's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			s3ConfigExtended, err := remote.ParseExtendedS3Config(testCase.config)
			require.NoError(t, err, "Unexpected error parsing config for test: %v", err)

			s3Client, err := remote.CreateS3Client(s3ConfigExtended.GetAwsSessionConfig(), terragruntOptions)
			require.NoError(t, err, "Unexpected error creating client for test: %v", err)

			actual := aws.BoolValue(s3Client.Config.S3ForcePathStyle)
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

func TestAwsCustomStateEndpoints(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		config   map[string]interface{}
		expected *awshelper.AwsSessionConfig
	}{
		{
			name:   "using pre 1.6.x settings only",
			config: map[string]interface{}{"endpoint": "foo", "dynamodb_endpoint": "bar"},
			expected: &awshelper.AwsSessionConfig{
				CustomS3Endpoint:       "foo",
				CustomDynamoDBEndpoint: "bar",
			},
		},
		{
			name: "using 1.6+ settings",
			config: map[string]interface{}{
				"endpoint": "foo", "dynamodb_endpoint": "bar",
				"endpoints": map[string]interface{}{
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

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			s3ConfigExtended, err := remote.ParseExtendedS3Config(testCase.config)
			require.NoError(t, err, "Unexpected error parsing config for test: %v", err)

			actual := s3ConfigExtended.GetAwsSessionConfig()

			assert.Equal(t, testCase.expected, actual)
		})
	}
}

func TestAwsGetAwsSessionConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		config map[string]interface{}
	}{
		{
			"all-values",
			map[string]interface{}{"region": "foo", "endpoint": "bar", "profile": "baz", "role_arn": "arn::it", "shared_credentials_file": "my-file", "force_path_style": true},
		},
		{
			"no-values",
			map[string]interface{}{},
		},
		{
			"extra-values",
			map[string]interface{}{"something": "unexpected", "region": "foo", "endpoint": "bar", "dynamodb_endpoint": "foobar", "profile": "baz", "role_arn": "arn::it", "shared_credentials_file": "my-file", "force_path_style": false},
		},
	}

	for _, testCase := range testCases {
		// The following is necessary to make sure testCase's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			s3ConfigExtended, err := remote.ParseExtendedS3Config(testCase.config)
			require.NoError(t, err, "Unexpected error parsing config for test: %v", err)

			expected := &awshelper.AwsSessionConfig{
				Region:                  s3ConfigExtended.RemoteStateConfigS3.Region,
				CustomS3Endpoint:        s3ConfigExtended.RemoteStateConfigS3.Endpoint,
				CustomDynamoDBEndpoint:  s3ConfigExtended.RemoteStateConfigS3.DynamoDBEndpoint,
				Profile:                 s3ConfigExtended.RemoteStateConfigS3.Profile,
				RoleArn:                 s3ConfigExtended.RemoteStateConfigS3.RoleArn,
				CredsFilename:           s3ConfigExtended.RemoteStateConfigS3.CredsFilename,
				S3ForcePathStyle:        s3ConfigExtended.RemoteStateConfigS3.S3ForcePathStyle,
				DisableComputeChecksums: s3ConfigExtended.DisableAWSClientChecksums,
			}

			actual := s3ConfigExtended.GetAwsSessionConfig()
			assert.Equal(t, expected, actual)
		})
	}
}

func TestAwsGetAwsSessionConfigWithAssumeRole(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		config map[string]interface{}
	}{
		{
			"all-values",
			map[string]interface{}{"role_arn": "arn::it", "external_id": "123", "session_name": "foobar"},
		},
	}

	for _, testCase := range testCases {
		// The following is necessary to make sure testCase's values don't
		// get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			config := map[string]interface{}{"assume_role": testCase.config}
			s3ConfigExtended, err := remote.ParseExtendedS3Config(config)
			require.NoError(t, err, "Unexpected error parsing config for test: %v", err)

			expected := &awshelper.AwsSessionConfig{
				RoleArn:     s3ConfigExtended.RemoteStateConfigS3.AssumeRole.RoleArn,
				ExternalID:  s3ConfigExtended.RemoteStateConfigS3.AssumeRole.ExternalID,
				SessionName: s3ConfigExtended.RemoteStateConfigS3.AssumeRole.SessionName,
			}

			actual := s3ConfigExtended.GetAwsSessionConfig()
			assert.Equal(t, expected, actual)
		})
	}
}

func TestAwsGetTerraformInitArgs(t *testing.T) {
	t.Parallel()

	initializer := remote.S3Initializer{}

	testCases := []struct {
		name          string
		config        map[string]interface{}
		expected      map[string]interface{}
		shouldBeEqual bool
	}{
		{
			"empty-no-values",
			map[string]interface{}{},
			map[string]interface{}{},
			true,
		},
		{
			"valid-s3-configuration-keys",
			map[string]interface{}{
				"bucket":  "foo",
				"encrypt": "bar",
				"key":     "baz",
				"region":  "quux",
			},
			map[string]interface{}{
				"bucket":  "foo",
				"encrypt": "bar",
				"key":     "baz",
				"region":  "quux",
			},
			true,
		},
		{
			"terragrunt-keys-filtered",
			map[string]interface{}{
				"bucket":                      "foo",
				"encrypt":                     "bar",
				"key":                         "baz",
				"region":                      "quux",
				"skip_credentials_validation": true,
				"s3_bucket_tags":              map[string]string{},
			},
			map[string]interface{}{
				"bucket":                      "foo",
				"encrypt":                     "bar",
				"key":                         "baz",
				"region":                      "quux",
				"skip_credentials_validation": true,
			},
			true,
		},
		{
			"empty-no-values-all-terragrunt-keys-filtered",
			map[string]interface{}{
				"s3_bucket_tags":                                    map[string]string{},
				"dynamodb_table_tags":                               map[string]string{},
				"accesslogging_bucket_tags":                         map[string]string{},
				"skip_bucket_versioning":                            true,
				"skip_bucket_ssencryption":                          false,
				"skip_bucket_root_access":                           false,
				"skip_bucket_enforced_tls":                          false,
				"skip_bucket_public_access_blocking":                false,
				"disable_bucket_update":                             true,
				"enable_lock_table_ssencryption":                    true,
				"disable_aws_client_checksums":                      false,
				"accesslogging_bucket_name":                         "test",
				"accesslogging_target_object_partition_date_source": "EventTime",
				"accesslogging_target_prefix":                       "test",
				"skip_accesslogging_bucket_acl":                     false,
				"skip_accesslogging_bucket_enforced_tls":            false,
				"skip_accesslogging_bucket_public_access_blocking":  false,
				"skip_accesslogging_bucket_ssencryption":            false,
			},
			map[string]interface{}{},
			true,
		},
		{
			"lock-table-replaced-with-dynamodb-table",
			map[string]interface{}{
				"bucket":     "foo",
				"encrypt":    "bar",
				"key":        "baz",
				"region":     "quux",
				"lock_table": "xyzzy",
			},
			map[string]interface{}{
				"bucket":         "foo",
				"encrypt":        "bar",
				"key":            "baz",
				"region":         "quux",
				"dynamodb_table": "xyzzy",
			},
			true,
		},
		{
			"dynamodb-table-not-replaced-with-lock-table",
			map[string]interface{}{
				"bucket":         "foo",
				"encrypt":        "bar",
				"key":            "baz",
				"region":         "quux",
				"dynamodb_table": "xyzzy",
			},
			map[string]interface{}{
				"bucket":     "foo",
				"encrypt":    "bar",
				"key":        "baz",
				"region":     "quux",
				"lock_table": "xyzzy",
			},
			false,
		},
		{
			"assume-role",
			map[string]interface{}{
				"bucket": "foo",
				"assume_role": map[string]interface{}{
					"role_arn":     "arn:aws:iam::123:role/role",
					"external_id":  "123",
					"session_name": "qwe",
				},
			},
			map[string]interface{}{
				"bucket":      "foo",
				"assume_role": "{external_id=\"123\",role_arn=\"arn:aws:iam::123:role/role\",session_name=\"qwe\"}",
			},
			true,
		},
	}

	for _, testCase := range testCases {
		// Save the testCase in local scope so all the t.Run calls don't end up with the last item in the list
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			actual := initializer.GetTerraformInitArgs(testCase.config)

			if !testCase.shouldBeEqual {
				assert.NotEqual(t, testCase.expected, actual)
				return
			}
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

// Test to validate cases when is not possible to read all S3 configurations
// https://github.com/gruntwork-io/terragrunt/issues/2109
func TestAwsNegativePublicAccessResponse(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		response *s3.GetPublicAccessBlockOutput
	}{
		{
			name: "nil-response",
			response: &s3.GetPublicAccessBlockOutput{
				PublicAccessBlockConfiguration: nil,
			},
		},
		{
			name: "legacy-bucket",
			response: &s3.GetPublicAccessBlockOutput{
				PublicAccessBlockConfiguration: &s3.PublicAccessBlockConfiguration{
					BlockPublicAcls:       nil,
					BlockPublicPolicy:     nil,
					IgnorePublicAcls:      nil,
					RestrictPublicBuckets: nil,
				},
			},
		},
		{
			name: "false-response",
			response: &s3.GetPublicAccessBlockOutput{
				PublicAccessBlockConfiguration: &s3.PublicAccessBlockConfiguration{
					BlockPublicAcls:       aws.Bool(false),
					BlockPublicPolicy:     aws.Bool(false),
					IgnorePublicAcls:      aws.Bool(false),
					RestrictPublicBuckets: aws.Bool(false),
				},
			},
		},
	}
	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			response, err := remote.ValidatePublicAccessBlock(testCase.response)
			require.NoError(t, err)
			assert.False(t, response)
		})
	}
}

func TestAwsValidateS3Config(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name           string
		extendedConfig *remote.ExtendedRemoteStateConfigS3
		expectedErr    error
		expectedOutput string
	}{
		{
			name:           "no-region",
			extendedConfig: &remote.ExtendedRemoteStateConfigS3{},
			expectedErr:    remote.MissingRequiredS3RemoteStateConfig("region"),
		},
		{
			name: "no-bucket",
			extendedConfig: &remote.ExtendedRemoteStateConfigS3{
				RemoteStateConfigS3: remote.RemoteStateConfigS3{
					Region: "us-west-2",
				},
			},
			expectedErr: remote.MissingRequiredS3RemoteStateConfig("bucket"),
		},
		{
			name: "no-key",
			extendedConfig: &remote.ExtendedRemoteStateConfigS3{
				RemoteStateConfigS3: remote.RemoteStateConfigS3{
					Region: "us-west-2",
					Bucket: "state-bucket",
				},
			},
			expectedErr: remote.MissingRequiredS3RemoteStateConfig("key"),
		},
	}
	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			buf := &bytes.Buffer{}
			logger := logrus.New()
			logger.SetLevel(logrus.DebugLevel)
			logger.SetOutput(buf)
			err := remote.ValidateS3Config(testCase.extendedConfig)
			if err != nil {
				require.ErrorIs(t, err, testCase.expectedErr)
			}
			assert.Contains(t, buf.String(), testCase.expectedOutput)
		})
	}
}
