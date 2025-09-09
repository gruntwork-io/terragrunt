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

	repoPath := filepath.Join(tmpDir, "terralith-to-terragrunt")

	t.Run("setup", func(t *testing.T) {
		helpers.ExecWithTestLogger(t, repoPath, "git", "init")

		helpers.ExecWithTestLogger(t, repoPath, "mise", "use", "terragrunt@0.83.2")
		helpers.ExecWithTestLogger(t, repoPath, "mise", "use", "opentofu@1.10.3")
		helpers.ExecWithTestLogger(t, repoPath, "mise", "use", "aws@2.27.63")
		helpers.ExecWithTestLogger(t, repoPath, "mise", "use", "node@22.17.1")

		miseTomlPath := util.JoinPath(repoPath, "mise.toml")
		require.FileExists(t, miseTomlPath)
		miseToml, err := os.ReadFile(miseTomlPath)
		require.NoError(t, err)

		assert.Equal(t, string(miseToml), `[tools]
aws = "2.27.63"
node = "22.17.1"
opentofu = "1.10.3"
terragrunt = "0.83.2"
`)

		helpers.ExecWithTestLogger(t, repoPath, "mkdir", "-p", "app/best-cat")

		bestCatPath := filepath.Join(repoPath, "app", "best-cat")

		helpers.CopyFile(
			t, filepath.Join(
				fixturePath,
				"app",
				"best-cat",
				"package.json",
			), filepath.Join(bestCatPath, "package.json"),
		)

		helpers.CopyFile(
			t, filepath.Join(
				fixturePath,
				"app",
				"best-cat",
				"index.js",
			), filepath.Join(bestCatPath, "index.js"),
		)

		helpers.CopyFile(
			t, filepath.Join(
				fixturePath,
				"app",
				"best-cat",
				"template.html",
			), filepath.Join(bestCatPath, "template.html"),
		)

		helpers.CopyFile(
			t, filepath.Join(
				fixturePath,
				"app",
				"best-cat",
				"styles.css",
			), filepath.Join(bestCatPath, "styles.css"),
		)

		helpers.CopyFile(
			t, filepath.Join(
				fixturePath,
				"app",
				"best-cat",
				"script.js",
			), filepath.Join(bestCatPath, "script.js"),
		)

		helpers.CopyFile(
			t, filepath.Join(
				fixturePath,
				"app",
				"best-cat",
				"package-lock.json",
			), filepath.Join(bestCatPath, "package-lock.json"),
		)

		helpers.ExecWithTestLogger(t, repoPath, "mkdir", "dist")

		helpers.ExecWithTestLogger(t, bestCatPath, "npm", "i")
		helpers.ExecWithTestLogger(t, bestCatPath, "npm", "run", "package")

		// Create some fake files in the dist directory to mock assets.
		distDir := filepath.Join(repoPath, "dist")

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
	})
}
