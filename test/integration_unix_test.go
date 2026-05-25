//go:build linux || darwin
// +build linux darwin

package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureDownloadPath                      = "fixtures/download"
	testFixtureLocalRelativeArgsUnixDownloadPath = "fixtures/download/local-relative-extra-args-unix"
)

func TestLocalWithRelativeExtraArgsUnix(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDownloadPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureLocalRelativeArgsUnixDownloadPath)

	testPath, err := filepath.EvalSymlinks(testPath)
	require.NoError(t, err)

	helpers.CleanupTerraformFolder(t, testPath)

	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testPath)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+testPath)
}

// buildSymlinksExperimentFixture lays out a tree with one real unit at `a` and
// two symlinks (`b`, `c`) pointing to it, all referencing a shared module. It
// returns the root directory.
func buildSymlinksExperimentFixture(t *testing.T) string {
	t.Helper()

	rootDir := helpers.TmpDirWOSymlinks(t)

	moduleDir := filepath.Join(rootDir, "module")
	require.NoError(t, os.Mkdir(moduleDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "main.tf"), []byte("resource \"null_resource\" \"a\" {}\n"), 0644))

	unitA := filepath.Join(rootDir, "a")
	require.NoError(t, os.Mkdir(unitA, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(unitA, "terragrunt.hcl"), []byte("terraform {\n  source = \"../module\"\n}\n"), 0644))

	require.NoError(t, os.Symlink(unitA, filepath.Join(rootDir, "b")))
	require.NoError(t, os.Symlink(unitA, filepath.Join(rootDir, "c")))

	return rootDir
}

// TestSymlinksExperimentUnitDiscoveryWithRacing pins the symlinks experiment's effect on
// unit discovery: with the experiment enabled, symlinked unit directories are walked as if
// they were real directories and surface as distinct units; without it, they're skipped.
//
// The CI "Race" job runs tests matching .*WithRacing with -race, which catches concurrent
// access regressions in the discovery walk.
func TestSymlinksExperimentUnitDiscoveryWithRacing(t *testing.T) {
	t.Parallel()

	if helpers.IsExperimentMode(t) {
		t.Skip("Skipping: TG_EXPERIMENT_MODE forces all experiments on, defeating the disabled-vs-enabled comparison this test pins")
	}

	rootDir := buildSymlinksExperimentFixture(t)

	type entry struct {
		Type string `json:"type"`
		Path string `json:"path"`
	}

	parse := func(t *testing.T, stdout string) []entry {
		t.Helper()

		var out []entry

		require.NoError(t, json.Unmarshal([]byte(stdout), &out))

		return out
	}

	t.Run("experiment disabled", func(t *testing.T) {
		t.Parallel()

		stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
			"terragrunt find --no-color --json --working-dir "+rootDir)
		require.NoError(t, err)

		assert.ElementsMatch(t, []entry{{Type: "unit", Path: "a"}}, parse(t, stdout))
	})

	t.Run("experiment enabled", func(t *testing.T) {
		t.Parallel()

		stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
			"terragrunt find --no-color --json --experiment symlinks --working-dir "+rootDir)
		require.NoError(t, err)

		assert.ElementsMatch(t, []entry{
			{Type: "unit", Path: "a"},
			{Type: "unit", Path: "b"},
			{Type: "unit", Path: "c"},
		}, parse(t, stdout))
	})
}

// TestSymlinksExperimentRunAllWithRacing pins the symlinks experiment's effect on
// `run --all`: with the experiment enabled, each symlinked unit is executed as a
// distinct unit; without it, only the real unit runs. The check counts terraform's
// "Apply complete!" lines on stdout — one per executed unit.
//
// `--parallelism 1` serializes execution since the symlink targets share a single
// on-disk working directory, so concurrent applies would race over state.
func TestSymlinksExperimentRunAllWithRacing(t *testing.T) {
	t.Parallel()

	if helpers.IsExperimentMode(t) {
		t.Skip("Skipping: TG_EXPERIMENT_MODE forces all experiments on, defeating the disabled-vs-enabled comparison this test pins")
	}

	t.Run("experiment disabled", func(t *testing.T) {
		t.Parallel()

		rootDir := buildSymlinksExperimentFixture(t)

		stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
			"terragrunt run --all --no-color --non-interactive --parallelism 1 --working-dir "+rootDir+" -- apply -auto-approve")
		require.NoError(t, err)

		assert.Equal(t, 1, strings.Count(stdout, "Apply complete!"))
	})

	t.Run("experiment enabled", func(t *testing.T) {
		t.Parallel()

		rootDir := buildSymlinksExperimentFixture(t)

		stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
			"terragrunt run --all --no-color --non-interactive --parallelism 1 --experiment symlinks --working-dir "+rootDir+" -- apply -auto-approve")
		require.NoError(t, err)

		assert.Equal(t, 3, strings.Count(stdout, "Apply complete!"))
	})
}
