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

	t.Run("step-01", func(t *testing.T) {
		t.Parallel()

		s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

		stepPath := util.JoinPath(testFixtureOverview, "step-01")

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
}
