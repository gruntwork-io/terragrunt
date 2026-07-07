//nolint:paralleltest // Tests use t.Setenv, and CPU profiling is process-global (pprof.StartCPUProfile), so tests in this file must not run in parallel.
package test_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFixtureProfileMultiUnit = "fixtures/profile/multi-unit"

func TestTGProfileEnvVarsCreateProfileFiles(t *testing.T) {
	testCases := []struct {
		name     string
		envName  string
		fileName string
	}{
		{name: "cpu", envName: "TG_PROFILE_CPU", fileName: "terragrunt_cpu.prof"},
		{name: "mem", envName: "TG_PROFILE_MEM", fileName: "terragrunt_mem.prof"},
		{name: "goroutine", envName: "TG_PROFILE_GOROUTINE", fileName: "terragrunt_goroutine.prof"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			profilePath := filepath.Join(helpers.TmpDirWOSymlinks(t), tc.fileName)

			t.Setenv("TG_EXPERIMENT", "profiling")
			t.Setenv(tc.envName, profilePath)

			helpers.RunTerragrunt(t, "terragrunt version")

			requireNonEmptyFile(t, profilePath)
		})
	}
}

func TestProfileFlagsCreateProfileFiles(t *testing.T) {
	testCases := []struct {
		name string
		args string
	}{
		{name: "cpu", args: "terragrunt --experiment profiling --profile-cpu %s version"},
		{name: "cpu equals form", args: "terragrunt --experiment=profiling --profile-cpu=%s version"},
		{name: "cpu flags after command", args: "terragrunt version --experiment profiling --profile-cpu %s"},
		{name: "mem", args: "terragrunt --experiment profiling --profile-mem %s version"},
		{name: "goroutine", args: "terragrunt --experiment profiling --profile-goroutine %s version"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			profilePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "from_flag.prof")

			helpers.RunTerragrunt(t, fmt.Sprintf(tc.args, profilePath))

			requireNonEmptyFile(t, profilePath)
		})
	}
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

	requireNonEmptyFile(t, profilePath)

	assert.Empty(t, findProfileFiles(t, tmpEnvPath),
		"TG_PROFILE_CPU alone should not produce OpenTofu profiles anywhere under the working dir")
	assert.Equal(t, []string{profilePath}, findProfileFiles(t, tmpDir),
		"the Terragrunt profile should be the only profile produced")
}

func TestTGProfileDirDoesNotOverrideExplicitTofuCPUProfile(t *testing.T) {
	if isTerraform(t.Context()) {
		t.Skip("TOFU_CPU_PROFILE is only supported by OpenTofu")
	}

	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputs)
	rootPath := filepath.Join(tmpEnvPath, testFixtureInputs)

	tmpDir := helpers.TmpDirWOSymlinks(t)
	profileDir := filepath.Join(tmpDir, "profiles")
	tofuProfilePath := filepath.Join(tmpDir, "tofu_cpu.prof")

	t.Setenv("TG_EXPERIMENT", "profiling")
	t.Setenv("TG_PROFILE_DIR", profileDir)
	t.Setenv("TOFU_CPU_PROFILE", tofuProfilePath)

	helpers.RunTerragrunt(t, "terragrunt plan --non-interactive --working-dir "+rootPath)

	requireNonEmptyFile(t, tofuProfilePath)

	derived, err := filepath.Glob(filepath.Join(profileDir, "tofu_cpu*"))
	require.NoError(t, err)
	assert.Empty(t, derived, "explicit TOFU_CPU_PROFILE should suppress the derived per-unit profile paths")
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

	for _, name := range []string{"terragrunt_cpu.prof", "terragrunt_mem.prof", "terragrunt_goroutine.prof", "tofu_cpu_plan.prof"} {
		requireNonEmptyFile(t, filepath.Join(profileDir, name))
	}
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
		requireNonEmptyFile(t, filepath.Join(profileDir, unit, "tofu_cpu_plan.prof"))
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

	requireNonEmptyFile(t, customProfile)
	assert.NoFileExists(t, filepath.Join(profileDir, "terragrunt_cpu.prof"),
		"default profile should not exist when TG_PROFILE_CPU is explicitly set")
}

func TestProfileDirFlagCollectsProfiles(t *testing.T) {
	profileDir := filepath.Join(helpers.TmpDirWOSymlinks(t), "profiles_from_flag")

	helpers.RunTerragrunt(t, "terragrunt --experiment profiling --profile-dir "+profileDir+" version")

	for _, name := range []string{"terragrunt_cpu.prof", "terragrunt_mem.prof", "terragrunt_goroutine.prof"} {
		requireNonEmptyFile(t, filepath.Join(profileDir, name))
	}
}

func TestProfileDirFlagPointingAtFileErrors(t *testing.T) {
	filePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "not_a_dir")
	require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o600))

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt --experiment profiling --profile-dir "+filePath+" version")
	require.ErrorContains(t, err, "could not create profile directory")
}

func TestProfileMemFlagBadPathErrors(t *testing.T) {
	profilePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "missing", "mem.prof")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt --experiment profiling --profile-mem "+profilePath+" version")
	require.ErrorContains(t, err, "could not create memory profile")
	assert.NoFileExists(t, profilePath)
}

func TestProfileFlagsRequireExperiment(t *testing.T) {
	if helpers.IsExperimentMode(t) {
		t.Skip("Skipping because we can't verify the experiment is required when experiment mode is enabled")
	}

	profilePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "cpu_no_exp.prof")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt --profile-cpu "+profilePath+" version")
	require.ErrorIs(t, err, commands.ErrProfilingRequiresExperiment)
	assert.NoFileExists(t, profilePath, "profile should not be created without experiment")
}

func TestTGProfileCPUEnvRequiresExperiment(t *testing.T) {
	if helpers.IsExperimentMode(t) {
		t.Skip("Skipping because we can't verify the experiment is required when experiment mode is enabled")
	}

	profilePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "cpu_env_no_exp.prof")

	t.Setenv("TG_PROFILE_CPU", profilePath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt version")
	require.ErrorIs(t, err, commands.ErrProfilingRequiresExperiment)
	assert.NoFileExists(t, profilePath, "profile should not be created without experiment")
}

func TestTGProfileDirEnvRequiresExperiment(t *testing.T) {
	if helpers.IsExperimentMode(t) {
		t.Skip("Skipping because we can't verify the experiment is required when experiment mode is enabled")
	}

	profileDir := filepath.Join(helpers.TmpDirWOSymlinks(t), "profiles_no_exp")

	t.Setenv("TG_PROFILE_DIR", profileDir)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt version")
	require.ErrorIs(t, err, commands.ErrProfilingRequiresExperiment)
	assert.NoDirExists(t, profileDir, "profile directory should not be created without experiment")
}

// requireNonEmptyFile asserts that the profile at path exists and is non-empty.
func requireNonEmptyFile(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	require.NoError(t, err, "profile %s should exist", path)
	assert.Positive(t, info.Size(), "profile %s should be non-empty", path)
}

// findProfileFiles returns all *.prof files under root, recursively.
func findProfileFiles(t *testing.T, root string) []string {
	t.Helper()

	var profiles []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && filepath.Ext(path) == ".prof" {
			profiles = append(profiles, path)
		}

		return nil
	})
	require.NoError(t, err)

	return profiles
}
