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
	devDir := filepath.Join(liveDir, "dev")
	prodDir := filepath.Join(liveDir, "prod")

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
		t.Log("Running step 3 - Adding dev")

		// We do a check like this to make sure we properly clean up infrastructure only when we fail.
		//
		// We need our infrastructure to persist between steps so that we can test stateful refactoring.
		pass := false
		defer func() {
			if !pass {
				helpers.ExecWithTestLogger(t, liveDir, "tofu", "destroy", "-auto-approve")
			}
		}()

		// Path to step 3 fixtures
		fixtureStepPath := filepath.Join(fixturePath, "walkthrough", "step-3-adding-dev")

		// Create the best_cat module directory
		bestCatModulePath := filepath.Join(catalogModulesDir, "best_cat")
		helpers.ExecWithTestLogger(t, repoDir, "mkdir", "-p", bestCatModulePath)

		// Copy the best_cat module files
		bestCatSourcePath := filepath.Join(fixtureStepPath, "catalog", "modules", "best_cat")
		bestCatFiles := []string{
			"main.tf",
			"outputs.tf",
			"vars-optional.tf",
			"vars-required.tf",
		}

		for _, file := range bestCatFiles {
			helpers.CopyFile(
				t,
				filepath.Join(bestCatSourcePath, file),
				filepath.Join(bestCatModulePath, file),
			)
		}

		// Update the live directory with step 3 files
		liveSourcePath := filepath.Join(fixtureStepPath, "live")

		// Files to copy/update in the live directory for step 3
		liveFiles := []string{
			"main.tf",
			"moved.tf",
			"outputs.tf",
		}

		for _, file := range liveFiles {
			helpers.CopyFile(
				t,
				filepath.Join(liveSourcePath, file),
				filepath.Join(liveDir, file),
			)
		}

		// Re-initialize since we're now using the new best_cat module
		helpers.ExecWithTestLogger(t, liveDir, "tofu", "init")

		// Run plan to verify the refactoring - should show only new dev resources due to moved blocks
		stdout, _ := helpers.ExecAndCaptureOutput(t, liveDir, "tofu", "plan")

		// Verify that the plan shows the expected new dev resources (11 new resources for dev environment)
		assert.Contains(t, stdout, "11 to add")
		assert.Contains(t, stdout, "0 to change")
		assert.Contains(t, stdout, "0 to destroy")

		// Apply the configuration to create the dev environment
		helpers.ExecWithTestLogger(t, liveDir, "tofu", "apply", "-auto-approve")

		// Verify outputs for both dev and prod environments
		outputStdout, _ := helpers.ExecAndCaptureOutput(t, liveDir, "tofu", "output")

		// Check that both dev and prod outputs exist
		assert.Contains(t, outputStdout, "dev_lambda_function_url")
		assert.Contains(t, outputStdout, "dev_s3_bucket_name")
		assert.Contains(t, outputStdout, "prod_lambda_function_url")
		assert.Contains(t, outputStdout, "prod_s3_bucket_name")

		// Verify that we can get the function URLs for both environments
		devFunctionURL, _ := helpers.ExecAndCaptureOutput(t, liveDir, "tofu", "output", "-raw", "dev_lambda_function_url")
		prodFunctionURL, _ := helpers.ExecAndCaptureOutput(t, liveDir, "tofu", "output", "-raw", "prod_lambda_function_url")

		require.NotEmpty(t, strings.TrimSpace(devFunctionURL))
		require.NotEmpty(t, strings.TrimSpace(prodFunctionURL))

		// Verify the URLs are different (confirming we have two separate environments)
		assert.NotEqual(t, strings.TrimSpace(devFunctionURL), strings.TrimSpace(prodFunctionURL))

		t.Log("Step 3 - Adding dev completed successfully")
		pass = true
	}()

	func() {
		t.Log("Running step 4 - Breaking the Terralith")

		// We do a check like this to make sure we properly clean up infrastructure only when we fail.
		//
		// We need our infrastructure to persist between steps so that we can test stateful refactoring.
		pass := false
		defer func() {
			if !pass {
				// Cleanup both dev and prod environments if we get here.
				if _, err := os.Stat(devDir); err == nil {
					helpers.ExecWithTestLogger(t, devDir, "tofu", "destroy", "-auto-approve")
				}

				if _, err := os.Stat(prodDir); err == nil {
					helpers.ExecWithTestLogger(t, prodDir, "tofu", "destroy", "-auto-approve")
				}
			}
		}()

		helpers.ExecWithTestLogger(t, liveDir, "mkdir", "prod")

		// Get list of files/directories to move (everything except the newly created prod directory)
		entries, err := os.ReadDir(liveDir)
		require.NoError(t, err)

		for _, entry := range entries {
			if entry.Name() != "prod" {
				oldPath := filepath.Join(liveDir, entry.Name())
				newPath := filepath.Join(prodDir, entry.Name())
				require.NoError(t, os.Rename(oldPath, newPath))
			}
		}

		helpers.ExecWithTestLogger(t, liveDir, "cp", "-R", "prod", "dev")

		fixtureStepPath := filepath.Join(fixturePath, "walkthrough", "step-4-breaking-the-terralith")

		devBackendPath := filepath.Join(devDir, "backend.tf")
		devBackendContent := fmt.Sprintf(`terraform {
  backend "s3" {
    bucket       = "%s"
    key          = "dev/tofu.tfstate"
    region       = "%s"
    encrypt      = true
    use_lockfile = true
  }
}
`, stateBucketName, region)
		require.NoError(t, os.WriteFile(devBackendPath, []byte(devBackendContent), 0644))

		prodBackendPath := filepath.Join(prodDir, "backend.tf")
		prodBackendContent := fmt.Sprintf(`terraform {
  backend "s3" {
    bucket       = "%s"
    key          = "prod/tofu.tfstate"
    region       = "%s"
    encrypt      = true
    use_lockfile = true
  }
}
`, stateBucketName, region)
		require.NoError(t, os.WriteFile(prodBackendPath, []byte(prodBackendContent), 0644))

		devMainSourcePath := filepath.Join(fixtureStepPath, "live", "dev")
		prodMainSourcePath := filepath.Join(fixtureStepPath, "live", "prod")

		// Files to copy/update in both directories
		envFiles := []string{
			"main.tf",
			"moved.tf",
			"outputs.tf",
			"removed.tf",
		}

		// Copy files to dev environment
		for _, file := range envFiles {
			helpers.CopyFile(
				t,
				filepath.Join(devMainSourcePath, file),
				filepath.Join(devDir, file),
			)
		}

		// Copy files to prod environment
		for _, file := range envFiles {
			helpers.CopyFile(
				t,
				filepath.Join(prodMainSourcePath, file),
				filepath.Join(prodDir, file),
			)
		}

		devTfvarsContent := fmt.Sprintf(`# Required: Name used for all resources (must be unique)
name = "%s-dev"

# Required: Path to your Lambda function zip file
lambda_zip_file = "../../dist/best-cat.zip"

# AWS region
aws_region = "%s"

force_destroy = true
`, name, region)

		prodTfvarsContent := fmt.Sprintf(`# Required: Name used for all resources (must be unique)
name = "%s"

# Required: Path to your Lambda function zip file
lambda_zip_file = "../../dist/best-cat.zip"

# AWS region
aws_region = "%s"

force_destroy = true
`, name, region)

		require.NoError(t, os.WriteFile(
			filepath.Join(devDir, ".auto.tfvars"),
			[]byte(devTfvarsContent),
			0644,
		))

		require.NoError(t, os.WriteFile(
			filepath.Join(prodDir, ".auto.tfvars"),
			[]byte(prodTfvarsContent),
			0644,
		))

		helpers.ExecWithTestLogger(
			t, liveDir, "cp", "-R",
			filepath.Join("prod", ".terraform"),
			filepath.Join("dev", ".terraform"),
		)

		// We can't use non-interactive mode here, so we just pipe in "yes" to the prompts.
		helpers.ExecWithTestLogger(t, devDir, "bash", "-c", "echo 'yes' | tofu init -migrate-state")
		helpers.ExecWithTestLogger(t, prodDir, "bash", "-c", "echo 'yes' | tofu init -migrate-state")

		devPlanOutput, _ := helpers.ExecAndCaptureOutput(t, devDir, "tofu", "plan")
		assert.Contains(t, devPlanOutput, "0 to add")
		assert.Contains(t, devPlanOutput, "1 to change")
		assert.Contains(t, devPlanOutput, "0 to destroy")
		assert.Contains(t, devPlanOutput, "11 to forget")

		helpers.ExecWithTestLogger(t, devDir, "tofu", "apply", "-auto-approve")

		prodPlanOutput, _ := helpers.ExecAndCaptureOutput(t, prodDir, "tofu", "plan")
		assert.Contains(t, prodPlanOutput, "0 to add")
		assert.Contains(t, prodPlanOutput, "1 to change")
		assert.Contains(t, prodPlanOutput, "0 to destroy")
		assert.Contains(t, prodPlanOutput, "11 to forget")

		helpers.ExecWithTestLogger(t, prodDir, "tofu", "apply", "-auto-approve")

		devOutputStdout, _ := helpers.ExecAndCaptureOutput(t, devDir, "tofu", "output")
		prodOutputStdout, _ := helpers.ExecAndCaptureOutput(t, prodDir, "tofu", "output")

		assert.Contains(t, devOutputStdout, "lambda_function_url")
		assert.Contains(t, devOutputStdout, "s3_bucket_name")
		assert.Contains(t, prodOutputStdout, "lambda_function_url")
		assert.Contains(t, prodOutputStdout, "s3_bucket_name")

		devFunctionURL, _ := helpers.ExecAndCaptureOutput(t, devDir, "tofu", "output", "-raw", "lambda_function_url")
		prodFunctionURL, _ := helpers.ExecAndCaptureOutput(t, prodDir, "tofu", "output", "-raw", "lambda_function_url")

		assert.NotEqual(t, strings.TrimSpace(devFunctionURL), strings.TrimSpace(prodFunctionURL))

		devBucketName, _ := helpers.ExecAndCaptureOutput(t, devDir, "tofu", "output", "-raw", "s3_bucket_name")
		prodBucketName, _ := helpers.ExecAndCaptureOutput(t, prodDir, "tofu", "output", "-raw", "s3_bucket_name")

		assert.NotEqual(t, strings.TrimSpace(devBucketName), strings.TrimSpace(prodBucketName))

		t.Log("Step 4 - Breaking the Terralith completed successfully")
		pass = true
	}()

	func() {
		t.Log("Running step 5 - Adding Terragrunt")

		// We do a check like this to make sure we properly clean up infrastructure only when we fail.
		//
		// We need our infrastructure to persist between steps so that we can test stateful refactoring.
		pass := false
		defer func() {
			if !pass {
				// Cleanup both dev and prod environments if we get here.
				if _, err := os.Stat(devDir); err == nil {
					helpers.ExecWithTestLogger(t, devDir, "terragrunt", "destroy", "-auto-approve")
				}

				if _, err := os.Stat(prodDir); err == nil {
					helpers.ExecWithTestLogger(t, prodDir, "terragrunt", "destroy", "-auto-approve")
				}
			}
		}()

		fixtureStepPath := filepath.Join(fixturePath, "walkthrough", "step-5-adding-terragrunt")

		require.NoError(t, os.WriteFile(filepath.Join(devDir, "terragrunt.hcl"), []byte(""), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(prodDir, "terragrunt.hcl"), []byte(""), 0644))

		_, stderr := helpers.ExecAndCaptureOutput(t, liveDir, "terragrunt", "run", "--all", "plan", "--non-interactive")

		// This version of Terragrunt uses the new "Module" term instead of "Unit"
		assert.Contains(t, stderr, "Module ./dev")
		assert.Contains(t, stderr, "Module ./prod")

		oldFiles := []string{"main.tf", "outputs.tf", "vars-required.tf", "vars-optional.tf", "versions.tf"}
		for _, file := range oldFiles {
			require.NoError(t, os.Remove(filepath.Join(devDir, file)))
			require.NoError(t, os.Remove(filepath.Join(prodDir, file)))
		}

		require.NoError(t, os.Remove(filepath.Join(devDir, ".auto.tfvars")))
		require.NoError(t, os.Remove(filepath.Join(prodDir, ".auto.tfvars")))

		require.NoError(t, os.Remove(filepath.Join(devDir, "backend.tf")))
		require.NoError(t, os.Remove(filepath.Join(prodDir, "backend.tf")))
		require.NoError(t, os.Remove(filepath.Join(devDir, "providers.tf")))
		require.NoError(t, os.Remove(filepath.Join(prodDir, "providers.tf")))

		require.NoError(t, os.Remove(filepath.Join(devDir, "removed.tf")))
		require.NoError(t, os.Remove(filepath.Join(prodDir, "removed.tf")))

		rootHclContent := fmt.Sprintf(`remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    bucket       = "%s"
    key          = "${path_relative_to_include()}/tofu.tfstate"
    region       = "%s"
    encrypt      = true
    use_lockfile = true
  }
}

generate "providers" {
  path      = "providers.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
provider "aws" {
  region = "%s"
}
EOF
}
`, stateBucketName, region, region)

		require.NoError(t, os.WriteFile(
			filepath.Join(liveDir, "root.hcl"),
			[]byte(rootHclContent),
			0644,
		))

		// Create dev terragrunt.hcl
		devTerragruntContent := fmt.Sprintf(`include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "../../catalog/modules//best_cat"
}

inputs = {
  name            = "%s-dev"
  lambda_zip_file = "${get_repo_root()}/dist/best-cat.zip"

  force_destroy = true
}
`, name)

		require.NoError(t, os.WriteFile(
			filepath.Join(devDir, "terragrunt.hcl"),
			[]byte(devTerragruntContent),
			0644,
		))

		// Create prod terragrunt.hcl
		prodTerragruntContent := fmt.Sprintf(`include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "../../catalog/modules//best_cat"
}

inputs = {
  name            = "%s"
  lambda_zip_file = "${get_repo_root()}/dist/best-cat.zip"

  force_destroy = true
}
`, name)

		require.NoError(t, os.WriteFile(
			filepath.Join(prodDir, "terragrunt.hcl"),
			[]byte(prodTerragruntContent),
			0644,
		))

		helpers.CopyFile(
			t,
			filepath.Join(fixtureStepPath, "live", "dev", "moved.tf"),
			filepath.Join(devDir, "moved.tf"),
		)

		helpers.CopyFile(
			t,
			filepath.Join(fixtureStepPath, "live", "prod", "moved.tf"),
			filepath.Join(prodDir, "moved.tf"),
		)

		devPlanOutput, _ := helpers.ExecAndCaptureOutput(t, devDir, "terragrunt", "plan")
		assert.Contains(t, devPlanOutput, "0 to add, 1 to change, 0 to destroy")

		prodPlanOutput, _ := helpers.ExecAndCaptureOutput(t, prodDir, "terragrunt", "plan")
		assert.Contains(t, prodPlanOutput, "0 to add, 1 to change, 0 to destroy")

		helpers.ExecWithTestLogger(t, devDir, "terragrunt", "apply", "-auto-approve")
		helpers.ExecWithTestLogger(t, prodDir, "terragrunt", "apply", "-auto-approve")

		runAllPlanStdout, runAllPlanStderr := helpers.ExecAndCaptureOutput(t, liveDir, "terragrunt", "run", "--all", "plan")
		assert.Contains(t, runAllPlanStderr, "Module ./dev")
		assert.Contains(t, runAllPlanStderr, "Module ./prod")
		assert.Contains(t, runAllPlanStdout, "found no differences, so no changes are needed.")

		devOnlyPlanStdout, devOnlyPlanStderr := helpers.ExecAndCaptureOutput(t, liveDir, "terragrunt", "run", "--all", "--queue-include-dir", "dev", "plan", "--non-interactive")
		assert.Contains(t, devOnlyPlanStderr, "Module ./dev")
		assert.NotContains(t, devOnlyPlanStderr, "Module ./prod")
		assert.Contains(t, devOnlyPlanStdout, "found no differences, so no changes are needed.")

		devOutputStdout, _ := helpers.ExecAndCaptureOutput(t, devDir, "terragrunt", "output")
		prodOutputStdout, _ := helpers.ExecAndCaptureOutput(t, prodDir, "terragrunt", "output")

		assert.Contains(t, devOutputStdout, "lambda_function_url")
		assert.Contains(t, devOutputStdout, "s3_bucket_name")
		assert.Contains(t, prodOutputStdout, "lambda_function_url")
		assert.Contains(t, prodOutputStdout, "s3_bucket_name")

		t.Log("Step 5 - Adding Terragrunt completed successfully")
		pass = true
	}()

	func() {
		t.Log("Running step 6 - Breaking the Terralith Further")

		// We do a check like this to make sure we properly clean up infrastructure only when we fail.
		//
		// We need our infrastructure to persist between steps so that we can test stateful refactoring.
		pass := false
		defer func() {
			if !pass {
				// Cleanup all component units if we get here.
				helpers.ExecWithTestLogger(t, liveDir, "terragrunt", "run", "--all", "--non-interactive", "destroy")
			}
		}()

		fixtureStepPath := filepath.Join(fixturePath, "walkthrough", "step-6-breaking-the-terralith-further")

		// Create directories for each component in both environments
		components := []string{"s3", "ddb", "iam", "lambda"}
		environments := []string{"dev", "prod"}

		for _, env := range environments {
			for _, component := range components {
				componentDir := filepath.Join(liveDir, env, component)
				require.NoError(t, os.MkdirAll(componentDir, 0755))
			}
		}

		// Copy terragrunt.hcl files from fixtures for each component
		for _, env := range environments {
			for _, component := range components {
				sourceFile := filepath.Join(fixtureStepPath, "live", env, component, "terragrunt.hcl")
				destFile := filepath.Join(liveDir, env, component, "terragrunt.hcl")

				// Read the source file and replace the hardcoded name with our test name
				sourceContent, err := os.ReadFile(sourceFile)
				require.NoError(t, err)

				// Replace the hardcoded name in the fixture with our test-specific name
				nameToUse := name
				if env == "dev" {
					nameToUse = name + "-dev"
				}

				content := strings.ReplaceAll(string(sourceContent), "best-cat-2025-07-31-01", name)
				content = strings.ReplaceAll(content, name+"-dev", nameToUse)
				content = strings.ReplaceAll(content, name, nameToUse)

				require.NoError(t, os.WriteFile(destFile, []byte(content), 0644))
			}
		}

		// Migrate state from existing units to new component units
		// First, pull state from existing dev and prod units
		for _, env := range environments {
			envDir := filepath.Join(liveDir, env)

			tmpDir := t.TempDir()
			tempStateFile := filepath.Join(tmpDir, "tofu-"+env+".tfstate")

			// Pull state from existing environment unit
			stateContent, _ := helpers.ExecAndCaptureOutput(t, envDir, "terragrunt", "state", "pull")
			require.NoError(t, os.WriteFile(tempStateFile, []byte(stateContent), 0644))

			for _, component := range components {
				componentDir := filepath.Join(envDir, component)
				helpers.ExecWithTestLogger(t, componentDir, "terragrunt", "state", "push", tempStateFile)
			}
		}

		require.NoError(t, os.WriteFile(filepath.Join(liveDir, "dev", "s3", "terragrunt.hcl"), fmt.Appendf(nil, `include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${find_in_parent_folders("catalog/modules")}//s3"
}

inputs = {
  name = "%s-dev"
  force_destroy = true
}
`, name), 0644))

		require.NoError(t, os.WriteFile(filepath.Join(liveDir, "prod", "s3", "terragrunt.hcl"), fmt.Appendf(nil, `include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${find_in_parent_folders("catalog/modules")}//s3"
}

inputs = {
  name = "%s"
  force_destroy = true
}
`, name), 0644))

		// Remove the old terragrunt.hcl and moved.tf files from the environment root directories
		for _, env := range environments {
			envDir := filepath.Join(liveDir, env)
			require.NoError(t, os.Remove(filepath.Join(envDir, "terragrunt.hcl")))
			require.NoError(t, os.Remove(filepath.Join(envDir, "moved.tf")))
		}

		// Copy moved.tf and removed.tf files for state transitions
		for _, env := range environments {
			for _, component := range components {
				sourceMovedFile := filepath.Join(fixtureStepPath, "live", env, component, "moved.tf")
				destMovedFile := filepath.Join(liveDir, env, component, "moved.tf")
				helpers.CopyFile(t, sourceMovedFile, destMovedFile)

				sourceRemovedFile := filepath.Join(fixtureStepPath, "live", env, component, "removed.tf")
				destRemovedFile := filepath.Join(liveDir, env, component, "removed.tf")
				helpers.CopyFile(t, sourceRemovedFile, destRemovedFile)
			}
		}

		// Verify plans show no destroys across all components
		_, planStderr := helpers.ExecAndCaptureOutput(t, liveDir, "terragrunt", "run", "--all", "plan", "--non-interactive")

		// The plan output should show modules for all components
		for _, env := range environments {
			for _, component := range components {
				expectedModulePath := fmt.Sprintf("Module ./%s/%s", env, component)
				assert.Contains(t, planStderr, expectedModulePath)
			}
		}

		// Apply all changes to complete the migration
		helpers.ExecWithTestLogger(t, liveDir, "terragrunt", "run", "--all", "apply", "--non-interactive")

		// Verify outputs still work after breaking down into components
		// Check a few key components to ensure they're working
		devS3Output, _ := helpers.ExecAndCaptureOutput(t, filepath.Join(liveDir, "dev", "s3"), "terragrunt", "output")
		prodS3Output, _ := helpers.ExecAndCaptureOutput(t, filepath.Join(liveDir, "prod", "s3"), "terragrunt", "output")

		assert.Contains(t, devS3Output, "name")
		assert.Contains(t, prodS3Output, "name")

		devLambdaOutput, _ := helpers.ExecAndCaptureOutput(t, filepath.Join(liveDir, "dev", "lambda"), "terragrunt", "output")
		prodLambdaOutput, _ := helpers.ExecAndCaptureOutput(t, filepath.Join(liveDir, "prod", "lambda"), "terragrunt", "output")

		assert.Contains(t, devLambdaOutput, "url")
		assert.Contains(t, prodLambdaOutput, "url")

		// Verify dependency resolution works by running a plan on lambda (which depends on other components)
		devLambdaPlan, _ := helpers.ExecAndCaptureOutput(t, filepath.Join(liveDir, "dev", "lambda"), "terragrunt", "plan")
		assert.Contains(t, devLambdaPlan, "found no differences, so no changes are needed.")

		prodLambdaPlan, _ := helpers.ExecAndCaptureOutput(t, filepath.Join(liveDir, "prod", "lambda"), "terragrunt", "plan")
		assert.Contains(t, prodLambdaPlan, "found no differences, so no changes are needed.")

		t.Log("Step 6 - Breaking the Terralith Further completed successfully")
		pass = true
	}()

	func() {
		t.Log("Cleanup")

		helpers.ExecWithTestLogger(t, liveDir, "terragrunt", "run", "--all", "--non-interactive", "destroy")
	}()
}
