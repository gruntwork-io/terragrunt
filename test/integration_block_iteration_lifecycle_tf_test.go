//go:build tf

package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

func applyStack(t *testing.T, dir string) {
	t.Helper()

	helpers.RunTerragrunt(
		t,
		"terragrunt stack run apply --experiment block-iteration --non-interactive"+
			" --report-file "+helpers.ReportFile+" --working-dir "+dir+" -- -auto-approve",
	)
}

// stackOutputs returns the whole stack's outputs, under the addresses stack output gives them.
func stackOutputs(t *testing.T, dir string) map[string]any {
	t.Helper()

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack output --format json --experiment block-iteration"+
			" --non-interactive --working-dir "+dir,
	)
	require.NoError(t, err)

	outputs := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	return outputs
}

// appliedRole returns the role recorded in the state a generated unit applied. State lives
// under the unit's own directory, so an orphaned unit keeps its own copy of it.
func appliedRole(t *testing.T, dir, unit string) string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(
		dir, generatedStackDir, unit, ".terragrunt-cache", "*", "*", "terraform.tfstate",
	))
	require.NoError(t, err)
	require.Lenf(t, matches, 1, "expected exactly one state file for unit %s", unit)

	contents, err := os.ReadFile(matches[0])
	require.NoError(t, err)

	state := struct {
		Outputs map[string]struct {
			Value string `json:"value"`
		} `json:"outputs"`
	}{}
	require.NoError(t, json.Unmarshal(contents, &state))

	return state.Outputs["role"].Value
}

// TestTFBlockIterationUnitStaticToDynamicStrandsState pins what an applied unit leaves behind
// when it gains an expansion. Its state stays where the bare address applied it, holding the
// value it was applied with, while stack output answers only for the keyed addresses the config
// now declares.
func TestTFBlockIterationUnitStaticToDynamicStrandsState(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationUnits)
	live := filepath.Join(root, "static")

	applyStack(t, live)
	assert.Equal(t, map[string]any{
		"aurora": map[string]any{"role": "aurora"},
	}, stackOutputs(t, live))

	switchStackConfig(t, root, "static", "for-each-set")
	applyStack(t, live)

	assert.Equal(t, map[string]any{
		"aurora": map[string]any{
			"api": map[string]any{"role": "api"},
			"web": map[string]any{"role": "web"},
		},
	}, stackOutputs(t, live))

	assert.Equal(t, "aurora", appliedRole(t, live, "aurora"))
	assert.Equal(t, "api", appliedRole(t, live, filepath.Join("aurora", "api")))
	assert.Equal(t, "web", appliedRole(t, live, filepath.Join("aurora", "web")))
}

// TestTFBlockIterationOrphanedUnitKeepsRunning pins what a stranded unit costs. A run walks
// the generated tree rather than the config, so the orphan is applied again alongside its
// replacements even though nothing addresses it anymore.
func TestTFBlockIterationOrphanedUnitKeepsRunning(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationUnits)
	live := filepath.Join(root, "static")

	applyStack(t, live)
	assert.Equal(
		t,
		[]string{filepath.Join(generatedStackDir, "aurora")},
		helpers.ReadReport(t, live, helpers.ReportFile).Names(),
	)

	switchStackConfig(t, root, "static", "for-each-set")
	generateStack(t, live)
	applyStack(t, live)

	runs := helpers.ReadReport(t, live, helpers.ReportFile)
	assert.ElementsMatch(
		t,
		[]string{
			filepath.Join(generatedStackDir, "aurora"),
			filepath.Join(generatedStackDir, "aurora", "api"),
			filepath.Join(generatedStackDir, "aurora", "web"),
		},
		runs.Names(),
	)

	for _, name := range runs.Names() {
		run := runs.FindByName(name)

		require.NotNilf(t, run, "%s never ran", name)
		assert.Equalf(t, "succeeded", run.Result, "%s did not succeed", name)
	}
}

// TestTFBlockIterationSweepingOrphansLeavesOnlyDeclaredUnits pins the way out of a stranded
// tree. A regeneration that deletes it first takes the stranded state along, so what remains
// answers to the keyed addresses and nothing else.
func TestTFBlockIterationSweepingOrphansLeavesOnlyDeclaredUnits(t *testing.T) {
	t.Parallel()

	root := copyBlockIterationFixture(t, testFixtureBlockIterationUnits)
	live := filepath.Join(root, "static")

	applyStack(t, live)

	switchStackConfig(t, root, "static", "for-each-set")
	generateStack(t, live, sweepOrphans)
	applyStack(t, live)

	assert.ElementsMatch(
		t,
		[]string{
			filepath.Join(generatedStackDir, "aurora", "api"),
			filepath.Join(generatedStackDir, "aurora", "web"),
		},
		helpers.ReadReport(t, live, helpers.ReportFile).Names(),
	)

	assert.Equal(t, map[string]any{
		"aurora": map[string]any{
			"api": map[string]any{"role": "api"},
			"web": map[string]any{"role": "web"},
		},
	}, stackOutputs(t, live))
}
