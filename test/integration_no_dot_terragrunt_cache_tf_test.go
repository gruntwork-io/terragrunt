//go:build tf

package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/info/print"
	"github.com/gruntwork-io/terragrunt/internal/codegen"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureNoDotTerragruntCacheHooks      = "fixtures/no-dot-terragrunt-cache/hooks"
	testFixtureNoDotTerragruntCacheGenerate   = "fixtures/no-dot-terragrunt-cache/generate"
	testFixtureNoDotTerragruntCacheWorkingDir = "fixtures/no-dot-terragrunt-cache/working-dir"

	noDotTerragruntCacheFlags = " --non-interactive --experiment no-dot-terragrunt-cache --working-dir "
)

func TestTFNoDotTerragruntCacheHooksResolveRelativePathsInUnitDir(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoDotTerragruntCacheHooks)
	unitPath := filepath.Join(tmpEnvPath, testFixtureNoDotTerragruntCacheHooks)

	helpers.RunTerragrunt(t, "terragrunt plan"+noDotTerragruntCacheFlags+unitPath)

	for _, name := range []string{"before.out", "after.out"} {
		contents, err := os.ReadFile(filepath.Join(unitPath, name))
		require.NoError(t, err)
		assert.Equal(t, "relative\n", string(contents))
	}

	assert.NoFileExists(t, filepath.Join(unitPath, "source.out"))
	assert.NoDirExists(t, filepath.Join(unitPath, helpers.TerragruntCache))
}

func TestTFNoDotTerragruntCacheGenerateWritesIntoUnitDir(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoDotTerragruntCacheGenerate)
	unitPath := filepath.Join(tmpEnvPath, testFixtureNoDotTerragruntCacheGenerate, "unit")
	flags := noDotTerragruntCacheFlags + unitPath

	for range 2 {
		helpers.RunTerragrunt(t, "terragrunt apply -auto-approve"+flags)
	}

	assert.FileExists(t, filepath.Join(unitPath, "provider.tf"))
	assert.FileExists(t, filepath.Join(unitPath, "backend.tf"))
	assert.FileExists(t, filepath.Join(unitPath, "state", "terraform.tfstate"))
	assert.NoDirExists(t, filepath.Join(unitPath, helpers.TerragruntCache))

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt output -raw generated"+flags)
	require.NoError(t, err)
	assert.Equal(t, "generated", stdout)
}

func TestTFNoDotTerragruntCacheGenerateKeepsUserFile(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoDotTerragruntCacheGenerate)
	unitPath := filepath.Join(tmpEnvPath, testFixtureNoDotTerragruntCacheGenerate, "user-file")
	providerPath := filepath.Join(unitPath, "provider.tf")

	before, err := os.ReadFile(providerPath)
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan"+noDotTerragruntCacheFlags+unitPath)

	var existsErr codegen.GenerateFileExistsError

	require.ErrorAs(t, err, &existsErr)

	after, err := os.ReadFile(providerPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

func TestTFNoDotTerragruntCacheRunsInWorkingDirWithConfigElsewhere(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configDir string
	}{
		{name: "config directory without code", configDir: "config"},
		{name: "config directory with code", configDir: filepath.Join("units", "a")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoDotTerragruntCacheWorkingDir)
			rootPath := filepath.Join(tmpEnvPath, testFixtureNoDotTerragruntCacheWorkingDir)
			unitPath := filepath.Join(rootPath, "unit")
			configDir := filepath.Join(rootPath, tc.configDir)
			flags := noDotTerragruntCacheFlags + unitPath +
				" --config " + filepath.Join(configDir, config.DefaultTerragruntConfigPath)

			helpers.RunTerragrunt(t, "terragrunt apply -auto-approve"+flags)

			assert.FileExists(t, filepath.Join(unitPath, "terraform.tfstate"))
			assert.NoFileExists(t, filepath.Join(configDir, "terraform.tfstate"))
			assert.NoDirExists(t, filepath.Join(unitPath, helpers.TerragruntCache))
			assert.NoDirExists(t, filepath.Join(configDir, helpers.TerragruntCache))

			helpers.RunTerragrunt(t, "terragrunt exec --in-download-dir"+flags+" -- touch exec.out")

			assert.FileExists(t, filepath.Join(unitPath, "exec.out"))
			assert.NoFileExists(t, filepath.Join(configDir, "exec.out"))

			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt info print"+flags)
			require.NoError(t, err)

			var info print.InfoOutput

			require.NoError(t, json.Unmarshal([]byte(stdout), &info))
			assert.Equal(t, unitPath, info.WorkingDir)
		})
	}
}

func TestTFNoDotTerragruntCacheExecRunsInWorkingDirWithRelativeConfig(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoDotTerragruntCacheWorkingDir)
	rootPath := filepath.Join(tmpEnvPath, testFixtureNoDotTerragruntCacheWorkingDir)
	unitPath := filepath.Join(rootPath, "unit")

	currentDir, err := os.Getwd()
	require.NoError(t, err)

	configPath, err := filepath.Rel(currentDir, filepath.Join(rootPath, "config", config.DefaultTerragruntConfigPath))
	require.NoError(t, err)

	flags := noDotTerragruntCacheFlags + unitPath + " --config " + configPath

	helpers.RunTerragrunt(t, "terragrunt exec --in-download-dir"+flags+" -- touch exec.out")

	assert.FileExists(t, filepath.Join(unitPath, "exec.out"))
}

func TestTFNoDotTerragruntCacheExecGeneratesIntoWorkingDirWithConfigElsewhere(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoDotTerragruntCacheWorkingDir)
	rootPath := filepath.Join(tmpEnvPath, testFixtureNoDotTerragruntCacheWorkingDir)
	unitPath := filepath.Join(rootPath, "unit")
	configDir := filepath.Join(rootPath, "generated")
	flags := noDotTerragruntCacheFlags + unitPath +
		" --config " + filepath.Join(configDir, config.DefaultTerragruntConfigPath)

	helpers.RunTerragrunt(t, "terragrunt exec --in-download-dir"+flags+" -- touch exec.out")

	assert.FileExists(t, filepath.Join(unitPath, "generated.tf"))
	assert.FileExists(t, filepath.Join(unitPath, "exec.out"))
	assert.NoFileExists(t, filepath.Join(configDir, "generated.tf"))
	assert.NoFileExists(t, filepath.Join(configDir, "exec.out"))

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt info print"+flags)
	require.NoError(t, err)

	var info print.InfoOutput

	require.NoError(t, json.Unmarshal([]byte(stdout), &info))
	assert.Equal(t, unitPath, info.WorkingDir)
}

func TestTFNoDotTerragruntCacheRunAllRunsEachUnitInPlace(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureNoDotTerragruntCacheWorkingDir)
	unitsPath := filepath.Join(tmpEnvPath, testFixtureNoDotTerragruntCacheWorkingDir, "units")

	helpers.RunTerragrunt(
		t,
		"terragrunt run --all"+noDotTerragruntCacheFlags+unitsPath+" -- apply -auto-approve",
	)

	for _, unit := range []string{"a", "b"} {
		assert.FileExists(t, filepath.Join(unitsPath, unit, "terraform.tfstate"))
		assert.NoDirExists(t, filepath.Join(unitsPath, unit, helpers.TerragruntCache))
	}

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -raw value"+noDotTerragruntCacheFlags+filepath.Join(unitsPath, "b"),
	)
	require.NoError(t, err)
	assert.Equal(t, "b-a", stdout)
}
