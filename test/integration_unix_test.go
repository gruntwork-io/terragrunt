//go:build linux || darwin

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

// buildSymlinksExperimentFixture lays out a tree with one real unit at `a` and
// two symlinks (`b`, `c`) pointing to it, all referencing a shared module. It
// returns the root directory.
func buildSymlinksExperimentFixture(t *testing.T) string {
	t.Helper()

	rootDir := helpers.TmpDirWOSymlinks(t)

	moduleDir := filepath.Join(rootDir, "module")
	require.NoError(t, os.Mkdir(moduleDir, 0755))
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(moduleDir, "main.tf"),
			[]byte("resource \"null_resource\" \"a\" {}\n"),
			0644,
		),
	)

	unitA := filepath.Join(rootDir, "a")
	require.NoError(t, os.Mkdir(unitA, 0755))
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(unitA, "terragrunt.hcl"),
			[]byte("terraform {\n  source = \"../module\"\n}\n"),
			0644,
		),
	)

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
		t.Skip(
			"Skipping: TG_EXPERIMENT_MODE forces all experiments on, defeating the disabled-vs-enabled comparison this test pins",
		)
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

// buildSymlinkedStackFileFixture lays out a tree where the terragrunt.stack.hcl in `live`
// is a symlink to a real stack file in `source`. The real stack file reads a sibling config
// with a path relative to get_terragrunt_dir(), and that sibling exists only next to the
// symlink (in `live`), not next to the symlink target (in `source`). It returns the `live`
// working directory and the value the sibling contributes to the generated unit's values.
func buildSymlinkedStackFileFixture(t *testing.T) (liveDir, siblingData string) {
	t.Helper()

	rootDir := helpers.TmpDirWOSymlinks(t)
	siblingData = "from-sibling"

	// The unit source: a module dir carrying a terragrunt.hcl so generation's post-copy
	// validation (which requires terragrunt.hcl at the generated unit root) passes.
	moduleDir := filepath.Join(rootDir, "module")
	require.NoError(t, os.Mkdir(moduleDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "terragrunt.hcl"),
		[]byte("inputs = {\n  data = \"placeholder\"\n}\n"), 0644))

	// The real stack file lives in `source` and reads its sibling relative to its own dir.
	sourceDir := filepath.Join(rootDir, "source")
	require.NoError(t, os.Mkdir(sourceDir, 0755))
	stackHCL := `locals {
  sibling = read_terragrunt_config("${get_terragrunt_dir()}/sibling.hcl")
}

unit "app" {
  source = "${get_terragrunt_dir()}/../module"
  path   = "app"
  values = {
    data = local.sibling.locals.data
  }
}
`
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "terragrunt.stack.hcl"), []byte(stackHCL), 0644))

	// The `live` dir holds the symlink to the real stack file, plus the sibling it reads.
	// The sibling exists here only, so generation succeeds only if reads resolve against
	// `live` (where the symlink lives), not `source` (the symlink target's dir).
	liveDir = filepath.Join(rootDir, "live")
	require.NoError(t, os.Mkdir(liveDir, 0755))
	require.NoError(t, os.Symlink(
		filepath.Join(sourceDir, "terragrunt.stack.hcl"),
		filepath.Join(liveDir, "terragrunt.stack.hcl"),
	))
	require.NoError(t, os.WriteFile(filepath.Join(liveDir, "sibling.hcl"),
		[]byte("locals {\n  data = \""+siblingData+"\"\n}\n"), 0644))

	return liveDir, siblingData
}

// TestStackGenerateSymlinkedStackFileReadsRelative is a regression test for a symlinked
// terragrunt.stack.hcl whose relative read_terragrunt_config reads were resolved against the
// symlink target's directory instead of the directory the symlink lives in. Stack generation
// canonicalises the stack file's directory but must keep the file anchored to its own location,
// so a sibling config read relative to get_terragrunt_dir() resolves next to the symlink.
func TestStackGenerateSymlinkedStackFileReadsRelative(t *testing.T) {
	t.Parallel()

	liveDir, siblingData := buildSymlinkedStackFileFixture(t)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt stack generate --working-dir "+liveDir)
	require.NoError(t, err, "stack generate should resolve the sibling read next to the symlink; stderr: %s", stderr)

	// The generated unit's values must carry the value read from the sibling, proving the
	// relative read resolved against the symlink's directory rather than its target's.
	valuesPath := filepath.Join(liveDir, ".terragrunt-stack", "app", "terragrunt.values.hcl")
	contents, readErr := os.ReadFile(valuesPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(contents), siblingData)
}
