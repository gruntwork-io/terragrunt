//go:build awsoidc

// These tests aren't hooked up to CI right now, as
// we'll soon be moving to a new CI system (GitHub Actions)
// and we don't want to add complexity to the migration by handling
// both CircleCI and GitHub Actions at the same time.

package test_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAwsAssumeRoleWebIdentityFile(t *testing.T) {
	if os.Getenv("CIRCLECI") != "true" {
		t.Skip("Skipping test because it requires valid CircleCI OIDC credentials to work")
	}

	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	// These tests need to be run without the static key + secret
	// used by most AWS tests here.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	os.Unsetenv("AWS_SECRET_ACCESS_KEY")

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAssumeRoleWebIdentityFile)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureAssumeRoleWebIdentityFile)

	originalTerragruntConfigPath := util.JoinPath(testFixtureAssumeRoleWebIdentityFile, "terragrunt.hcl")
	tmpTerragruntConfigFile := util.JoinPath(testPath, "terragrunt.hcl")
	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	role := os.Getenv("AWS_TEST_OIDC_ROLE_ARN")
	require.NotEmpty(t, role)
	token := os.Getenv("CIRCLE_OIDC_TOKEN_V2")
	require.NotEmpty(t, token)

	tokenFile := t.TempDir() + "/oidc-token"
	require.NoError(t, os.WriteFile(tokenFile, []byte(token), 0400))

	defer func() {
		t.Setenv("AWS_ACCESS_KEY_ID", accessKeyID)
		t.Setenv("AWS_SECRET_ACCESS_KEY", secretAccessKey)

		helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName, options.WithIAMRoleARN(role), options.WithIAMWebIdentityToken(token))
	}()

	helpers.CopyAndFillMapPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, map[string]string{
		"__FILL_IN_BUCKET_NAME__":              s3BucketName,
		"__FILL_IN_REGION__":                   helpers.TerraformRemoteStateS3Region,
		"__FILL_IN_ASSUME_ROLE__":              role,
		"__FILL_IN_IDENTITY_TOKEN_FILE_PATH__": tokenFile,
	})

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt apply -auto-approve --non-interactive --log-level trace --working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)

	output := fmt.Sprintf("%s %s", stderr.String(), stdout.String())
	assert.Contains(t, output, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.")
}

func TestAwsAssumeRoleWebIdentityFlag(t *testing.T) {
	if os.Getenv("CIRCLECI") != "true" {
		t.Skip("Skipping test because it requires valid CircleCI OIDC credentials to work")
	}

	// These tests need to be run without the static key + secret
	// used by most AWS tests here.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	os.Unsetenv("AWS_SECRET_ACCESS_KEY")

	tmp := t.TempDir()

	emptyTerragruntConfigPath := filepath.Join(tmp, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(emptyTerragruntConfigPath, []byte(""), 0400))

	emptyMainTFPath := filepath.Join(tmp, "main.tf")
	require.NoError(t, os.WriteFile(emptyMainTFPath, []byte(""), 0400))

	roleARN := os.Getenv("AWS_TEST_OIDC_ROLE_ARN")
	require.NotEmpty(t, roleARN)
	token := os.Getenv("CIRCLE_OIDC_TOKEN_V2")
	require.NotEmpty(t, token)

	helpers.RunTerragrunt(t, "terragrunt apply --non-interactive --log-level trace --working-dir "+tmp+" --iam-assume-role "+roleARN+" --iam-assume-role-web-identity-token "+token)
}
