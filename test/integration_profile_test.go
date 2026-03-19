//nolint:paralleltest
package test_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTGCPUProfileCreatesProfileFile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "cpu.prof")
	expectedFile := filepath.Join(tmpDir, "terragrunt_cpu.prof")

	t.Setenv("TG_CPU_PROFILE", profilePath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt version")
	require.NoError(t, err)

	info, err := os.Stat(expectedFile)
	require.NoError(t, err, "CPU profile file %s should exist", expectedFile)
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
	profilePath := filepath.Join(tmpDir, "cpu.prof")

	t.Setenv("TG_CPU_PROFILE", profilePath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt version")
	require.NoError(t, err)

	// Verify terragrunt profile was created
	expectedFile := filepath.Join(tmpDir, "terragrunt_cpu.prof")
	info, err := os.Stat(expectedFile)
	require.NoError(t, err, "CPU profile file should exist")
	assert.Positive(t, info.Size(), "CPU profile file should be non-empty")
}

func TestTGCPUProfileDoesNotOverrideExplicitTofuCPUProfile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "cpu.prof")

	t.Setenv("TG_CPU_PROFILE", profilePath)
	t.Setenv("TOFU_CPU_PROFILE", "/custom/tofu-path.prof")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt version")
	require.NoError(t, err)

	// Verify terragrunt profile was still created
	expectedFile := filepath.Join(tmpDir, "terragrunt_cpu.prof")
	info, err := os.Stat(expectedFile)
	require.NoError(t, err, "CPU profile file should exist")
	assert.Positive(t, info.Size(), "CPU profile file should be non-empty")
}
