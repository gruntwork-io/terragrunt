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

func TestAwsDocsTerralithToTerragruntGuide(t *testing.T) {
	t.Parallel()

	fixturePath := filepath.Join("..", "docs-starlight", "src", "fixtures", "terralith-to-terragrunt")

	// Create a temporary workspace for the test
	tmpDir := t.TempDir()
	helpers.ExecWithTestLogger(t, tmpDir, "mkdir", "terralith-to-terragrunt")

	// Determine the paths used throughout the steps.
	repoDir := filepath.Join(tmpDir, "terralith-to-terragrunt")
	liveDir := filepath.Join(repoDir, "live")
	distDir := filepath.Join(repoDir, "dist")
	distStaticDir := filepath.Join(repoDir, "dist", "static")
	catalogDir := filepath.Join(repoDir, "catalog")
	catalogModulesDir := filepath.Join(catalogDir, "modules")

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

	func() {
		t.Log("Running step 0 - Setup")

		helpers.ExecWithTestLogger(t, repoDir, "git", "init")

		helpers.ExecWithTestLogger(t, repoDir, "mise", "use", "terragrunt@0.83.2")
		helpers.ExecWithTestLogger(t, repoDir, "mise", "use", "opentofu@1.10.3")
		helpers.ExecWithTestLogger(t, repoDir, "mise", "use", "aws@2.27.63")
		helpers.ExecWithTestLogger(t, repoDir, "mise", "use", "node@22.17.1")

		miseTomlPath := util.JoinPath(repoDir, "mise.toml")
		require.FileExists(t, miseTomlPath)
		miseToml, err := os.ReadFile(miseTomlPath)
		require.NoError(t, err)

		assert.Equal(t, string(miseToml), `[tools]
aws = "2.27.63"
node = "22.17.1"
opentofu = "1.10.3"
terragrunt = "0.83.2"
`)

		helpers.ExecWithTestLogger(t, repoDir, "mkdir", "-p", "app/best-cat")

		bestCatPath := filepath.Join(repoDir, "app", "best-cat")

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
				t,
				filepath.Join(fixturePath, "app", "best-cat", file),
				filepath.Join(bestCatPath, file),
			)
		}

		helpers.ExecWithTestLogger(t, repoDir, "mkdir", "dist")

		helpers.ExecWithTestLogger(t, bestCatPath, "npm", "i")
		helpers.ExecWithTestLogger(t, bestCatPath, "npm", "run", "package")

		require.NoError(t, os.Mkdir(filepath.Join(distDir, "static"), 0755))

		for i := range 10 {
			require.NoError(
				t,
				os.WriteFile(
					filepath.Join(
						distDir,
						"static", fmt.Sprintf("%d-cat.png", i+1)),
					[]byte(""),
					0644,
				),
			)
		}

		t.Log("Setup complete")
	}()

	func() {
		t.Log("Running step 1 - Starting the Terralith")

		// We do a check like this to make sure we properly clean up infrastructure only when we fail.
		//
		// We need our infrastructure to persist between steps so that we can test stateful refactoring.
		pass := false
		defer func() {
			if !pass {
				helpers.ExecWithTestLogger(t, liveDir, "tofu", "destroy", "-auto-approve")
			}
		}()

		// Create the live directory
		helpers.ExecWithTestLogger(t, repoDir, "mkdir", "live")

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
				t,
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

		require.NoError(t, os.WriteFile(
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

		require.NoError(t, os.WriteFile(
			filepath.Join(liveDir, "backend.tf"),
			[]byte(backendContent),
			0644,
		))

		// Initialize and apply the Terraform configuration
		helpers.ExecWithTestLogger(t, liveDir, "tofu", "init")

		// Apply the Terraform configuration
		helpers.ExecWithTestLogger(t, liveDir, "tofu", "apply", "-auto-approve")

		// Verify the apply was successful by checking outputs
		stdout, _ := helpers.ExecAndCaptureOutput(t, liveDir, "tofu", "output")

		// Check that key outputs exist
		assert.Contains(t, stdout, "lambda_function_url")
		assert.Contains(t, stdout, "s3_bucket_name")
		assert.Contains(t, stdout, "dynamodb_table_name")

		// Get the S3 bucket name from output for asset upload test
		bucketNameOutput, _ := helpers.ExecAndCaptureOutput(t, liveDir, "tofu", "output", "-raw", "s3_bucket_name")

		actualBucketName := strings.TrimSpace(bucketNameOutput)

		require.NotEmpty(t, actualBucketName)

		helpers.ExecWithTestLogger(
			t,
			distStaticDir,
			"aws", "s3", "sync", ".", fmt.Sprintf("s3://%s/", actualBucketName),
		)

		t.Log("Step 1 - Starting the Terralith completed successfully")
		pass = true
	}()

	func() {
		t.Log("Running step 2 - Refactoring")

		// We do a check like this to make sure we properly clean up infrastructure only when we fail.
		//
		// We need our infrastructure to persist between steps so that we can test stateful refactoring.
		pass := false
		defer func() {
			if !pass {
				helpers.ExecWithTestLogger(t, liveDir, "tofu", "destroy", "-auto-approve")
			}
		}()

		// Create the catalog directory structure
		helpers.ExecWithTestLogger(t, repoDir, "bash", "-c", "mkdir -p catalog/modules/{s3,lambda,iam,ddb}")

		// Remove the old individual .tf files that will be moved to modules
		oldFiles := []string{"ddb.tf", "iam.tf", "data.tf", "lambda.tf", "s3.tf"}
		for _, file := range oldFiles {
			filePath := filepath.Join(liveDir, file)
			require.NoError(t, os.Remove(filePath))
		}

		// Path to step 2 fixtures
		fixtureStepPath := filepath.Join(fixturePath, "walkthrough", "step-2-refactoring")

		// Copy all module files from fixtures to the catalog directory
		modules := []string{"s3", "lambda", "iam", "ddb"}
		for _, module := range modules {
			moduleSourcePath := filepath.Join(fixtureStepPath, "catalog", "modules", module)
			moduleDestPath := filepath.Join(catalogModulesDir, module)

			// List of files that may exist in each module
			moduleFiles := []string{
				"main.tf",
				"outputs.tf",
				"vars-required.tf",
				"vars-optional.tf",
				"versions.tf",
				"data.tf",
			}

			for _, file := range moduleFiles {
				sourceFile := filepath.Join(moduleSourcePath, file)
				destFile := filepath.Join(moduleDestPath, file)

				// Only copy if the source file exists
				if _, err := os.Stat(sourceFile); err == nil {
					helpers.CopyFile(t, sourceFile, destFile)
				}
			}
		}

		// Update the live directory with refactored files
		liveSourcePath := filepath.Join(fixtureStepPath, "live")

		// Files to copy/update in the live directory
		liveFiles := []string{
			"main.tf",
			"moved.tf",
			"outputs.tf",
			"vars-optional.tf",
		}

		for _, file := range liveFiles {
			helpers.CopyFile(
				t,
				filepath.Join(liveSourcePath, file),
				filepath.Join(liveDir, file),
			)
		}

		// Re-initialize since we're now using modules
		helpers.ExecWithTestLogger(t, liveDir, "tofu", "init")

		// Run plan to verify the refactoring - should show 0 changes due to moved blocks
		stdout, _ := helpers.ExecAndCaptureOutput(t, liveDir, "tofu", "plan")

		// Verify that the plan shows no changes (the moved blocks should handle state migration)
		assert.Contains(t, stdout, "0 to add, 0 to change, 0 to destroy")

		// Apply the configuration to ensure everything works
		helpers.ExecWithTestLogger(t, liveDir, "tofu", "apply", "-auto-approve")

		// Verify outputs still work after refactoring
		outputStdout, _ := helpers.ExecAndCaptureOutput(t, liveDir, "tofu", "output")

		// Check that key outputs still exist after refactoring
		assert.Contains(t, outputStdout, "lambda_function_url")
		assert.Contains(t, outputStdout, "s3_bucket_name")
		assert.Contains(t, outputStdout, "dynamodb_table_name")

		t.Log("Step 2 - Refactoring completed successfully")
		pass = true
	}()

	func() {
		t.Log("Cleanup")

		// Always cleanup the infrastructure at the end
		helpers.ExecWithTestLogger(t, liveDir, "tofu", "destroy", "-auto-approve")
	}()
}
