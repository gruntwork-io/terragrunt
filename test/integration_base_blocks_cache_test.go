package test_test

import (
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixtureBaseBlocksCache = "fixtures/base-blocks-cache"

// renderedConfigName is the file `render --json --write` leaves next to each unit.
const renderedConfigName = "terragrunt.rendered.json"

// baseBlocksCacheUnits are the units the fixture renders. unit-a and unit-b share a parent
// the cache may reuse, which is what puts the cache in the path of this comparison at all;
// unit-c and unit-d share one it must refuse, so that a run with the cache on still has to
// answer both of them from their own directory, plan file and read set.
//
// A run renders the same whether or not a file was reused, so nothing here can see the cache
// working. TestBaseBlocksCacheReusesTheRenderFixtureParent holds the shared parent below to
// being cacheable, which is what keeps that first claim true.
var baseBlocksCacheUnits = []string{
	filepath.Join("unit-a", renderedConfigName),
	filepath.Join("unit-c", renderedConfigName),
	filepath.Join("nested", "unit-b", renderedConfigName),
	filepath.Join("nested", "unit-d", renderedConfigName),
}

// TestBaseBlocksCacheRendersIdenticallyWithTheCacheOff renders a tree twice from the command
// line, once with the base blocks cache on and once with it off, and holds the two sets of
// rendered files to being the same file for file.
//
// Both runs render the same directory. Rendered config carries absolute paths, so rendering
// two copies of the tree would differ on the copies' own locations rather than on anything
// the cache did; the rendered files are moved aside between the runs instead.
func TestBaseBlocksCacheRendersIdenticallyWithTheCacheOff(t *testing.T) {
	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows since the fixture's run_cmd calls a Unix command")
	}

	helpers.CleanupTerraformFolder(t, fixtureBaseBlocksCache)

	tmpEnvPath := helpers.CopyEnvironment(t, fixtureBaseBlocksCache)
	rootPath := filepath.Join(tmpEnvPath, fixtureBaseBlocksCache)

	// get_repo_root and its two relatives shell out to git, so the copy has to be a
	// repository before the fixture can name where it sits.
	gitInit := exec.CommandContext(t.Context(), "git", "init", rootPath)
	require.NoError(t, gitInit.Run())

	render := "terragrunt render --all --json -w --non-interactive --experiment deep-merge --working-dir " + rootPath

	// The runs below take their environment from this process, so a shell that already turned
	// the cache off would leave both halves comparing an uncached run against another. An empty
	// value reads as unset, which pins the first half to running with the cache on.
	t.Setenv(config.EnvDisableBaseBlocksCache, "")

	helpers.RunTerragrunt(t, render)
	withCache := collectRenderedConfigs(t, rootPath)

	t.Setenv(config.EnvDisableBaseBlocksCache, "true")

	helpers.RunTerragrunt(t, render)
	withoutCache := collectRenderedConfigs(t, rootPath)

	require.ElementsMatch(
		t,
		baseBlocksCacheUnits,
		slices.Collect(maps.Keys(withCache)),
		"the fixture did not render every one of its units",
	)

	assert.Equal(t, withoutCache, withCache)
}

// collectRenderedConfigs reads every rendered config under root, keyed by its path relative
// to root, and removes it so the next run renders into the tree the first one started from.
func collectRenderedConfigs(t *testing.T, root string) map[string]string {
	t.Helper()

	rendered := map[string]string{}

	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != renderedConfigName {
			return err
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		rendered[rel] = string(contents)

		return os.Remove(path)
	}))

	return rendered
}
