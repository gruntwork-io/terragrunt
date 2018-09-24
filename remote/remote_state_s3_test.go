package remote

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestConfigValuesEqual(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("remote_state_test")
	require.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	testCases := []struct {
		name          string
		config        map[string]interface{}
		backend       *TerraformBackend
		shouldBeEqual bool
	}{
		{
			"equal-both-empty",
			map[string]interface{}{},
			&TerraformBackend{Type: "s3", Config: map[string]interface{}{}},
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
			&TerraformBackend{Type: "s3"},
			true,
		},
		{
			"equal-one-key",
			map[string]interface{}{"foo": "bar"},
			&TerraformBackend{Type: "s3", Config: map[string]interface{}{"foo": "bar"}},
			true,
		},
		{
			"equal-multiple-keys",
			map[string]interface{}{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true},
			&TerraformBackend{Type: "s3", Config: map[string]interface{}{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true}},
			true,
		},
		{
			"equal-encrypt-bool-handling",
			map[string]interface{}{"encrypt": true},
			&TerraformBackend{Type: "s3", Config: map[string]interface{}{"encrypt": "true"}},
			true,
		},
		{
			"equal-general-bool-handling",
			map[string]interface{}{"something": true, "encrypt": true},
			&TerraformBackend{Type: "s3", Config: map[string]interface{}{"something": "true", "encrypt": "true"}},
			true,
		},
		{
			"equal-ignore-s3-tags",
			map[string]interface{}{"foo": "bar", "s3_bucket_tags": []map[string]string{{"foo": "bar"}}},
			&TerraformBackend{Type: "s3", Config: map[string]interface{}{"foo": "bar"}},
			true,
		},
		{
			"equal-ignore-dynamodb-tags",
			map[string]interface{}{"dynamodb_table_tags": []map[string]string{{"foo": "bar"}}},
			&TerraformBackend{Type: "s3", Config: map[string]interface{}{}},
			true,
		},
		{
			"unequal-wrong-backend",
			map[string]interface{}{"foo": "bar"},
			&TerraformBackend{Type: "wrong", Config: map[string]interface{}{"foo": "bar"}},
			false,
		},
		{
			"unequal-values",
			map[string]interface{}{"foo": "bar"},
			&TerraformBackend{Type: "s3", Config: map[string]interface{}{"foo": "different"}},
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
			&TerraformBackend{Type: "s3", Config: map[string]interface{}{"something": "false"}},
			false,
		},
	}

	for _, testCase := range testCases {
		// Save the testCase in local scope so all the t.Run calls don't end up with the last item in the list
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			actual := configValuesEqual(testCase.config, testCase.backend, terragruntOptions)
			assert.Equal(t, testCase.shouldBeEqual, actual)
		})
	}
}

func TestForcePathStyleClientSession(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("s3_client_test")
	require.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

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
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			s3Config, err := parseS3Config(testCase.config)
			require.Nil(t, err, "Unexpected error parsing config for test: %v", err)

			s3Client, err := CreateS3Client(s3Config.GetAwsSessionConfig(), terragruntOptions)
			require.Nil(t, err, "Unexpected error creating client for test: %v", err)

			actual := aws.BoolValue(s3Client.Config.S3ForcePathStyle)
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

func TestGetAwsSessionConfig(t *testing.T) {
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
			map[string]interface{}{"something": "unexpected", "region": "foo", "endpoint": "bar", "profile": "baz", "role_arn": "arn::it", "shared_credentials_file": "my-file", "force_path_style": false},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			s3Config, err := parseS3Config(testCase.config)
			require.Nil(t, err, "Unexpected error parsing config for test: %v", err)

			expected := &aws_helper.AwsSessionConfig{
				Region:           s3Config.Region,
				CustomS3Endpoint: s3Config.Endpoint,
				Profile:          s3Config.Profile,
				RoleArn:          s3Config.RoleArn,
				CredsFilename:    s3Config.CredsFilename,
				S3ForcePathStyle: s3Config.S3ForcePathStyle,
			}

			actual := s3Config.GetAwsSessionConfig()
			assert.Equal(t, expected, actual)
		})
	}
}
