package helpers_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The AWS client constructors read the process environment, so these tests point config files at missing paths.

//nolint:paralleltest // pkgTIsolateAWSConfig calls t.Setenv on the process environment.
func TestCreateS3ClientForTest(t *testing.T) {
	pkgTIsolateAWSConfig(t)

	applied := false
	withRole := func(opts *options.TerragruntOptions) {
		applied = true
		opts.IAMRoleOptions.RoleARN = "arn:aws:iam::123456789012:role/terragrunt-test"
	}

	client := helpers.CreateS3ClientForTest(t, "eu-central-1", withRole)

	require.NotNil(t, client)
	assert.True(t, applied, "every option func is applied to the options")
	assert.Equal(t, "eu-central-1", client.Options().Region)
	assert.NotNil(t, client.Options().Credentials)
}

//nolint:paralleltest // pkgTIsolateAWSConfig calls t.Setenv on the process environment.
func TestCreateDynamoDBClientForTest(t *testing.T) {
	pkgTIsolateAWSConfig(t)

	testCases := []struct {
		name    string
		region  string
		roleArn string
	}{
		{
			name:   "default credentials",
			region: "us-east-2",
		},
		{
			name:    "assumed role",
			region:  "ap-southeast-1",
			roleArn: "arn:aws:iam::123456789012:role/terragrunt-test",
		},
	}

	for _, tc := range testCases {
		client := helpers.CreateDynamoDBClientForTest(t, tc.region, "", tc.roleArn)

		require.NotNil(t, client, tc.name)
		assert.Equal(t, tc.region, client.Options().Region, tc.name)
	}
}

// pkgTIsolateAWSConfig points the AWS SDK at missing config files and clears the profile, so local setup is ignored.
func pkgTIsolateAWSConfig(t *testing.T) {
	t.Helper()

	dir := t.TempDir()

	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "credentials"))
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_DEFAULT_PROFILE", "")
}
