//nolint:paralleltest // Tests use t.Setenv, and CPU profiling is process-global (pprof.StartCPUProfile), so tests in this file must not run in parallel.
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

const testFixtureProfileMultiUnit = "fixtures/profile/multi-unit"

func TestTGProfileCPUCreatesProfileFile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "terragrunt_cpu.prof")

	t.Setenv("TG_EXPERIMENT", "profiling")
	t.Setenv("TG_PROFILE_CPU", profilePath)

	helpers.RunTerragrunt(t, "terragrunt version")

	info, err := os.Stat(profilePath)
	require.NoError(t, err, "CPU profile file %s should exist", profilePath)
	assert.Positive(t, info.Size(), "CPU profile file should be non-empty")
}

func TestTGProfileCPUDoesNotPropagateToTofu(t *testing.T) {
	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputs)
	rootPath := filepath.Join(tmpEnvPath, testFixtureInputs)

	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "terragrunt_cpu.prof")

	t.Setenv("TG_EXPERIMENT", "profiling")
	t.Setenv("TG_PROFILE_CPU", profilePath)

	helpers.RunTerragrunt(t, "terragrunt plan --non-interactive --working-dir "+rootPath)

	info, err := os.Stat(profilePath)
	require.NoError(t, err, "Terragrunt CPU profile should exist")
	assert.Positive(t, info.Size(), "Terragrunt CPU profile should be non-empty")

	cacheWorkingDir := helpers.FindCacheWorkingDir(t, rootPath)
	profiles, err := filepath.Glob(filepath.Join(cacheWorkingDir, "*.prof"))
	require.NoError(t, err)
	assert.Empty(t, profiles, "TG_PROFILE_CPU alone should not produce OpenTofu profiles")
}

func TestTGProfileCPUDoesNotOverrideExplicitTofuCPUProfile(t *testing.T) {
	if isTerraform(t.Context()) {
		t.Skip("TOFU_CPU_PROFILE is only supported by OpenTofu")
	}

	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputs)
	rootPath := filepath.Join(tmpEnvPath, testFixtureInputs)

	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "terragrunt_cpu.prof")
	tofuProfilePath := filepath.Join(tmpDir, "tofu_cpu.prof")

	t.Setenv("TG_EXPERIMENT", "profiling")
	t.Setenv("TG_PROFILE_CPU", profilePath)
	t.Setenv("TOFU_CPU_PROFILE", tofuProfilePath)

	helpers.RunTerragrunt(t, "terragrunt plan --non-interactive --working-dir "+rootPath)

	info, err := os.Stat(profilePath)
	require.NoError(t, err, "CPU profile file should exist")
	assert.Positive(t, info.Size(), "CPU profile file should be non-empty")

	tofuInfo, err := os.Stat(tofuProfilePath)
	require.NoError(t, err, "Explicit OpenTofu CPU profile should exist")
	assert.Positive(t, tofuInfo.Size(), "Explicit OpenTofu CPU profile should be non-empty")
}

func TestTGProfileDirCollectsProfiles(t *testing.T) {
	if isTerraform(t.Context()) {
		t.Skip("TOFU_CPU_PROFILE is only supported by OpenTofu")
	}

	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputs)
	rootPath := filepath.Join(tmpEnvPath, testFixtureInputs)

	profileDir := filepath.Join(helpers.TmpDirWOSymlinks(t), "profiles")
	t.Setenv("TG_EXPERIMENT", "profiling")
	t.Setenv("TG_PROFILE_DIR", profileDir)

	helpers.RunTerragrunt(t, "terragrunt plan --non-interactive --working-dir "+rootPath)

	tgCPUProfile := filepath.Join(profileDir, "terragrunt_cpu.prof")
	require.True(t, util.FileExists(tgCPUProfile),
		"Terragrunt CPU profile should exist at %s", tgCPUProfile)

	tgInfo, err := os.Stat(tgCPUProfile)
	require.NoError(t, err)
	assert.Positive(t, tgInfo.Size(), "Terragrunt CPU profile should be non-empty")

	tofuProfile := filepath.Join(profileDir, "tofu_cpu.prof")
	require.True(t, util.FileExists(tofuProfile),
		"OpenTofu CPU profile should exist at %s", tofuProfile)

	memProfile := filepath.Join(profileDir, "terragrunt_mem.prof")
	require.True(t, util.FileExists(memProfile), "Memory profile should exist at %s", memProfile)

	goroutineProfile := filepath.Join(profileDir, "terragrunt_goroutine.prof")
	require.True(t, util.FileExists(goroutineProfile), "Goroutine profile should exist at %s", goroutineProfile)
}

func TestTGProfileDirPerUnitTofuProfiles(t *testing.T) {
	if isTerraform(t.Context()) {
		t.Skip("TOFU_CPU_PROFILE is only supported by OpenTofu")
	}

	helpers.CleanupTerraformFolder(t, testFixtureProfileMultiUnit)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureProfileMultiUnit)
	rootPath := filepath.Join(tmpEnvPath, testFixtureProfileMultiUnit)

	profileDir := filepath.Join(helpers.TmpDirWOSymlinks(t), "profiles")
	t.Setenv("TG_EXPERIMENT", "profiling")
	t.Setenv("TG_PROFILE_DIR", profileDir)

	helpers.RunTerragrunt(t, "terragrunt run --all plan --non-interactive --working-dir "+rootPath)

	for _, unit := range []string{"app1", "app2"} {
		tofuProfile := filepath.Join(profileDir, unit, "tofu_cpu.prof")
		require.True(t, util.FileExists(tofuProfile),
			"OpenTofu CPU profile for unit %s should exist at %s", unit, tofuProfile)

		info, err := os.Stat(tofuProfile)
		require.NoError(t, err)
		assert.Positive(t, info.Size(), "OpenTofu CPU profile for unit %s should be non-empty", unit)
	}
}

func TestTGProfileDirWithExplicitTGProfileCPU(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profileDir := filepath.Join(tmpDir, "profiles")
	customProfile := filepath.Join(tmpDir, "my_custom.prof")

	t.Setenv("TG_EXPERIMENT", "profiling")
	t.Setenv("TG_PROFILE_DIR", profileDir)
	t.Setenv("TG_PROFILE_CPU", customProfile)

	helpers.RunTerragrunt(t, "terragrunt version")

	info, err := os.Stat(customProfile)
	require.NoError(t, err, "Custom CPU profile should exist at %s", customProfile)
	assert.Positive(t, info.Size(), "Custom CPU profile should be non-empty")

	defaultProfile := filepath.Join(profileDir, "terragrunt_cpu.prof")
	assert.False(t, util.FileExists(defaultProfile),
		"Default profile should not exist when TG_PROFILE_CPU is explicitly set")
}

func TestTGProfileMemCreatesProfileFile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "terragrunt_mem.prof")

	t.Setenv("TG_EXPERIMENT", "profiling")
	t.Setenv("TG_PROFILE_MEM", profilePath)

	helpers.RunTerragrunt(t, "terragrunt version")

	info, err := os.Stat(profilePath)
	require.NoError(t, err, "Memory profile file %s should exist", profilePath)
	assert.Positive(t, info.Size(), "Memory profile file should be non-empty")
}

func TestTGProfileGoroutineCreatesProfileFile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "terragrunt_goroutine.prof")

	t.Setenv("TG_EXPERIMENT", "profiling")
	t.Setenv("TG_PROFILE_GOROUTINE", profilePath)

	helpers.RunTerragrunt(t, "terragrunt version")

	info, err := os.Stat(profilePath)
	require.NoError(t, err)
	assert.Positive(t, info.Size(), "Goroutine profile should be non-empty")
}

func TestProfileCPUFlagCreatesProfileFile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "cpu_from_flag.prof")

	helpers.RunTerragrunt(t, "terragrunt --experiment profiling --profile-cpu "+profilePath+" version")

	info, err := os.Stat(profilePath)
	require.NoError(t, err, "CPU profile from --profile-cpu should exist")
	assert.Positive(t, info.Size())
}

func TestProfileCPUFlagEqualsFormCreatesProfileFile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "cpu_from_equals_flag.prof")

	helpers.RunTerragrunt(t, "terragrunt --experiment=profiling --profile-cpu="+profilePath+" version")

	info, err := os.Stat(profilePath)
	require.NoError(t, err, "CPU profile from --profile-cpu= should exist")
	assert.Positive(t, info.Size())
}

func TestProfileCPUFlagAfterCommandCreatesProfileFile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "cpu_from_trailing_flag.prof")

	helpers.RunTerragrunt(t, "terragrunt version --experiment profiling --profile-cpu "+profilePath)

	info, err := os.Stat(profilePath)
	require.NoError(t, err, "CPU profile from flags placed after the command should exist")
	assert.Positive(t, info.Size())
}

func TestProfileMemFlagCreatesProfileFile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "mem_from_flag.prof")

	helpers.RunTerragrunt(t, "terragrunt --experiment profiling --profile-mem "+profilePath+" version")

	info, err := os.Stat(profilePath)
	require.NoError(t, err, "Memory profile from --profile-mem should exist")
	assert.Positive(t, info.Size())
}

func TestProfileGoroutineFlagCreatesProfileFile(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "goroutine_from_flag.prof")

	helpers.RunTerragrunt(t, "terragrunt --experiment profiling --profile-goroutine "+profilePath+" version")

	info, err := os.Stat(profilePath)
	require.NoError(t, err, "Goroutine profile from --profile-goroutine should exist")
	assert.Positive(t, info.Size())
}

func TestProfileDirFlagCollectsProfiles(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profileDir := filepath.Join(tmpDir, "profiles_from_flag")

	helpers.RunTerragrunt(t, "terragrunt --experiment profiling --profile-dir "+profileDir+" version")

	cpu := filepath.Join(profileDir, "terragrunt_cpu.prof")
	mem := filepath.Join(profileDir, "terragrunt_mem.prof")
	gor := filepath.Join(profileDir, "terragrunt_goroutine.prof")

	require.True(t, util.FileExists(cpu), "cpu profile should exist in dir")
	require.True(t, util.FileExists(mem), "mem profile should exist in dir")
	require.True(t, util.FileExists(gor), "goroutine profile should exist in dir")

	for _, p := range []string{cpu, mem, gor} {
		info, err := os.Stat(p)
		require.NoError(t, err)
		assert.Positive(t, info.Size(), p+" should be non-empty")
	}
}

func TestProfileFlagsRequireExperiment(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "cpu_no_exp.prof")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt --profile-cpu "+profilePath+" version")
	require.ErrorContains(t, err, "require the 'profiling' experiment")
	assert.False(t, util.FileExists(profilePath), "profile should not be created without experiment")
}

func TestTGProfileCPUEnvRequiresExperiment(t *testing.T) {
	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "cpu_env_no_exp.prof")

	t.Setenv("TG_PROFILE_CPU", profilePath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt version")
	require.ErrorContains(t, err, "require the 'profiling' experiment")
	assert.False(t, util.FileExists(profilePath), "profile should not be created without experiment")
}

func TestTGProfileDirEnvRequiresExperiment(t *testing.T) {
	profileDir := filepath.Join(helpers.TmpDirWOSymlinks(t), "profiles_no_exp")

	t.Setenv("TG_PROFILE_DIR", profileDir)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt version")
	require.ErrorContains(t, err, "require the 'profiling' experiment")
	assert.False(t, util.FileExists(profileDir), "profile directory should not be created without experiment")
}
