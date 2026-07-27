//go:build aws

package awshelper_test

import (
	"context"
	"os"
	"strings"
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

	require.NotNil(t, cfg.Credentials)

	// With no role configured, the env credentials must be used verbatim.
	creds, err := cfg.Credentials.Retrieve(ctx)
	require.NoError(t, err)
	assert.Equal(t, "test-access-key", creds.AccessKeyID)
	assert.Equal(t, "test-secret-key", creds.SecretAccessKey)
	assert.Equal(t, "test-session-token", creds.SessionToken)
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

// TestAwsConfigWithAuthProviderEnvChainsAssumeRole verifies that credentials provided via
// env (e.g. from --auth-provider-cmd) do not short-circuit role assumption: when a role ARN is
// configured (e.g. via the assume_role attribute of the remote_state block), the resulting
// identity must be the assumed role, with the env credentials serving only as the source
// identity for the STS exchange.
func TestAwsConfigWithAuthProviderEnvChainsAssumeRole(t *testing.T) {
	t.Parallel()

	roleARN := os.Getenv("AWS_TEST_S3_ASSUME_ROLE")
	require.NotEmpty(t, roleARN, "AWS_TEST_S3_ASSUME_ROLE environment variable not set")

	// Pass the real test credentials through the builder env, simulating credentials handed
	// over by an auth provider command.
	env := map[string]string{
		"AWS_ACCESS_KEY_ID":     os.Getenv("AWS_ACCESS_KEY_ID"),
		"AWS_SECRET_ACCESS_KEY": os.Getenv("AWS_SECRET_ACCESS_KEY"),
		"AWS_SESSION_TOKEN":     os.Getenv("AWS_SESSION_TOKEN"),
		"AWS_REGION":            "us-west-2",
	}
	require.NotEmpty(
		t,
		env["AWS_ACCESS_KEY_ID"],
		"static AWS credentials are required to act as the source identity",
	)
	require.NotEmpty(
		t,
		env["AWS_SECRET_ACCESS_KEY"],
		"static AWS credentials are required to act as the source identity",
	)

	l := logger.CreateLogger()

	baseCfg, err := awshelper.NewAWSConfigBuilder().
		WithEnv(env).
		Build(t.Context(), l)
	require.NoError(t, err)

	baseARN, err := awshelper.GetAWSIdentityArn(t.Context(), &baseCfg)
	require.NoError(t, err)

	const sessionName = "terragrunt-chained-assume-role-test"

	chainedCfg, err := awshelper.NewAWSConfigBuilder().
		WithEnv(env).
		WithSessionConfig(&awshelper.AwsSessionConfig{
			RoleArn:     roleARN,
			SessionName: sessionName,
		}).
		Build(t.Context(), l)
	require.NoError(t, err)

	chainedARN, err := awshelper.GetAWSIdentityArn(t.Context(), &chainedCfg)
	require.NoError(t, err)

	// The chained identity must be the assumed role, not the source credentials reused as-is.
	roleName := roleARN[strings.LastIndex(roleARN, "/")+1:]

	assert.NotEqual(t, baseARN, chainedARN)
	assert.Contains(t, chainedARN, ":assumed-role/"+roleName+"/"+sessionName)
}

// credsExpectation names the credential resolution outcome a Build permutation must produce.
type credsExpectation int

const (
	// wantEnvCredsVerbatim: resolution returns the env credentials as-is, with no role assumption.
	wantEnvCredsVerbatim credsExpectation = iota
	// wantAssumeAttempted: resolution performs an STS role assumption signed with the env credentials.
	wantAssumeAttempted
	// wantRoleProvider: a role-assuming provider is installed on top of the default credential chain.
	wantRoleProvider
	// wantDefaultChain: the default credential chain is left untouched.
	wantDefaultChain
)

// TestAwsConfigRoleSourcePermutations covers every combination of ambient env credentials, merged
// IAM role options (the iam_role attribute and the --iam-assume-role flag both arrive here), and
// a backend role (the assume_role attribute of the remote_state block).
//
// When env credentials are present, IAM role options must be ignored: the amazonsts credentials
// provider already applied them, so the env credentials are that role's session and re-assuming
// would make the role assume itself. A backend role is never pre-applied, so it must always be
// assumed, with present env credentials serving as the source identity for the exchange.
func TestAwsConfigRoleSourcePermutations(t *testing.T) {
	t.Parallel()

	envCreds := map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-access-key",
		"AWS_SECRET_ACCESS_KEY": "test-secret-key",
		"AWS_SESSION_TOKEN":     "test-session-token",
		"AWS_REGION":            "us-west-2",
	}
	iamRoleOpts := iam.RoleOptions{
		RoleARN: "arn:aws:iam::111111111111:role/deploy-role",
	}
	backendRole := &awshelper.AwsSessionConfig{
		RoleArn: "arn:aws:iam::111111111111:role/backend-role",
	}

	testCases := []struct {
		name          string
		env           map[string]string
		sessionConfig *awshelper.AwsSessionConfig
		iamRoleOpts   iam.RoleOptions
		want          credsExpectation
	}{
		{
			name: "no-creds-no-roles",
			env:  map[string]string{},
			want: wantDefaultChain,
		},
		{
			name:        "iam-role-only",
			env:         map[string]string{},
			iamRoleOpts: iamRoleOpts,
			want:        wantRoleProvider,
		},
		{
			name:          "backend-role-only",
			env:           map[string]string{},
			sessionConfig: backendRole,
			want:          wantRoleProvider,
		},
		{
			name:          "iam-role-and-backend-role",
			env:           map[string]string{},
			iamRoleOpts:   iamRoleOpts,
			sessionConfig: backendRole,
			want:          wantRoleProvider,
		},
		{
			name: "env-creds-only",
			env:  envCreds,
			want: wantEnvCredsVerbatim,
		},
		{
			name:        "env-creds-and-iam-role",
			env:         envCreds,
			iamRoleOpts: iamRoleOpts,
			want:        wantEnvCredsVerbatim,
		},
		{
			name:          "env-creds-and-backend-role",
			env:           envCreds,
			sessionConfig: backendRole,
			want:          wantAssumeAttempted,
		},
		{
			name:          "env-creds-iam-role-and-backend-role",
			env:           envCreds,
			iamRoleOpts:   iamRoleOpts,
			sessionConfig: backendRole,
			want:          wantAssumeAttempted,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()

			cfg, err := awshelper.NewAWSConfigBuilder().
				WithEnv(tc.env).
				WithSessionConfig(tc.sessionConfig).
				WithIAMRoleOptions(tc.iamRoleOpts).
				Build(t.Context(), l)
			require.NoError(t, err)
			require.NotNil(t, cfg.Credentials)

			switch tc.want {
			case wantEnvCredsVerbatim:
				creds, err := cfg.Credentials.Retrieve(t.Context())
				require.NoError(t, err)
				assert.Equal(t, "test-access-key", creds.AccessKeyID)
				assert.Equal(t, "test-secret-key", creds.SecretAccessKey)
				assert.Equal(t, "test-session-token", creds.SessionToken)
			case wantAssumeAttempted:
				// A canceled context makes the STS exchange fail before any network I/O: an error
				// here proves resolution attempts the assumption instead of returning the env
				// values verbatim.
				ctx, cancel := context.WithCancel(t.Context())
				cancel()

				_, err := cfg.Credentials.Retrieve(ctx)
				require.Error(t, err)
			case wantRoleProvider:
				// Build installs role-assuming providers as aws.CredentialsProviderFunc; the
				// default chain resolves to a different provider type. Retrieval is not probed
				// here because without env credentials it would consult the host environment.
				assert.IsType(t, aws.CredentialsProviderFunc(nil), cfg.Credentials)
			case wantDefaultChain:
				_, isRoleProvider := cfg.Credentials.(aws.CredentialsProviderFunc)
				assert.False(t, isRoleProvider)
			}
		})
	}
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
