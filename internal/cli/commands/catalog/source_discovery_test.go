package catalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tmpDirResolved returns a temp directory with symlinks resolved.
// This avoids macOS issues where t.TempDir() returns a symlinked path.
func tmpDirResolved(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	return resolved
}

func TestExtractTerraformSource_Simple(t *testing.T) {
	t.Parallel()

	content := `
terraform {
  source = "github.com/gruntwork-io/terraform-aws-vpc"
}
`
	result := extractTerraformSource(content)
	assert.Equal(t, "github.com/gruntwork-io/terraform-aws-vpc", result)
}

func TestExtractTerraformSource_WithSubdirAndRef(t *testing.T) {
	t.Parallel()

	content := `
terraform {
  source = "github.com/gruntwork-io/terraform-aws-vpc//modules/vpc-app?ref=v0.26.0"
}
`
	result := extractTerraformSource(content)
	assert.Equal(t, "github.com/gruntwork-io/terraform-aws-vpc//modules/vpc-app?ref=v0.26.0", result)
}

func TestExtractTerraformSource_GitPrefix(t *testing.T) {
	t.Parallel()

	content := `
terraform {
  source = "git::https://github.com/gruntwork-io/terraform-aws-vpc.git//modules/vpc?ref=v1.0.0"
}
`
	result := extractTerraformSource(content)
	assert.Equal(t, "git::https://github.com/gruntwork-io/terraform-aws-vpc.git//modules/vpc?ref=v1.0.0", result)
}

func TestExtractTerraformSource_Interpolated(t *testing.T) {
	t.Parallel()

	content := `
terraform {
  source = "${local.base_source_url}?ref=v0.26.0"
}
`
	result := extractTerraformSource(content)
	assert.Empty(t, result)
}

func TestExtractTerraformSource_NoTerraformBlock(t *testing.T) {
	t.Parallel()

	content := `
include "root" {
  path = find_in_parent_folders()
}

inputs = {
  name = "my-vpc"
}
`
	result := extractTerraformSource(content)
	assert.Empty(t, result)
}

func TestExtractTerraformSource_WithOtherAttributes(t *testing.T) {
	t.Parallel()

	content := `
terraform {
  source = "github.com/gruntwork-io/terraform-aws-vpc//modules/vpc-app?ref=v0.26.0"

  extra_arguments "common" {
    arguments = ["-var-file=common.tfvars"]
  }
}
`
	result := extractTerraformSource(content)
	assert.Equal(t, "github.com/gruntwork-io/terraform-aws-vpc//modules/vpc-app?ref=v0.26.0", result)
}

func TestExtractRepoURL_Simple(t *testing.T) {
	t.Parallel()

	result := extractRepoURL("github.com/gruntwork-io/terraform-aws-vpc")
	assert.Equal(t, "github.com/gruntwork-io/terraform-aws-vpc", result)
}

func TestExtractRepoURL_WithSubdirAndRef(t *testing.T) {
	t.Parallel()

	result := extractRepoURL("github.com/gruntwork-io/terraform-aws-vpc//modules/vpc-app?ref=v0.26.0")
	assert.Equal(t, "github.com/gruntwork-io/terraform-aws-vpc", result)
}

func TestExtractRepoURL_GitPrefix(t *testing.T) {
	t.Parallel()

	result := extractRepoURL("git::https://github.com/gruntwork-io/terraform-aws-vpc.git")
	assert.Equal(t, "https://github.com/gruntwork-io/terraform-aws-vpc.git", result)
}

func TestExtractRepoURL_GitPrefixWithSubdir(t *testing.T) {
	t.Parallel()

	result := extractRepoURL("git::https://github.com/gruntwork-io/terraform-aws-vpc.git//modules/vpc?ref=v1.0.0")
	assert.Equal(t, "https://github.com/gruntwork-io/terraform-aws-vpc.git", result)
}

func TestExtractRepoURL_LocalPath(t *testing.T) {
	t.Parallel()

	assert.Empty(t, extractRepoURL("../modules/vpc"))
	assert.Empty(t, extractRepoURL("./modules/vpc"))
	assert.Empty(t, extractRepoURL("/absolute/path/to/modules"))
}

func TestExtractRepoURL_Registry(t *testing.T) {
	t.Parallel()

	result := extractRepoURL("tfr:///terraform-aws-modules/vpc/aws?version=3.5.0")
	assert.Empty(t, result)
}

func TestExtractRepoURL_S3Prefix(t *testing.T) {
	t.Parallel()

	result := extractRepoURL("s3::https://s3-eu-west-1.amazonaws.com/bucket/module.zip")
	assert.Equal(t, "https://s3-eu-west-1.amazonaws.com/bucket/module.zip", result)
}

func TestDiscoverSourceURLs(t *testing.T) {
	t.Parallel()

	tmpDir := tmpDirResolved(t)

	// Create directory structure:
	// tmpDir/
	//   unit-a/terragrunt.hcl  -> source = "github.com/org/repo-a//modules/vpc?ref=v1.0"
	//   unit-b/terragrunt.hcl  -> source = "github.com/org/repo-b"
	//   unit-c/terragrunt.hcl  -> source = "github.com/org/repo-a//modules/ecs?ref=v2.0" (same repo as unit-a)
	//   unit-d/terragrunt.hcl  -> source = "${local.base}?ref=v1" (interpolated, should be skipped)
	//   unit-e/terragrunt.hcl  -> no terraform block (should be skipped)

	unitA := filepath.Join(tmpDir, "unit-a")
	unitB := filepath.Join(tmpDir, "unit-b")
	unitC := filepath.Join(tmpDir, "unit-c")
	unitD := filepath.Join(tmpDir, "unit-d")
	unitE := filepath.Join(tmpDir, "unit-e")

	for _, dir := range []string{unitA, unitB, unitC, unitD, unitE} {
		require.NoError(t, os.MkdirAll(dir, 0755))
	}

	require.NoError(t, os.WriteFile(filepath.Join(unitA, "terragrunt.hcl"), []byte(`
terraform {
  source = "github.com/org/repo-a//modules/vpc?ref=v1.0"
}
`), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(unitB, "terragrunt.hcl"), []byte(`
terraform {
  source = "github.com/org/repo-b"
}
`), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(unitC, "terragrunt.hcl"), []byte(`
terraform {
  source = "github.com/org/repo-a//modules/ecs?ref=v2.0"
}
`), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(unitD, "terragrunt.hcl"), []byte(`
terraform {
  source = "${local.base_source_url}?ref=v1.0"
}
`), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(unitE, "terragrunt.hcl"), []byte(`
include "root" {
  path = find_in_parent_folders()
}

inputs = {
  name = "test"
}
`), 0644))

	urls, err := discoverSourceURLs(tmpDir, experiment.NewExperiments())
	require.NoError(t, err)

	// Should have 2 unique repo URLs (repo-a deduplicated, repo-b, interpolated and no-source skipped)
	assert.Len(t, urls, 2)
	assert.Contains(t, urls, "github.com/org/repo-a")
	assert.Contains(t, urls, "github.com/org/repo-b")
}

func TestDiscoverSourceURLs_EmptyDir(t *testing.T) {
	t.Parallel()

	tmpDir := tmpDirResolved(t)

	urls, err := discoverSourceURLs(tmpDir, experiment.NewExperiments())
	require.NoError(t, err)
	assert.Empty(t, urls)
}
