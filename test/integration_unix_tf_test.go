//go:build (linux || darwin) && tf

package test_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFLocalWithRelativeExtraArgsUnix(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.NewGitServer(t).RenderFixture(testFixtureDownloadPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureLocalRelativeArgsUnixDownloadPath)

	testPath, err := filepath.EvalSymlinks(testPath)
	require.NoError(t, err)

	helpers.CleanupTerraformFolder(t, testPath)

	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+testPath,
	)

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+testPath,
	)
}

// TestTFSymlinksExperimentRunAllWithRacing pins the symlinks experiment's effect on
// `run --all`: with the experiment enabled, each symlinked unit is executed as a
// distinct unit; without it, only the real unit runs. The check counts terraform's
// "Apply complete!" lines on stdout — one per executed unit.
//
// `--parallelism 1` serializes execution since the symlink targets share a single
// on-disk working directory, so concurrent applies would race over state.
func TestTFSymlinksExperimentRunAllWithRacing(t *testing.T) {
	t.Parallel()

	if helpers.IsExperimentMode(t) {
		t.Skip(
			"Skipping: TG_EXPERIMENT_MODE forces all experiments on, defeating the disabled-vs-enabled comparison this test pins",
		)
	}

	t.Run("experiment disabled", func(t *testing.T) {
		t.Parallel()

		rootDir := buildSymlinksExperimentFixture(t)

		stdout, _, err := helpers.RunTerragruntCommandWithOutput(
			t,
			"terragrunt run --all --no-color --non-interactive --parallelism 1 --working-dir "+rootDir+" -- apply -auto-approve",
		)
		require.NoError(t, err)

		assert.Equal(t, 1, strings.Count(stdout, "Apply complete!"))
	})

	t.Run("experiment enabled", func(t *testing.T) {
		t.Parallel()

		rootDir := buildSymlinksExperimentFixture(t)

		stdout, _, err := helpers.RunTerragruntCommandWithOutput(
			t,
			"terragrunt run --all --no-color --non-interactive --parallelism 1 --experiment symlinks --working-dir "+rootDir+" -- apply -auto-approve",
		)
		require.NoError(t, err)

		assert.Equal(t, 3, strings.Count(stdout, "Apply complete!"))
	})
}
