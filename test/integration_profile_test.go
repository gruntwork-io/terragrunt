//nolint:paralleltest
package test_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFixturePprofInputs = "fixtures/inputs"

func TestTGCPUProfileCreatesProfileFile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "terragrunt_cpu.prof")

	t.Setenv("TG_CPU_PROFILE", profilePath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt version")
	require.NoError(t, err)

	info, err := os.Stat(profilePath)
	require.NoError(t, err, "CPU profile file %s should exist", profilePath)
	assert.Positive(t, info.Size(), "CPU profile file should be non-empty")
}

func TestTGCPUProfileNotSetNoFile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt version")
	require.NoError(t, err)

	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "No profile files should exist when TG_CPU_PROFILE is not set")
}

func TestTGCPUProfileSetsTofu(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "terragrunt_cpu.prof")

	t.Setenv("TG_CPU_PROFILE", profilePath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt version")
	require.NoError(t, err)

	info, err := os.Stat(profilePath)
	require.NoError(t, err, "CPU profile file should exist")
	assert.Positive(t, info.Size(), "CPU profile file should be non-empty")
}

func TestTGCPUProfileDoesNotOverrideExplicitTofuCPUProfile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "terragrunt_cpu.prof")

	t.Setenv("TG_CPU_PROFILE", profilePath)
	t.Setenv("TOFU_CPU_PROFILE", "/custom/tofu-path.prof")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt version")
	require.NoError(t, err)

	info, err := os.Stat(profilePath)
	require.NoError(t, err, "CPU profile file should exist")
	assert.Positive(t, info.Size(), "CPU profile file should be non-empty")
}

func TestTGCPUProfileDownstreamTofuProfile(t *testing.T) {
	if isTerraform() {
		t.Skip("TOFU_CPU_PROFILE is only supported by OpenTofu")
	}

	helpers.CleanupTerraformFolder(t, testFixturePprofInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePprofInputs)
	rootPath := filepath.Join(tmpEnvPath, testFixturePprofInputs)

	// Use relative path so tofu writes its profile inside its own cache working directory,
	// and terragrunt writes its profile in the current working directory.
	t.Setenv("TG_CPU_PROFILE", "terragrunt_cpu.prof")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	// Verify tofu profile was created in the cache working directory
	cacheWorkingDir := helpers.FindCacheWorkingDir(t, rootPath)
	require.NotEmpty(t, cacheWorkingDir, "should find cache working directory")

	tofuProfilePath := filepath.Join(cacheWorkingDir, "terragrunt_cpu.prof")
	require.True(t, util.FileExists(tofuProfilePath),
		"OpenTofu CPU profile should exist at %s", tofuProfilePath)

	info, err := os.Stat(tofuProfilePath)
	require.NoError(t, err)
	assert.Positive(t, info.Size(), "OpenTofu CPU profile should be non-empty")
}
