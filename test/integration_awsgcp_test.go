//go:build awsgcp

package test_test

import (
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureS3BackendMigrate = "fixtures/s3-backend-migrate"
)

func TestAwsGcpMigrateBetweenDifferentBackends(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureS3BackendMigrate)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureS3BackendMigrate)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureS3BackendMigrate)

	testID := strings.ToLower(helpers.UniqueID())

	s3BucketName := "terragrunt-test-bucket-" + testID
	dynamoDBName := "terragrunt-test-dynamodb-" + testID
	gcsBucketName := "terragrunt-test-bucket-" + testID

	project := os.Getenv("GOOGLE_CLOUD_PROJECT")

	unit1Path := util.JoinPath(rootPath, "unit1")
	unit2Path := util.JoinPath(rootPath, "unit2")

	defer func() {
		deleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)
		cleanupTableForTest(t, dynamoDBName, helpers.TerraformRemoteStateS3Region)
		deleteGCSBucket(t, gcsBucketName)
	}()

	unit1ConfigPath := util.JoinPath(unit1Path, "terragrunt.hcl")
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, unit1ConfigPath, unit1ConfigPath, s3BucketName, dynamoDBName, helpers.TerraformRemoteStateS3Region)

	unit2ConfigPath := util.JoinPath(unit2Path, "terragrunt.hcl")
	copyTerragruntGCSConfigAndFillPlaceholders(t, unit2ConfigPath, unit2ConfigPath, project, terraformRemoteStateGcpRegion, gcsBucketName)

	// Bootstrap backend and create remote state for unit1.

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run apply --backend-bootstrap --non-interactive --log-level debug --working-dir "+unit1Path+" -- -auto-approve")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Changes to Outputs")

	stdout, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run plan --backend-bootstrap --non-interactive --log-level debug --working-dir "+unit2Path+"")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Changes to Outputs")

	// Migrate remote state from unit1 to unit2.
	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt backend migrate --log-level debug --working-dir "+rootPath+" unit1 unit2")
	require.NoError(t, err)

	// Run `tofu apply` for unit2 with migrated remote state from unit1.

	stdout, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run apply --backend-bootstrap --non-interactive --log-level debug --working-dir "+unit2Path+" -- -auto-approve")
	require.NoError(t, err)
	assert.Contains(t, stdout, "No changes")
}
