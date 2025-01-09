//go:build gcp

package test_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iterator"
)

const (
	terraformRemoteStateGcpRegion = "eu"

	testFixtureGcsPath              = "fixtures/gcs/"
	testFixtureGcsByoBucketPath     = "fixtures/gcs-byo-bucket/"
	testFixtureGcsImpersonatePath   = "fixtures/gcs-impersonate/"
	testFixtureGcsNoBucket          = "fixtures/gcs-no-bucket/"
	testFixtureGcsNoPrefix          = "fixtures/gcs-no-prefix/"
	testFixtureGcsParallelStateInit = "fixtures/gcs-parallel-state-init"
)

func TestGcpWorksWithBackend(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGcsPath)

	// We need a project to create the bucket in, so we pull one from the recommended environment variable.
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	defer deleteGCSBucket(t, gcsBucketName)

	tmpTerragruntGCSConfigPath := createTmpTerragruntGCSConfig(t, testFixtureGcsPath, project, terraformRemoteStateGcpRegion, gcsBucketName, config.DefaultTerragruntConfigPath)
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntGCSConfigPath, testFixtureGcsPath))

	var expectedGCSLabels = map[string]string{
		"owner": "terragrunt_test",
		"name":  "terraform_state_storage"}
	validateGCSBucketExistsAndIsLabeled(t, terraformRemoteStateGcpRegion, gcsBucketName, expectedGCSLabels)
}

func TestGcpWorksWithExistingBucket(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGcsByoBucketPath)

	// We need a project to create the bucket in, so we pull one from the recommended environment variable.
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	defer deleteGCSBucket(t, gcsBucketName)

	// manually create the GCS bucket outside the US (default) to test Terragrunt works correctly with an existing bucket.
	location := terraformRemoteStateGcpRegion
	createGCSBucket(t, project, location, gcsBucketName)

	tmpTerragruntGCSConfigPath := createTmpTerragruntGCSConfig(t, testFixtureGcsByoBucketPath, project, terraformRemoteStateGcpRegion, gcsBucketName, config.DefaultTerragruntConfigPath)
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntGCSConfigPath, testFixtureGcsByoBucketPath))

	validateGCSBucketExistsAndIsLabeled(t, location, gcsBucketName, nil)
}

func TestGcpCheckMissingBucket(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGcsNoBucket)

	// We need a project to create the bucket in, so we pull one from the recommended environment variable.
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	tmpTerragruntGCSConfigPath := createTmpTerragruntGCSConfig(t, testFixtureGcsNoBucket, project, terraformRemoteStateGcpRegion, gcsBucketName, config.DefaultTerragruntConfigPath)
	_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntGCSConfigPath, testFixtureGcsNoBucket))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Missing required GCS remote state configuration bucket")
}

func TestGcpNoPrefixBucket(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGcsNoPrefix)

	// We need a project to create the bucket in, so we pull one from the recommended environment variable.
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	defer deleteGCSBucket(t, gcsBucketName)

	tmpTerragruntGCSConfigPath := createTmpTerragruntGCSConfig(t, testFixtureGcsNoPrefix, project, terraformRemoteStateGcpRegion, gcsBucketName, config.DefaultTerragruntConfigPath)
	_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntGCSConfigPath, testFixtureGcsNoPrefix))
	require.NoError(t, err)
}

func TestGcpParallelStateInit(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-test")
	if err != nil {
		require.NoError(t, err)
	}
	for i := 0; i < 20; i++ {
		err := util.CopyFolderContents(createLogger(), testFixtureGcsParallelStateInit, tmpEnvPath, ".terragrunt-test", nil, nil)
		require.NoError(t, err)
		err = os.Rename(
			path.Join(tmpEnvPath, "template"),
			path.Join(tmpEnvPath, "app"+strconv.Itoa(i)))
		require.NoError(t, err)
	}

	tmpTerragruntConfigFile := util.JoinPath(tmpEnvPath, "root.hcl")
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	tmpTerragruntGCSConfigPath := createTmpTerragruntGCSConfig(t, testFixtureGcsParallelStateInit, project, terraformRemoteStateGcpRegion, gcsBucketName, "root.hcl")
	err = util.CopyFile(tmpTerragruntGCSConfigPath, tmpTerragruntConfigFile)
	require.NoError(t, err)

	helpers.RunTerragrunt(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+tmpEnvPath)
}

func createTmpTerragruntGCSConfig(t *testing.T, templatesPath string, project string, location string, gcsBucketName string, configFileName string) string {
	t.Helper()

	tmpFolder, err := os.MkdirTemp("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp folder due to error: %v", err)
	}

	tmpTerragruntConfigFile := util.JoinPath(tmpFolder, configFileName)
	originalTerragruntConfigPath := util.JoinPath(templatesPath, configFileName)
	copyTerragruntGCSConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, project, location, gcsBucketName)

	return tmpTerragruntConfigFile
}

func copyTerragruntGCSConfigAndFillPlaceholders(t *testing.T, configSrcPath string, configDestPath string, project string, location string, gcsBucketName string) {
	t.Helper()

	email := os.Getenv("GOOGLE_IDENTITY_EMAIL")

	helpers.CopyAndFillMapPlaceholders(t, configSrcPath, configDestPath, map[string]string{
		"__FILL_IN_PROJECT__":     project,
		"__FILL_IN_LOCATION__":    location,
		"__FILL_IN_BUCKET_NAME__": gcsBucketName,
		"__FILL_IN_GCP_EMAIL__":   email,
	})
}

// Check that the GCS Bucket of the given name and location exists. Terragrunt should create this bucket during the test.
// Also check if bucket got labeled properly.
func validateGCSBucketExistsAndIsLabeled(t *testing.T, location string, bucketName string, expectedLabels map[string]string) {
	t.Helper()

	remoteStateConfig := remote.RemoteStateConfigGCS{Bucket: bucketName}

	gcsClient, err := remote.CreateGCSClient(remoteStateConfig)
	require.NoError(t, err, "Error creating GCS client")

	// verify the bucket exists
	assert.True(t, remote.DoesGCSBucketExist(gcsClient, &remoteStateConfig), "Terragrunt failed to create remote state GCS bucket %s", bucketName)

	// verify the bucket location
	bucket := gcsClient.Bucket(bucketName)
	attrs, err := bucket.Attrs(context.Background())
	require.NoError(t, err)

	assert.Equal(t, strings.ToUpper(location), attrs.Location, "Did not find GCS bucket in expected location.")

	if expectedLabels != nil {
		assertGCSLabels(t, expectedLabels, bucketName, gcsClient)
	}
}

// gcsObjectAttrs returns the attributes of the specified object in the bucket
func gcsObjectAttrs(t *testing.T, bucketName string, objectName string) *storage.ObjectAttrs {
	t.Helper()

	remoteStateConfig := remote.RemoteStateConfigGCS{Bucket: bucketName}

	gcsClient, err := remote.CreateGCSClient(remoteStateConfig)
	if err != nil {
		t.Fatalf("Error creating GCS client: %v", err)
	}

	ctx := context.Background()
	bucket := gcsClient.Bucket(bucketName)

	handle := bucket.Object(objectName)
	attrs, err := handle.Attrs(ctx)
	if err != nil {
		t.Fatalf("Error reading object attributes %s %v", objectName, err)
	}
	return attrs
}

func assertGCSLabels(t *testing.T, expectedLabels map[string]string, bucketName string, client *storage.Client) {
	t.Helper()

	ctx := context.Background()
	bucket := client.Bucket(bucketName)

	attrs, err := bucket.Attrs(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var actualLabels = make(map[string]string)

	for key, value := range attrs.Labels {
		actualLabels[key] = value
	}

	assert.Equal(t, expectedLabels, actualLabels, "Did not find expected labels on GCS bucket.")
}

// Create the specified GCS bucket
func createGCSBucket(t *testing.T, projectID string, location string, bucketName string) {
	t.Helper()

	var gcsConfig remote.RemoteStateConfigGCS
	gcsClient, err := remote.CreateGCSClient(gcsConfig)
	if err != nil {
		t.Fatalf("Error creating GCS client: %v", err)
	}

	t.Logf("Creating test GCS bucket %s in project %s, location %s", bucketName, projectID, location)

	ctx := context.Background()
	bucket := gcsClient.Bucket(bucketName)

	bucketAttrs := &storage.BucketAttrs{
		Location:          location,
		VersioningEnabled: true,
	}

	if err := bucket.Create(ctx, projectID, bucketAttrs); err != nil {
		t.Fatalf("Failed to create GCS bucket %s: %v", bucketName, err)
	}
}

// Delete the specified GCS bucket to clean up after a test
func deleteGCSBucket(t *testing.T, bucketName string) {
	t.Helper()

	var gcsConfig remote.RemoteStateConfigGCS
	gcsClient, err := remote.CreateGCSClient(gcsConfig)
	if err != nil {
		t.Fatalf("Error creating GCS client: %v", err)
	}

	t.Logf("Deleting test GCS bucket %s", bucketName)

	ctx := context.Background()

	// List all objects including their versions in the bucket
	bucket := gcsClient.Bucket(bucketName)
	q := &storage.Query{
		Versions: true,
	}
	it := bucket.Objects(ctx, q)
	for {
		objectAttrs, err := it.Next()

		if errors.Is(err, iterator.Done) {
			break
		}

		if err != nil {
			t.Fatalf("Failed to list objects and versions in GCS bucket %s: %v", bucketName, err)
		}

		// purge the object version
		if err := bucket.Object(objectAttrs.Name).Generation(objectAttrs.Generation).Delete(ctx); err != nil {
			t.Fatalf("Failed to delete GCS bucket object %s: %v", objectAttrs.Name, err)
		}
	}

	// remote empty bucket
	if err := bucket.Delete(ctx); err != nil {
		t.Fatalf("Failed to delete GCS bucket %s: %v", bucketName, err)
	}
}
