// FIXME: Add this back

// //go:build aws

package test_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
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
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt destroy -auto-approve --non-interactive --working-dir "+rootPath)
			require.NoError(t, err)
		}()

		rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
		helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", region)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --backend-bootstrap --working-dir "+rootPath)
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
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve")
			require.NoError(t, err)
		}()

		rootTerragruntConfigPath := util.JoinPath(rootPath, "root.hcl")
		helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", region)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")
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
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve")
			require.NoError(t, err)
		}()

		rootTerragruntConfigPath := util.JoinPath(rootPath, "root.hcl")
		helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", region)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")
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
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve")
			require.NoError(t, err)
		}()

		rootTerragruntConfigPath := util.JoinPath(rootPath, "root.hcl")
		helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", region)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")
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
			_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- destroy -auto-approve")
			require.NoError(t, err)
		}()

		rootTerragruntConfigPath := util.JoinPath(rootPath, "root.hcl")
		helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", region)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run --all --non-interactive --working-dir "+rootPath+" -- apply -auto-approve")
		require.NoError(t, err)
	})
}

// We don't run these subtests in parallel because each subtest builds on the previous one.
//
//nolint:paralleltest
func TestAwsDocsTerralithToTerragruntGuide(t *testing.T) {
	t.Parallel()

	fixturePath := filepath.Join("..", "docs-starlight", "src", "fixtures", "terralith-to-terragrunt")

	// Create a temporary workspace for the test
	tmpDir := t.TempDir()
	helpers.ExecWithTestLogger(t, tmpDir, "mkdir", "terralith-to-terragrunt")

	// Determine the paths used throughout the steps.
	repoPath := filepath.Join(tmpDir, "terralith-to-terragrunt")
	liveDir := filepath.Join(repoPath, "live")
	distDir := filepath.Join(repoPath, "dist")
	distStaticDir := filepath.Join(repoPath, "dist", "static")

	// Generate unique identifier for the test run
	uniqueID := strings.ToLower(helpers.UniqueID())

	stateBucketName := "terragrunt-terralith-tfstate-" + uniqueID
	name := "terragrunt-terralith-project-" + uniqueID

	region := "us-east-1"

	// Create the backend S3 bucket manually using AWS CLI (as mentioned in the guide)
	//
	// Do it early so we can be sure it's going to be cleaned up at the end.
	helpers.ExecWithTestLogger(t, tmpDir, "aws", "s3api", "create-bucket",
		"--bucket", stateBucketName, "--region", region)
	helpers.ExecWithTestLogger(t, tmpDir, "aws", "s3api", "put-bucket-versioning",
		"--bucket", stateBucketName, "--versioning-configuration", "Status=Enabled")

	// Defer cleanup of state bucket
	defer helpers.DeleteS3Bucket(t, region, stateBucketName)

	t.Run("setup", func(st *testing.T) {
		helpers.ExecWithTestLogger(st, repoPath, "git", "init")

		helpers.ExecWithTestLogger(st, repoPath, "mise", "use", "terragrunt@0.83.2")
		helpers.ExecWithTestLogger(st, repoPath, "mise", "use", "opentofu@1.10.3")
		helpers.ExecWithTestLogger(st, repoPath, "mise", "use", "aws@2.27.63")
		helpers.ExecWithTestLogger(st, repoPath, "mise", "use", "node@22.17.1")

		miseTomlPath := util.JoinPath(repoPath, "mise.toml")
		require.FileExists(st, miseTomlPath)
		miseToml, err := os.ReadFile(miseTomlPath)
		require.NoError(st, err)

		assert.Equal(st, string(miseToml), `[tools]
aws = "2.27.63"
node = "22.17.1"
opentofu = "1.10.3"
terragrunt = "0.83.2"
`)

		helpers.ExecWithTestLogger(st, repoPath, "mkdir", "-p", "app/best-cat")

		bestCatPath := filepath.Join(repoPath, "app", "best-cat")

		// Copy all the best-cat application files
		bestCatFiles := []string{
			"package.json",
			"index.js",
			"template.html",
			"styles.css",
			"script.js",
			"package-lock.json",
		}

		for _, file := range bestCatFiles {
			helpers.CopyFile(
				st,
				filepath.Join(fixturePath, "app", "best-cat", file),
				filepath.Join(bestCatPath, file),
			)
		}

		helpers.ExecWithTestLogger(st, repoPath, "mkdir", "dist")

		helpers.ExecWithTestLogger(st, bestCatPath, "npm", "i")
		helpers.ExecWithTestLogger(st, bestCatPath, "npm", "run", "package")

		require.NoError(st, os.Mkdir(filepath.Join(distDir, "static"), 0755))

		for i := range 10 {
			require.NoError(
				st,
				os.WriteFile(
					filepath.Join(
						distDir,
						"static", fmt.Sprintf("%d-cat.png", i+1)),
					[]byte(""),
					0644,
				),
			)
		}

		st.Log("Setup complete")
	})

	t.Run("step-1-starting-the-terralith", func(st *testing.T) {
		// Create the live directory
		helpers.ExecWithTestLogger(st, repoPath, "mkdir", "live")

		// Copy all the OpenTofu files from fixtures
		fixtureStepPath := filepath.Join(fixturePath, "walkthrough", "step-1-starting-the-terralith", "live")

		// List of files to copy
		terraformFiles := []string{
			"providers.tf",
			"versions.tf",
			"data.tf",
			"ddb.tf",
			"s3.tf",
			"iam.tf",
			"lambda.tf",
			"vars-required.tf",
			"vars-optional.tf",
			"outputs.tf",
			"backend.tf",
		}

		// Copy each file from fixture to live directory
		for _, file := range terraformFiles {
			helpers.CopyFile(
				st,
				filepath.Join(fixtureStepPath, file),
				filepath.Join(liveDir, file),
			)
		}

		// Create the .auto.tfvars file with test-specific values.
		// We set force_destroy to true to avoid errors when destroying the infrastructure.
		tfvarsContent := fmt.Sprintf(`# Required: Name used for all resources (must be unique)
name = "%s"

# Required: Path to your Lambda function zip file
lambda_zip_file = "../dist/best-cat.zip"

# AWS region
aws_region = "%s"

force_destroy = true
`, name, region)

		require.NoError(st, os.WriteFile(
			filepath.Join(liveDir, ".auto.tfvars"),
			[]byte(tfvarsContent),
			0644,
		))

		// Update backend.tf with unique bucket name
		backendContent := fmt.Sprintf(`terraform {
  backend "s3" {
    bucket       = "%s"
    key          = "tofu.tfstate"
    region       = "%s"
    encrypt      = true
    use_lockfile = true
  }
}
`, stateBucketName, region)

		require.NoError(st, os.WriteFile(
			filepath.Join(liveDir, "backend.tf"),
			[]byte(backendContent),
			0644,
		))

		// Always cleanup the infrastructure in-case something goes wrong.
		// We'll re-apply it at the beginning of the next step.
		defer helpers.ExecWithTestLogger(st, liveDir, "tofu", "destroy", "-auto-approve")

		// Initialize and apply the Terraform configuration
		helpers.ExecWithTestLogger(st, liveDir, "tofu", "init")

		helpers.ExecWithTestLogger(st, liveDir, "tofu", "apply", "-auto-approve")

		// Verify the apply was successful by checking outputs
		stdout, _ := helpers.ExecAndCaptureOutput(st, liveDir, "tofu", "output")

		// Check that key outputs exist
		assert.Contains(st, stdout, "lambda_function_url")
		assert.Contains(st, stdout, "s3_bucket_name")
		assert.Contains(st, stdout, "dynamodb_table_name")

		// Get the S3 bucket name from output for asset upload test
		bucketNameOutput, _ := helpers.ExecAndCaptureOutput(st, liveDir, "tofu", "output", "-raw", "s3_bucket_name")

		actualBucketName := strings.TrimSpace(bucketNameOutput)

		require.NotEmpty(st, actualBucketName)

		// Verify the bucket was created and upload test assets using the AWS CLI (as mentioned in the guide)
		helpers.ExecWithTestLogger(st, distStaticDir, "aws", "s3", "sync", ".", fmt.Sprintf("s3://%s/", actualBucketName))

		st.Log("Step 1 - Starting the Terralith completed successfully")
	})
}
