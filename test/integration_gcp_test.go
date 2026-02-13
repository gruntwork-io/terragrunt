//go:build gcp || awsgcp

package test_test

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	gcsbackend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/gcs"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iterator"
)

const (
	terraformRemoteStateGcpRegion = "eu"

	testFixtureGcsPath                  = "fixtures/gcs/"
	testFixtureGcsByoBucketPath         = "fixtures/gcs-byo-bucket/"
	testFixtureGcsImpersonatePath       = "fixtures/gcs-impersonate/"
	testFixtureGcsNoBucket              = "fixtures/gcs-no-bucket/"
	testFixtureGcsNoPrefix              = "fixtures/gcs-no-prefix/"
	testFixtureGcsParallelStateInit     = "fixtures/gcs-parallel-state-init"
	testFixtureGCSBackend               = "fixtures/gcs-backend"
	testFixtureOutputFromRemoteStateGCS = "fixtures/output-from-remote-state-gcs"
)

func TestGcpBootstrapBackend(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		checkExpectedResultFn func(t *testing.T, stderr string, gcsBucketNameName string, err error)
		name                  string
		args                  string
	}{
		{
			name: "no bootstrap gcs backend without flag",
			args: "run apply",
			checkExpectedResultFn: func(t *testing.T, stderr string, gcsBucketNameName string, err error) {
				t.Helper()

				assert.Contains(t, stderr, "bucket doesn't exist")
				require.Error(t, err)
			},
		},
		{
			name: "bootstrap gcs backend with flag",
			args: "run apply --backend-bootstrap",
			checkExpectedResultFn: func(t *testing.T, stderr string, gcsBucketName string, err error) {
				t.Helper()

				validateGCSBucketExistsAndIsLabeled(t, terraformRemoteStateGcpRegion, gcsBucketName, nil)
				require.NoError(t, err)
			},
		},
		{
			name: "bootstrap gcs backend by backend command",
			args: "backend bootstrap --backend-bootstrap",
			checkExpectedResultFn: func(t *testing.T, stderr string, gcsBucketName string, err error) {
				t.Helper()

				validateGCSBucketExistsAndIsLabeled(t, terraformRemoteStateGcpRegion, gcsBucketName, nil)
				require.NoError(t, err)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureGCSBackend)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGCSBackend)
			rootPath := filepath.Join(tmpEnvPath, testFixtureGCSBackend)

			gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

			defer func() {
				deleteGCSBucket(t, gcsBucketName)
			}()

			project := os.Getenv("GOOGLE_CLOUD_PROJECT")
			commonConfigPath := filepath.Join(rootPath, "common.hcl")
			copyTerragruntGCSConfigAndFillPlaceholders(t, commonConfigPath, commonConfigPath, project, terraformRemoteStateGcpRegion, gcsBucketName)

			_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt "+tc.args+" --all --non-interactive --log-level debug --working-dir "+rootPath)

			tc.checkExpectedResultFn(t, stderr, gcsBucketName, err)
		})
	}
}

func TestGcpBootstrapBackendWithoutVersioning(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGCSBackend)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGCSBackend)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGCSBackend)

	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	defer func() {
		deleteGCSBucket(t, gcsBucketName)
	}()

	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	commonConfigPath := filepath.Join(rootPath, "common.hcl")
	copyTerragruntGCSConfigAndFillPlaceholders(t, commonConfigPath, commonConfigPath, project, terraformRemoteStateGcpRegion, gcsBucketName)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --log-level debug --working-dir "+rootPath+" --feature disable_versioning=true apply --backend-bootstrap",
	)
	require.NoError(t, err)

	validateGCSBucketExistsAndIsLabeled(t, terraformRemoteStateGcpRegion, gcsBucketName, nil)

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t, "terragrunt --non-interactive --log-level debug --working-dir "+rootPath+" backend delete --all --feature disable_versioning=true",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backend delete for unit")

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t, "terragrunt --non-interactive --log-level debug --working-dir "+rootPath+" backend delete --backend-bootstrap --feature disable_versioning=true --all --force",
	)
	require.NoError(t, err)
}

func TestGcpMigrateBackendWithoutVersioning(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGCSBackend)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGCSBackend)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGCSBackend)
	unitPath := filepath.Join(rootPath, "unit1")

	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	defer func() {
		deleteGCSBucket(t, gcsBucketName)
	}()

	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	commonConfigPath := filepath.Join(rootPath, "common.hcl")
	copyTerragruntGCSConfigAndFillPlaceholders(t, commonConfigPath, commonConfigPath, project, terraformRemoteStateGcpRegion, gcsBucketName)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --non-interactive --log-level debug --working-dir "+unitPath+" --feature disable_versioning=true apply --backend-bootstrap -- -auto-approve")
	require.NoError(t, err)

	validateGCSBucketExistsAndIsLabeled(t, terraformRemoteStateGcpRegion, gcsBucketName, nil)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt --non-interactive --log-level debug --working-dir "+rootPath+" backend migrate --backend-bootstrap --feature disable_versioning=true unit1 unit2")
	require.Error(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt --non-interactive --log-level debug --working-dir "+rootPath+" backend migrate --backend-bootstrap --feature disable_versioning=true --force unit1 unit2")
	require.NoError(t, err)
}

func TestGcpDeleteBackend(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGCSBackend)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGCSBackend)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGCSBackend)

	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	defer func() {
		deleteGCSBucket(t, gcsBucketName)
	}()

	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	commonConfigPath := filepath.Join(rootPath, "common.hcl")
	copyTerragruntGCSConfigAndFillPlaceholders(t, commonConfigPath, commonConfigPath, project, terraformRemoteStateGcpRegion, gcsBucketName)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run apply --backend-bootstrap --all --non-interactive --log-level debug --working-dir "+rootPath)
	require.NoError(t, err)

	remoteStateObjectNames := []string{
		"unit1/tofu.tfstate/default.tfstate",
		"unit2/tofu.tfstate/default.tfstate",
	}

	for _, objectName := range remoteStateObjectNames {
		assert.True(t, doesGCSBucketObjectExist(t, gcsBucketName, objectName), "GCS bucket object %s must exist", objectName)
	}

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt backend delete --all --non-interactive --log-level debug --working-dir "+rootPath)
	require.NoError(t, err)

	for _, objectName := range remoteStateObjectNames {
		assert.False(t, doesGCSBucketObjectExist(t, gcsBucketName, objectName), "GCS bucket object %s must not exist", objectName)
	}
}

func TestGcpMigrateBackend(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureGCSBackend)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGCSBackend)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGCSBackend)

	unit1Path := filepath.Join(rootPath, "unit1")
	unit2Path := filepath.Join(rootPath, "unit2")

	unit1BackendKey := "unit1/tofu.tfstate/default.tfstate"
	unit2BackendKey := "unit2/tofu.tfstate/default.tfstate"

	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	defer func() {
		deleteGCSBucket(t, gcsBucketName)
	}()

	commonConfigPath := filepath.Join(rootPath, "common.hcl")
	copyTerragruntGCSConfigAndFillPlaceholders(t, commonConfigPath, commonConfigPath, project, terraformRemoteStateGcpRegion, gcsBucketName)

	// Bootstrap backend and create remote state for unit1.

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run apply --backend-bootstrap --non-interactive --log-level debug --working-dir "+unit1Path+" -- -auto-approve")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Changes to Outputs")

	// Check for remote states.

	assert.True(t, doesGCSBucketObjectExist(t, gcsBucketName, unit1BackendKey), "GCS bucket object %s must exist", unit1BackendKey)
	assert.False(t, doesGCSBucketObjectExist(t, gcsBucketName, unit2BackendKey), "GCS bucket object %s must not exist", unit2BackendKey)

	// Migrate remote state from unit1 to unit2.
	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt backend migrate --log-level debug --working-dir "+rootPath+" unit1 unit2")
	require.NoError(t, err)

	// Check for remote states after migration.
	assert.False(t, doesGCSBucketObjectExist(t, gcsBucketName, unit1BackendKey), "GCS bucket object %s must not exist", unit1BackendKey)
	assert.True(t, doesGCSBucketObjectExist(t, gcsBucketName, unit2BackendKey), "GCS bucket object %s must exist", unit2BackendKey)

	// Run `tofu apply` for unit2 with migrated remote state from unit1.

	stdout, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt run apply --backend-bootstrap --non-interactive --log-level debug --working-dir "+unit2Path+" -- -auto-approve")
	require.NoError(t, err)
	assert.Contains(t, stdout, "No changes")
}

func TestGcpWorksWithBackend(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGcsPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGcsPath)
	helpers.CleanupTerraformFolder(t, rootPath)
	helpers.CleanupTerragruntFolder(t, rootPath)

	// We need a project to create the bucket in, so we pull one from the recommended environment variable.
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	defer deleteGCSBucket(t, gcsBucketName)

	tmpTerragruntGCSConfigPath := createTmpTerragruntGCSConfig(
		t,
		rootPath,
		project,
		terraformRemoteStateGcpRegion,
		gcsBucketName,
		config.DefaultTerragruntConfigPath,
	)
	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt apply -auto-approve --non-interactive --backend-bootstrap --config %s --working-dir %s",
			tmpTerragruntGCSConfigPath,
			rootPath,
		),
	)

	var expectedGCSLabels = map[string]string{
		"owner": "terragrunt_test",
		"name":  "terraform_state_storage"}
	validateGCSBucketExistsAndIsLabeled(t, terraformRemoteStateGcpRegion, gcsBucketName, expectedGCSLabels)
}

func TestGcpWorksWithExistingBucket(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGcsByoBucketPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGcsByoBucketPath)
	helpers.CleanupTerraformFolder(t, rootPath)

	// We need a project to create the bucket in, so we pull one from the recommended environment variable.
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	defer deleteGCSBucket(t, gcsBucketName)

	// manually create the GCS bucket outside the US (default) to test Terragrunt works correctly with an existing bucket.
	location := terraformRemoteStateGcpRegion
	createGCSBucket(t, project, location, gcsBucketName)

	tmpTerragruntGCSConfigPath := createTmpTerragruntGCSConfig(
		t,
		rootPath,
		project,
		terraformRemoteStateGcpRegion,
		gcsBucketName,
		config.DefaultTerragruntConfigPath,
	)
	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt apply -auto-approve --non-interactive --config %s --working-dir %s",
			tmpTerragruntGCSConfigPath,
			rootPath,
		),
	)

	validateGCSBucketExistsAndIsLabeled(t, location, gcsBucketName, nil)
}

func TestGcpCheckMissingBucket(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGcsNoBucket)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGcsNoBucket)
	helpers.CleanupTerraformFolder(t, rootPath)

	// We need a project to create the bucket in, so we pull one from the recommended environment variable.
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	tmpTerragruntGCSConfigPath := createTmpTerragruntGCSConfig(
		t,
		rootPath,
		project,
		terraformRemoteStateGcpRegion,
		gcsBucketName,
		config.DefaultTerragruntConfigPath,
	)
	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt apply -auto-approve --backend-bootstrap --non-interactive --config %s --working-dir %s",
			tmpTerragruntGCSConfigPath,
			rootPath,
		),
	)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "Missing required GCS remote state configuration bucket")
}

func TestGcpNoPrefixBucket(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGcsNoPrefix)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGcsNoPrefix)
	helpers.CleanupTerraformFolder(t, rootPath)

	// We need a project to create the bucket in, so we pull one from the recommended environment variable.
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	defer deleteGCSBucket(t, gcsBucketName)

	tmpTerragruntGCSConfigPath := createTmpTerragruntGCSConfig(t, rootPath, project, terraformRemoteStateGcpRegion, gcsBucketName, config.DefaultTerragruntConfigPath)
	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt apply -auto-approve --backend-bootstrap --non-interactive --config %s --working-dir %s",
			tmpTerragruntGCSConfigPath,
			rootPath,
		),
	)
	require.NoError(t, err)
}

func TestGcpParallelStateInit(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-test") //nolint:usetesting
	if err != nil {
		require.NoError(t, err)
	}

	for i := range 20 {
		err := util.CopyFolderContents(createLogger(), testFixtureGcsParallelStateInit, tmpEnvPath, ".terragrunt-test", nil, nil)
		require.NoError(t, err)

		err = os.Rename(
			path.Join(tmpEnvPath, "template"),
			path.Join(tmpEnvPath, "app"+strconv.Itoa(i)),
		)

		require.NoError(t, err)
	}

	tmpTerragruntConfigFile := filepath.Join(tmpEnvPath, "root.hcl")
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	tmpTerragruntGCSConfigPath := createTmpTerragruntGCSConfig(
		t,
		testFixtureGcsParallelStateInit,
		project,
		terraformRemoteStateGcpRegion,
		gcsBucketName,
		"root.hcl",
	)
	err = util.CopyFile(tmpTerragruntGCSConfigPath, tmpTerragruntConfigFile)
	require.NoError(t, err)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all --backend-bootstrap --non-interactive --working-dir "+tmpEnvPath+" -- apply",
	)
}

func createTmpTerragruntGCSConfig(t *testing.T, templatesPath string, project string, location string, gcsBucketName string, configFileName string) string {
	t.Helper()

	tmpFolder, err := os.MkdirTemp("", "terragrunt-test") //nolint:usetesting
	if err != nil {
		t.Fatalf("Failed to create temp folder due to error: %v", err)
	}

	tmpTerragruntConfigFile := filepath.Join(tmpFolder, configFileName)
	originalTerragruntConfigPath := filepath.Join(templatesPath, configFileName)
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

	extGCSCfg := &gcsbackend.ExtendedRemoteStateConfigGCS{
		RemoteStateConfigGCS: gcsbackend.RemoteStateConfigGCS{
			Bucket: bucketName,
		},
	}

	l := logger.CreateLogger()
	opts := options.NewTerragruntOptions()

	gcsClient, err := gcsbackend.NewClient(t.Context(), l, extGCSCfg, opts)
	require.NoError(t, err, "Error creating GCS client")

	// verify the bucket exists
	assert.True(t, gcsClient.DoesGCSBucketExist(t.Context(), bucketName), "Terragrunt failed to create remote state GCS bucket %s", bucketName)

	// verify the bucket location
	bucket := gcsClient.Bucket(bucketName)
	attrs, err := bucket.Attrs(t.Context())
	require.NoError(t, err)

	assert.Equal(t, strings.ToUpper(location), attrs.Location, "Did not find GCS bucket in expected location.")

	if expectedLabels != nil {
		assertGCSLabels(t, expectedLabels, bucketName, gcsClient.Client)
	}
}

func doesGCSBucketObjectExist(t *testing.T, bucketName, prefix string) bool {
	t.Helper()

	ctx := t.Context()

	extGCSCfg := &gcsbackend.ExtendedRemoteStateConfigGCS{
		RemoteStateConfigGCS: gcsbackend.RemoteStateConfigGCS{
			Bucket: bucketName,
		},
	}

	l := logger.CreateLogger()
	opts := options.NewTerragruntOptions()

	gcsClient, err := gcsbackend.NewClient(ctx, l, extGCSCfg, opts)
	require.NoError(t, err, "Error creating GCS client")

	defer gcsClient.Close()

	bucket := gcsClient.Bucket(bucketName)

	it := bucket.Objects(ctx, &storage.Query{
		Prefix: prefix,
	})

	if _, err := it.Next(); err != nil {
		if errors.Is(err, iterator.Done) {
			return false
		}

		require.NoError(t, err)
	}

	return true
}

// gcsObjectAttrs returns the attributes of the specified object in the bucket
func gcsObjectAttrs(t *testing.T, bucketName string, objectName string) *storage.ObjectAttrs {
	t.Helper()

	ctx := t.Context()

	extGCSCfg := &gcsbackend.ExtendedRemoteStateConfigGCS{
		RemoteStateConfigGCS: gcsbackend.RemoteStateConfigGCS{
			Bucket: bucketName,
		},
	}

	l := logger.CreateLogger()
	opts := options.NewTerragruntOptions()

	gcsClient, err := gcsbackend.NewClient(ctx, l, extGCSCfg, opts)
	require.NoError(t, err, "Error creating GCS client")

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

	ctx := t.Context()
	bucket := client.Bucket(bucketName)

	attrs, err := bucket.Attrs(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var actualLabels = make(map[string]string)

	maps.Copy(actualLabels, attrs.Labels)

	assert.Equal(t, expectedLabels, actualLabels, "Did not find expected labels on GCS bucket.")
}

// Create the specified GCS bucket
func createGCSBucket(t *testing.T, projectID string, location string, bucketName string) {
	t.Helper()

	ctx := t.Context()

	extGCSCfg := &gcsbackend.ExtendedRemoteStateConfigGCS{}

	l := logger.CreateLogger()
	opts := options.NewTerragruntOptions()

	gcsClient, err := gcsbackend.NewClient(ctx, l, extGCSCfg, opts)
	require.NoError(t, err, "Error creating GCS client")

	t.Logf("Creating test GCS bucket %s in project %s, location %s", bucketName, projectID, location)

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

	ctx := t.Context()

	extGCSCfg := &gcsbackend.ExtendedRemoteStateConfigGCS{}

	l := logger.CreateLogger()
	opts := options.NewTerragruntOptions()

	gcsClient, err := gcsbackend.NewClient(ctx, l, extGCSCfg, opts)
	require.NoError(t, err, "Error creating GCS client")

	t.Logf("Deleting test GCS bucket %s", bucketName)

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
			t.Logf("Failed to list objects and versions in GCS bucket %s: %v", bucketName, err)
			return
		}

		// purge the object version
		if err := bucket.Object(objectAttrs.Name).Generation(objectAttrs.Generation).Delete(ctx); err != nil {
			t.Logf("Failed to delete GCS bucket object %s: %v", objectAttrs.Name, err)
			return
		}
	}

	// remote empty bucket
	if err := bucket.Delete(ctx); err != nil {
		t.Fatalf("Failed to delete GCS bucket %s: %v", bucketName, err)
	}
}

func TestGcpOutputFromRemoteState(t *testing.T) { //nolint: paralleltest
	// NOTE: We can't run this test in parallel because there are other tests that also call `config.ClearOutputCache()`, but this function uses a global variable and sometimes it throws an unexpected error:
	// "fixtures/output-from-remote-state-gcs/env1/app2/terragrunt.hcl:23,38-48: Unsupported attribute; This object does not have an attribute named "app3_text"."
	// t.Parallel()
	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	defer deleteGCSBucket(t, gcsBucketName)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputFromRemoteStateGCS)

	rootTerragruntConfigPath := filepath.Join(tmpEnvPath, testFixtureOutputFromRemoteStateGCS, "root.hcl")
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	copyTerragruntGCSConfigAndFillPlaceholders(
		t,
		rootTerragruntConfigPath,
		rootTerragruntConfigPath,
		project,
		terraformRemoteStateGcpRegion,
		gcsBucketName,
	)

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputFromRemoteStateGCS)

	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt run --backend-bootstrap --dependency-fetch-output-from-state "+
				"--non-interactive --working-dir %s/app1 -- apply -auto-approve",
			environmentPath,
		),
	)
	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt run --backend-bootstrap --dependency-fetch-output-from-state "+
				"--non-interactive --working-dir %s/app3 -- apply -auto-approve",
			environmentPath,
		),
	)
	// Now delete dependencies cached state
	// Since terraform runs from cache, the state files are in the cache directories
	app1CacheDir := helpers.FindCacheWorkingDir(t, filepath.Join(environmentPath, "app1"))
	require.NotEmpty(t, app1CacheDir, "Cache directory for app1 should exist")
	require.NoError(t, os.Remove(filepath.Join(app1CacheDir, ".terraform/terraform.tfstate")))
	require.NoError(t, os.RemoveAll(filepath.Join(app1CacheDir, ".terraform")))
	app3CacheDir := helpers.FindCacheWorkingDir(t, filepath.Join(environmentPath, "app3"))
	require.NotEmpty(t, app3CacheDir, "Cache directory for app3 should exist")
	require.NoError(t, os.Remove(filepath.Join(app3CacheDir, ".terraform/terraform.tfstate")))
	require.NoError(t, os.RemoveAll(filepath.Join(app3CacheDir, ".terraform")))

	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt run --backend-bootstrap --dependency-fetch-output-from-state "+
				"--non-interactive --working-dir %s/app2 -- apply -auto-approve",
			environmentPath,
		),
	)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all output --backend-bootstrap --dependency-fetch-output-from-state --non-interactive --working-dir "+environmentPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "app1 output")
	assert.Contains(t, stdout, "app2 output")
	assert.Contains(t, stdout, "app3 output")
	assert.NotContains(t, stderr, "terraform output -json")
	assert.NotContains(t, stderr, "tofu output -json")

	assert.True(
		t, (strings.Index(stdout, "app3 output") < strings.Index(stdout, "app1 output")) &&
			(strings.Index(stdout, "app1 output") < strings.Index(stdout, "app2 output")),
	)
}

func TestGcpNoDependencyFetchOutputFromState(t *testing.T) { //nolint: paralleltest
	// NOTE: We can't run this test in parallel because there are other tests that also call `config.ClearOutputCache()`, but this function uses a global variable and sometimes it throws an unexpected error:
	// "fixtures/output-from-remote-state-gcs/env1/app2/terragrunt.hcl:23,38-48: Unsupported attribute; This object does not have an attribute named "app3_text"."
	// t.Parallel()
	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	defer deleteGCSBucket(t, gcsBucketName)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputFromRemoteStateGCS)

	rootTerragruntConfigPath := filepath.Join(tmpEnvPath, testFixtureOutputFromRemoteStateGCS, "root.hcl")
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	copyTerragruntGCSConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, project, terraformRemoteStateGcpRegion, gcsBucketName)

	environmentPath := fmt.Sprintf("%s/%s/env1", tmpEnvPath, testFixtureOutputFromRemoteStateGCS)

	// Apply dependencies first
	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt apply --backend-bootstrap --dependency-fetch-output-from-state "+
				"--auto-approve --non-interactive --working-dir %s/app1",
			environmentPath,
		),
	)
	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt apply --backend-bootstrap --dependency-fetch-output-from-state "+
				"--auto-approve --non-interactive --working-dir %s/app3",
			environmentPath,
		),
	)
	// Now delete dependencies cached state
	// Since terraform runs from cache, the state files are in the cache directories
	app1CacheDir := helpers.FindCacheWorkingDir(t, filepath.Join(environmentPath, "app1"))
	require.NotEmpty(t, app1CacheDir, "Cache directory for app1 should exist")
	require.NoError(t, os.Remove(filepath.Join(app1CacheDir, ".terraform/terraform.tfstate")))
	require.NoError(t, os.RemoveAll(filepath.Join(app1CacheDir, ".terraform")))
	app3CacheDir := helpers.FindCacheWorkingDir(t, filepath.Join(environmentPath, "app3"))
	require.NotEmpty(t, app3CacheDir, "Cache directory for app3 should exist")
	require.NoError(t, os.Remove(filepath.Join(app3CacheDir, ".terraform/terraform.tfstate")))
	require.NoError(t, os.RemoveAll(filepath.Join(app3CacheDir, ".terraform")))

	// Apply app2 with experiment enabled but --no-dependency-fetch-output-from-state flag set
	// This should fall back to using terraform output instead of fetching from state
	helpers.RunTerragrunt(
		t,
		fmt.Sprintf(
			"terragrunt apply --backend-bootstrap --experiment dependency-fetch-output-from-state "+
				"--no-dependency-fetch-output-from-state --auto-approve --non-interactive --working-dir %s/app2",
			environmentPath,
		),
	)

	// Run output command with experiment enabled but flag set to disable
	// When the flag is set, it should use terraform output instead of fetching from GCS
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --log-level debug --all output --backend-bootstrap --experiment dependency-fetch-output-from-state "+
			"--no-dependency-fetch-output-from-state --non-interactive --working-dir "+environmentPath,
	)
	require.NoError(t, err)

	// Verify outputs are still correct
	assert.Contains(t, stdout, "app1 output")
	assert.Contains(t, stdout, "app2 output")
	assert.Contains(t, stdout, "app3 output")

	// When --no-dependency-fetch-output-from-state is set, it should use terraform output
	// This means we should see "terraform output -json" or "tofu output -json" in stderr
	// (The exact command depends on which terraform implementation is being used)
	// This is the opposite of TestGcpOutputFromRemoteState which asserts this is NOT present
	assert.True(
		t,
		strings.Contains(
			stderr,
			"terraform output",
		) || strings.Contains(
			stderr,
			"tofu output",
		),
		"Expected to see terraform/tofu output command when --no-dependency-fetch-output-from-state flag is set, but stderr was: %s",
		stderr,
	)
}

func TestGcpMockOutputsFromRemoteState(t *testing.T) { //nolint: paralleltest
	// NOTE: We can't run this test in parallel because there are other tests that also call `config.ClearOutputCache()`, but this function uses a global variable and sometimes it throws an unexpected error:
	// "fixtures/output-from-remote-state-gcs/env1/app2/terragrunt.hcl:23,38-48: Unsupported attribute; This object does not have an attribute named "app3_text"."
	// t.Parallel()
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if project == "" {
		t.Skipf("Skipping test because GOOGLE_CLOUD_PROJECT environment variable is not set")
	}

	gcsBucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	defer deleteGCSBucket(t, gcsBucketName)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutputFromRemoteStateGCS)

	rootTerragruntConfigPath := filepath.Join(tmpEnvPath, testFixtureOutputFromRemoteStateGCS, "root.hcl")
	copyTerragruntGCSConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, project, terraformRemoteStateGcpRegion, gcsBucketName)

	environmentPath := filepath.Join(tmpEnvPath, testFixtureOutputFromRemoteStateGCS, "env1")

	// applying only the app1 dependency, the app3 dependency was purposely not applied and should be mocked when running the app2 module
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply --dependency-fetch-output-from-state --auto-approve --backend-bootstrap --non-interactive --working-dir %s/app1", environmentPath))
	// Now delete dependencies cached state
	// Since terraform runs from cache, the state files are in the cache directories
	app1CacheDir := helpers.FindCacheWorkingDir(t, filepath.Join(environmentPath, "app1"))
	require.NotEmpty(t, app1CacheDir, "Cache directory for app1 should exist")
	require.NoError(t, os.RemoveAll(filepath.Join(app1CacheDir, ".terraform")))

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt init --dependency-fetch-output-from-state --non-interactive --working-dir %s/app2", environmentPath))
	require.NoError(t, err)

	assert.Contains(t, stderr, "Failed to read outputs")
	assert.Contains(t, stderr, "fallback to mock outputs")
}
