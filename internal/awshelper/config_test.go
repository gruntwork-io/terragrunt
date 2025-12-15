//go:build aws

package awshelper_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAwsSessionValidationFail(t *testing.T) {
	t.Skip("Skipping for now as we need to change the signature of CreateAwsConfig")
	t.Parallel()

	l := logger.CreateLogger()

	// With AWS SDK v2, CreateAwsConfig now validates credentials internally
	// so it should fail when invalid credentials are provided
	_, err := awshelper.CreateAwsConfig(t.Context(), l, &awshelper.AwsSessionConfig{
		Region:        "not-existing-region",
		CredsFilename: "/tmp/not-existing-file",
	}, options.NewTerragruntOptions())
	assert.Error(t, err)
}

// Test to validate cases when is not possible to read all S3 configurations
// https://github.com/gruntwork-io/terragrunt/issues/2109
func TestAwsNegativePublicAccessResponse(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		response *s3.GetPublicAccessBlockOutput
		name     string
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
				PublicAccessBlockConfiguration: &s3types.PublicAccessBlockConfiguration{
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
				PublicAccessBlockConfiguration: &s3types.PublicAccessBlockConfiguration{
					BlockPublicAcls:       aws.Bool(false),
					BlockPublicPolicy:     aws.Bool(false),
					IgnorePublicAcls:      aws.Bool(false),
					RestrictPublicBuckets: aws.Bool(false),
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			response, err := awshelper.ValidatePublicAccessBlock(tc.response)
			require.NoError(t, err)
			assert.False(t, response)
		})
	}
}

func TestCreateAwsConfigWithAuthProviderEnv(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()

	opts := &options.TerragruntOptions{
		Env: map[string]string{
			"AWS_ACCESS_KEY_ID":     "test-access-key",
			"AWS_SECRET_ACCESS_KEY": "test-secret-key",
			"AWS_SESSION_TOKEN":     "test-session-token",
			"AWS_REGION":            "us-west-2",
		},
	}

	cfg, err := awshelper.CreateAwsConfig(ctx, l, nil, opts)
	require.NoError(t, err)
	assert.Equal(t, "us-west-2", cfg.Region)

	assert.NotNil(t, cfg.Credentials)
}

func TestCreateAwsConfigWithAuthProviderEnvDefaultRegion(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()

	opts := &options.TerragruntOptions{
		Env: map[string]string{
			"AWS_ACCESS_KEY_ID":     "test-access-key",
			"AWS_SECRET_ACCESS_KEY": "test-secret-key",
			"AWS_DEFAULT_REGION":    "eu-west-1",
		},
	}

	cfg, err := awshelper.CreateAwsConfig(ctx, l, nil, opts)
	require.NoError(t, err)
	assert.Equal(t, "eu-west-1", cfg.Region)
	assert.NotNil(t, cfg.Credentials)
}

func TestAwsConfigRegionTakesPrecedenceOverEnvVars(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()

	// Simulate env vars via opts.Env; do not mutate process env in parallel tests
	opts := &options.TerragruntOptions{
		Env: map[string]string{
			"AWS_REGION":            "us-west-1",
			"AWS_DEFAULT_REGION":    "us-west-1",
			"AWS_ACCESS_KEY_ID":     "test-access-key",
			"AWS_SECRET_ACCESS_KEY": "test-secret-key",
		},
	}

	// Create config with explicit region that should take precedence
	awsCfg := &awshelper.AwsSessionConfig{
		Region: "us-east-1", // This should override the env vars
	}

	cfg, err := awshelper.CreateAwsConfig(ctx, l, awsCfg, opts)
	require.NoError(t, err)

	// Verify that the config uses the region from awsCfg, not from environment variables
	assert.Equal(t, "us-east-1", cfg.Region)
}
