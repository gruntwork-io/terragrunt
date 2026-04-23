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

	helpers.RunTerragrunt(t, "terragrunt version")

	info, err := os.Stat(profilePath)
	require.NoError(t, err, "CPU profile file %s should exist", profilePath)
	assert.Positive(t, info.Size(), "CPU profile file should be non-empty")
}

func TestTGCPUProfileNotSetNoFile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)

	helpers.RunTerragrunt(t, "terragrunt version")

	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "No profile files should exist when TG_CPU_PROFILE is not set")
}

func TestTGCPUProfileSetsTofu(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "terragrunt_cpu.prof")

	t.Setenv("TG_CPU_PROFILE", profilePath)

	helpers.RunTerragrunt(t, "terragrunt version")

	info, err := os.Stat(profilePath)
	require.NoError(t, err, "CPU profile file should exist")
	assert.Positive(t, info.Size(), "CPU profile file should be non-empty")
}

func TestTGCPUProfileDoesNotOverrideExplicitTofuCPUProfile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "terragrunt_cpu.prof")

	t.Setenv("TG_CPU_PROFILE", profilePath)
	t.Setenv("TOFU_CPU_PROFILE", "/custom/tofu-path.prof")

	helpers.RunTerragrunt(t, "terragrunt version")

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

	helpers.RunTerragrunt(t, "terragrunt plan --non-interactive --working-dir "+rootPath)

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

func TestTGCPUProfileDirCollectsProfiles(t *testing.T) {
	if isTerraform() {
		t.Skip("TOFU_CPU_PROFILE is only supported by OpenTofu")
	}

	helpers.CleanupTerraformFolder(t, testFixturePprofInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixturePprofInputs)
	rootPath := filepath.Join(tmpEnvPath, testFixturePprofInputs)

	profileDir := filepath.Join(helpers.TmpDirWOSymlinks(t), "profiles")
	t.Setenv("TG_CPU_PROFILE_DIR", profileDir)

	helpers.RunTerragrunt(t, "terragrunt plan --non-interactive --working-dir "+rootPath)

	// Verify terragrunt profile was auto-created
	tgProfile := filepath.Join(profileDir, "terragrunt_cpu.prof")
	require.True(t, util.FileExists(tgProfile),
		"Terragrunt CPU profile should exist at %s", tgProfile)

	tgInfo, err := os.Stat(tgProfile)
	require.NoError(t, err)
	assert.Positive(t, tgInfo.Size(), "Terragrunt CPU profile should be non-empty")

	// Verify tofu profile was created in the profile dir
	tofuProfile := filepath.Join(profileDir, "tofu_cpu.prof")
	require.True(t, util.FileExists(tofuProfile),
		"OpenTofu CPU profile should exist at %s", tofuProfile)
}

func TestTGCPUProfileDirWithExplicitTGCPUProfile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profileDir := filepath.Join(tmpDir, "profiles")
	customProfile := filepath.Join(tmpDir, "my_custom.prof")

	t.Setenv("TG_CPU_PROFILE_DIR", profileDir)
	t.Setenv("TG_CPU_PROFILE", customProfile)

	helpers.RunTerragrunt(t, "terragrunt version")

	// TG_CPU_PROFILE takes precedence over the default
	info, err := os.Stat(customProfile)
	require.NoError(t, err, "Custom CPU profile should exist at %s", customProfile)
	assert.Positive(t, info.Size(), "Custom CPU profile should be non-empty")

	// Default terragrunt_cpu.prof should NOT exist since TG_CPU_PROFILE was explicit
	defaultProfile := filepath.Join(profileDir, "terragrunt_cpu.prof")
	assert.False(t, util.FileExists(defaultProfile),
		"Default profile should not exist when TG_CPU_PROFILE is explicitly set")
}

func TestTGMemProfileCreatesProfileFile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "terragrunt_mem.prof")

	t.Setenv("TG_MEM_PROFILE", profilePath)

	helpers.RunTerragrunt(t, "terragrunt version")

	info, err := os.Stat(profilePath)
	require.NoError(t, err, "Memory profile file %s should exist", profilePath)
	assert.Positive(t, info.Size(), "Memory profile file should be non-empty")
}

func TestTGMemProfileDirCollectsProfile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profileDir := filepath.Join(tmpDir, "profiles")

	t.Setenv("TG_MEM_PROFILE_DIR", profileDir)

	helpers.RunTerragrunt(t, "terragrunt version")

	memProfile := filepath.Join(profileDir, "terragrunt_mem.prof")
	require.True(t, util.FileExists(memProfile),
		"Memory profile should exist at %s", memProfile)

	info, err := os.Stat(memProfile)
	require.NoError(t, err)
	assert.Positive(t, info.Size(), "Memory profile should be non-empty")
}

func TestTGMemAndCPUProfileDirSameDirectory(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profileDir := filepath.Join(tmpDir, "profiles")

	t.Setenv("TG_CPU_PROFILE_DIR", profileDir)
	t.Setenv("TG_MEM_PROFILE_DIR", profileDir)

	helpers.RunTerragrunt(t, "terragrunt version")

	cpuProfile := filepath.Join(profileDir, "terragrunt_cpu.prof")
	memProfile := filepath.Join(profileDir, "terragrunt_mem.prof")

	require.True(t, util.FileExists(cpuProfile), "CPU profile should exist")
	require.True(t, util.FileExists(memProfile), "Memory profile should exist")

	cpuInfo, err := os.Stat(cpuProfile)
	require.NoError(t, err)
	assert.Positive(t, cpuInfo.Size())

	memInfo, err := os.Stat(memProfile)
	require.NoError(t, err)
	assert.Positive(t, memInfo.Size())
}
