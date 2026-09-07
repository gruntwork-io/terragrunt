package test_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const (
	generateWithoutExperiment = "terragrunt stack generate"
	renderWithoutExperiment   = "terragrunt render --format json --non-interactive"
)

type blockIterationGateCase struct {
	name       string
	fixture    string
	variant    string
	command    string
	blockType  string
	blockLabel string
}

// runGateCase runs command against the fixture variant without the experiment flag.
func runGateCase(t *testing.T, fixture, variant, command string) error {
	t.Helper()

	dir := filepath.Join(copyBlockIterationFixture(t, fixture), variant)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, command+" --working-dir "+dir)

	return err
}

// TestBlockIterationExpansionRequiresExperiment pins the closed gate for every block type an
// expansion block may appear in, in each of the ways it can be written.
func TestBlockIterationExpansionRequiresExperiment(t *testing.T) {
	t.Parallel()
	helpers.SkipInExperimentMode(t, experiment.BlockIteration)

	testCases := []blockIterationGateCase{
		{
			name:       "unit for_each set",
			fixture:    testFixtureBlockIterationUnits,
			variant:    "for-each-set",
			command:    generateWithoutExperiment,
			blockType:  "unit",
			blockLabel: "aurora",
		},
		{
			name:       "unit for_each map",
			fixture:    testFixtureBlockIterationUnits,
			variant:    "for-each-map",
			command:    generateWithoutExperiment,
			blockType:  "unit",
			blockLabel: "aurora",
		},
		{
			name:       "unit count",
			fixture:    testFixtureBlockIterationUnits,
			variant:    "count",
			command:    generateWithoutExperiment,
			blockType:  "unit",
			blockLabel: "aurora",
		},
		{
			name:       "stack for_each set",
			fixture:    testFixtureBlockIterationStacks,
			variant:    "for-each-set",
			command:    generateWithoutExperiment,
			blockType:  "stack",
			blockLabel: "team",
		},
		{
			name:       "stack count",
			fixture:    testFixtureBlockIterationStacks,
			variant:    "count",
			command:    generateWithoutExperiment,
			blockType:  "stack",
			blockLabel: "team",
		},
		{
			name:       "dependency for_each set",
			fixture:    testFixtureBlockIterationDependencies,
			variant:    "for-each-set",
			command:    renderWithoutExperiment,
			blockType:  "dependency",
			blockLabel: "aurora",
		},
		{
			name:       "dependency for_each map",
			fixture:    testFixtureBlockIterationDependencies,
			variant:    "for-each-map",
			command:    renderWithoutExperiment,
			blockType:  "dependency",
			blockLabel: "aurora",
		},
		{
			name:       "dependency count",
			fixture:    testFixtureBlockIterationDependencies,
			variant:    "count",
			command:    renderWithoutExperiment,
			blockType:  "dependency",
			blockLabel: "aurora",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var expansionErr config.ExpansionRequiresExperimentError
			require.ErrorAs(
				t,
				runGateCase(t, tc.fixture, tc.variant, tc.command),
				&expansionErr,
			)

			assert.Equal(t, tc.blockType, expansionErr.BlockType)
			assert.Equal(t, tc.blockLabel, expansionErr.BlockLabel)
		})
	}
}

// TestBlockIterationEnabledRequiresExperiment pins the closed gate for the bare enabled
// attribute, which the experiment introduced on unit and stack blocks.
func TestBlockIterationEnabledRequiresExperiment(t *testing.T) {
	t.Parallel()
	helpers.SkipInExperimentMode(t, experiment.BlockIteration)

	testCases := []blockIterationGateCase{
		{
			name:       "unit",
			fixture:    testFixtureBlockIterationUnits,
			variant:    "disabled",
			command:    generateWithoutExperiment,
			blockType:  "unit",
			blockLabel: "aurora",
		},
		{
			name:       "stack",
			fixture:    testFixtureBlockIterationStacks,
			variant:    "disabled",
			command:    generateWithoutExperiment,
			blockType:  "stack",
			blockLabel: "team",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var enabledErr config.EnabledRequiresExperimentError
			require.ErrorAs(
				t,
				runGateCase(t, tc.fixture, tc.variant, tc.command),
				&enabledErr,
			)

			assert.Equal(t, tc.blockType, enabledErr.BlockType)
			assert.Equal(t, tc.blockLabel, enabledErr.BlockLabel)
		})
	}
}

// TestBlockIterationDependencyEnabledPredatesTheExperiment pins the one cell of the gate matrix
// the experiment must leave alone. Dependency blocks accepted enabled long before it existed,
// so gating that spelling now would break configs that never opted in.
func TestBlockIterationDependencyEnabledPredatesTheExperiment(t *testing.T) {
	t.Parallel()

	helpers.SkipInExperimentMode(t, experiment.BlockIteration)

	root := copyBlockIterationFixture(t, testFixtureBlockIterationDependencies)

	assert.Equal(t, map[string]any{
		"addresses": []any{"aurora"},
		"aurora_id": "aurora-web-id",
	}, renderedInputs(t, gateClosed, filepath.Join(root, "disabled")))
}

// TestBlockIterationGateGeneratesNothing pins that a closed gate stops the whole run rather
// than generating part of it, so a user who forgot the flag gets no half-built tree.
func TestBlockIterationGateGeneratesNothing(t *testing.T) {
	t.Parallel()

	helpers.SkipInExperimentMode(t, experiment.BlockIteration)

	root := copyBlockIterationFixture(t, testFixtureBlockIterationUnits)
	live := filepath.Join(root, "for-each-set")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		generateWithoutExperiment+" --working-dir "+live,
	)
	require.Error(t, err)

	assert.NoDirExists(t, filepath.Join(live, generatedStackDir))
}
