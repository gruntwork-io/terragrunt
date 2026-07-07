//go:build aws

package awshelper_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAwsSessionValidationFail(t *testing.T) {
	t.Skip("Skipping for now as we need to change the signature of CreateAwsConfig")
	t.Parallel()

	l := logger.CreateLogger()

	_, err := awshelper.NewAWSConfigBuilder().
		WithSessionConfig(&awshelper.AwsSessionConfig{
			Region:        "not-existing-region",
			CredsFilename: "/tmp/not-existing-file",
		}).
		Build(t.Context(), l)
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

func TestAwsConfigWithAuthProviderEnv(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()

	env := map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access-key",
		"AWS_SECRET_ACCESS_KEY": "test-secret-key",
		"AWS_SESSION_TOKEN":     "test-session-token",
		"AWS_REGION":            "us-west-2",
	}

	cfg, err := awshelper.NewAWSConfigBuilder().
		WithEnv(env).
		Build(ctx, l)
	require.NoError(t, err)
	assert.Equal(t, "us-west-2", cfg.Region)

	assert.NotNil(t, cfg.Credentials)
}

func TestAwsConfigWithAuthProviderEnvDefaultRegion(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()

	env := map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access-key",
		"AWS_SECRET_ACCESS_KEY": "test-secret-key",
		"AWS_DEFAULT_REGION":    "eu-west-1",
	}

	cfg, err := awshelper.NewAWSConfigBuilder().
		WithEnv(env).
		Build(ctx, l)
	require.NoError(t, err)
	assert.Equal(t, "eu-west-1", cfg.Region)
	assert.NotNil(t, cfg.Credentials)
}

func TestAwsConfigRegionTakesPrecedenceOverEnvVars(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()

	// Simulate env vars; do not mutate process env in parallel tests
	env := map[string]string{
		"AWS_REGION":            "us-west-1",
		"AWS_DEFAULT_REGION":    "us-west-1",
		"AWS_ACCESS_KEY_ID":     "test-access-key",
		"AWS_SECRET_ACCESS_KEY": "test-secret-key",
	}

	// Create config with explicit region that should take precedence
	awsCfg := &awshelper.AwsSessionConfig{
		Region: "us-east-1", // This should override the env vars
	}

	cfg, err := awshelper.NewAWSConfigBuilder().
		WithSessionConfig(awsCfg).
		WithEnv(env).
		Build(ctx, l)
	require.NoError(t, err)

	// Verify that the config uses the region from awsCfg, not from environment variables
	assert.Equal(t, "us-east-1", cfg.Region)
}

// TestAwsConfigStillAssumesRoleWithEnvCredentials is a regression test for a bug found while
// investigating https://github.com/gruntwork-io/terragrunt/issues/4979: Build() returned as soon
// as it found static AWS credentials in the environment (AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY),
// before ever looking at the configured IAM role options. In production this environment is not
// just an auth-provider-cmd output -- it is also how ordinary IRSA/OIDC web-identity credential
// chains present themselves (a CI/EKS pod assumes its own base role once, exports the resulting
// temporary AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY/AWS_SESSION_TOKEN into the process environment,
// then terragrunt is expected to assume a further role on top of that base identity for a specific
// S3 backend, e.g. a cross-account dependency's remote_state.assume_role). Because Build() bailed
// out the moment it saw env credentials, that further role assumption was silently skipped
// entirely and every AWS call -- including the S3 fast-path dependency fetch added for #4979 --
// used the base identity's own credentials instead of the configured role.
func TestAwsConfigStillAssumesRoleWithEnvCredentials(t *testing.T) {
	// Not parallel: mutates the process-wide AWS_ENDPOINT_URL_STS env var, which the AWS SDK's
	// config.LoadDefaultConfig reads directly from the OS environment (unlike terragrunt's own
	// b.env/WithEnv, which only feeds createCredentialsFromEnv/getRegionFromEnv).

	var gotRoleArn string

	stsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		gotRoleArn = values.Get("RoleArn")

		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<AssumeRoleResponse><AssumeRoleResult><Credentials>` +
			`<AccessKeyId>assumed-access-key</AccessKeyId>` +
			`<SecretAccessKey>assumed-secret-key</SecretAccessKey>` +
			`<SessionToken>assumed-session-token</SessionToken>` +
			`<Expiration>2999-01-01T00:00:00Z</Expiration>` +
			`</Credentials></AssumeRoleResult></AssumeRoleResponse>`))
	}))
	defer stsServer.Close()

	t.Setenv("AWS_ENDPOINT_URL_STS", stsServer.URL)

	l := logger.CreateLogger()
	ctx := context.Background()

	const roleARN = "arn:aws:iam::222222222222:role/dependency-role"

	// Simulate a process whose own identity already comes from temporary credentials (as IRSA/OIDC
	// web-identity chains produce), while a specific AWS call still needs to assume a further role
	// (e.g. a dependency's own remote_state.assume_role.role_arn).
	env := map[string]string{
		"AWS_ACCESS_KEY_ID":     "base-access-key",
		"AWS_SECRET_ACCESS_KEY": "base-secret-key",
		"AWS_SESSION_TOKEN":     "base-session-token",
		"AWS_REGION":            "us-east-1",
	}

	cfg, err := awshelper.NewAWSConfigBuilder().
		WithEnv(env).
		WithIAMRoleOptions(iam.RoleOptions{RoleARN: roleARN}).
		Build(ctx, l)
	require.NoError(t, err)

	creds, err := cfg.Credentials.Retrieve(ctx)
	require.NoError(t, err)

	assert.Equal(t, roleARN, gotRoleArn,
		"Build must still call sts:AssumeRole for the configured role even when static credentials are already present in the environment")
	assert.Equal(t, "assumed-access-key", creds.AccessKeyID,
		"the resulting credentials must be the assumed role's session credentials, not the base env credentials")
}
