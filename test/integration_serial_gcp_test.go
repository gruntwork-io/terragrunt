//go:build gcp

package test_test

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/stretchr/testify/assert"
	"os"
	"strings"
	"testing"
)

func TestTerragruntWorksWithImpersonateGCSBackend(t *testing.T) {
	impersonatorKey := os.Getenv("GCLOUD_SERVICE_KEY_IMPERSONATOR")
	if impersonatorKey == "" {
		t.Fatalf("required environment variable `%s` - not found", "GCLOUD_SERVICE_KEY_IMPERSONATOR")
	}
	tmpImpersonatorCreds := createTmpTerragruntConfigContent(t, impersonatorKey, "impersonator-key.json")
	defer removeFile(t, tmpImpersonatorCreds)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", tmpImpersonatorCreds)

	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())

	// run with impersonation
	tmpTerragruntImpersonateGCSConfigPath := createTmpTerragruntGCSConfig(t, testFixtureGcsImpersonatePath, project, terraformRemoteStateGcpRegion, gcsBucketName, config.DefaultTerragruntConfigPath)
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntImpersonateGCSConfigPath, testFixtureGcsImpersonatePath))

	var expectedGCSLabels = map[string]string{
		"owner": "terragrunt_test",
		"name":  "terraform_state_storage"}
	validateGCSBucketExistsAndIsLabeled(t, terraformRemoteStateGcpRegion, gcsBucketName, expectedGCSLabels)

	email := os.Getenv("GOOGLE_IDENTITY_EMAIL")
	attrs := gcsObjectAttrs(t, gcsBucketName, "terraform.tfstate/default.tfstate")
	ownerEmail := false
	for _, a := range attrs.ACL {
		if (a.Role == "OWNER") && (a.Email == email) {
			ownerEmail = true
			break
		}
	}
	assert.True(t, ownerEmail, "Identity email should match the impersonated account")
}

func TestTerragruntCorrectlyMirrorsTerraformGCPAuth(t *testing.T) {
	// We need to ensure Terragrunt works correctly when GOOGLE_CREDENTIALS are specified.
	// There is no true way to properly unset env vars from the environment, but we still try
	// to unset the CI credentials during this test.
	defaultCreds := os.Getenv("GCLOUD_SERVICE_KEY")
	defer os.Setenv("GCLOUD_SERVICE_KEY", defaultCreds)
	os.Unsetenv("GCLOUD_SERVICE_KEY")
	os.Setenv("GOOGLE_CREDENTIALS", defaultCreds)

	cleanupTerraformFolder(t, testFixtureGcsPath)

	// We need a project to create the bucket in, so we pull one from the recommended environment variable.
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueId())

	defer deleteGCSBucket(t, gcsBucketName)

	tmpTerragruntGCSConfigPath := createTmpTerragruntGCSConfig(t, testFixtureGcsPath, project, terraformRemoteStateGcpRegion, gcsBucketName, config.DefaultTerragruntConfigPath)
	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntGCSConfigPath, testFixtureGcsPath))

	var expectedGCSLabels = map[string]string{
		"owner": "terragrunt_test",
		"name":  "terraform_state_storage"}
	validateGCSBucketExistsAndIsLabeled(t, terraformRemoteStateGcpRegion, gcsBucketName, expectedGCSLabels)
}
