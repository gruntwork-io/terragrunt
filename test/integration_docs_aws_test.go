//go:build aws

package test_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureOverview = "fixtures/docs/02-overview"
)

func TestAwsDocsOverview(t *testing.T) {
	t.Parallel()

	// These docs examples specifically run here
	region := "us-east-1"

	t.Run("step-01-terragrunt.hcl", func(t *testing.T) {
		t.Parallel()

		s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

		stepPath := util.JoinPath(testFixtureOverview, "step-01-terragrunt.hcl")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		defer helpers.DeleteS3Bucket(t, region, s3BucketName)
		defer func() {
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt destroy -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
			require.NoError(t, err)
		}()

		rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
		helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", region)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)
	})

	t.Run("step-02-dependencies", func(t *testing.T) {
		t.Parallel()

		s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

		stepPath := util.JoinPath(testFixtureOverview, "step-02-dependencies")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		defer helpers.DeleteS3Bucket(t, region, s3BucketName)
		defer func() {
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all destroy -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
			require.NoError(t, err)
		}()

		rootTerragruntConfigPath := util.JoinPath(rootPath, "root.hcl")
		helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", region)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)
	})

	t.Run("step-03-mock-outputs", func(t *testing.T) {
		t.Parallel()

		s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

		stepPath := util.JoinPath(testFixtureOverview, "step-03-mock-outputs")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		defer helpers.DeleteS3Bucket(t, region, s3BucketName)
		defer func() {
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all destroy -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
			require.NoError(t, err)
		}()

		rootTerragruntConfigPath := util.JoinPath(rootPath, "root.hcl")
		helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", region)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)
	})

	t.Run("step-04-configuration-hierarchy", func(t *testing.T) {
		t.Parallel()

		s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

		stepPath := util.JoinPath(testFixtureOverview, "step-04-configuration-hierarchy")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		defer helpers.DeleteS3Bucket(t, region, s3BucketName)
		defer func() {
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all destroy -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
			require.NoError(t, err)
		}()

		rootTerragruntConfigPath := util.JoinPath(rootPath, "root.hcl")
		helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", region)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)
	})

	t.Run("step-05-exposed-includes", func(t *testing.T) {
		t.Parallel()

		s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

		stepPath := util.JoinPath(testFixtureOverview, "step-05-exposed-includes")

		helpers.CleanupTerraformFolder(t, stepPath)
		tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
		rootPath := util.JoinPath(tmpEnvPath, stepPath)

		defer helpers.DeleteS3Bucket(t, region, s3BucketName)
		defer func() {
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all destroy -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
			require.NoError(t, err)
		}()

		rootTerragruntConfigPath := util.JoinPath(rootPath, "root.hcl")
		helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", region)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
		require.NoError(t, err)
	})
}
