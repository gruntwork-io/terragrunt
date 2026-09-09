package cli_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	generateCommand = []string{"stack", "generate"}
	renderCommand   = []string{"render", "--format", "json"}
)

type blockIterationGateCase struct {
	name       string
	blockType  string
	blockLabel string
	files      tree
	command    []string
}

// runGateCase runs command against files with the experiment left off.
func runGateCase(t *testing.T, files tree, command []string) error {
	t.Helper()

	v, _ := blockIterationVenv(t, files)

	_, err := runCLI(t, v, slices.Concat(command, []string{"--no-color", "--working-dir", liveDir})...)

	return err
}

// TestBlockIterationExpansionRequiresExperiment pins the closed gate for every block type an
// expansion block may appear in, in each of the ways it can be written.
func TestBlockIterationExpansionRequiresExperiment(t *testing.T) {
	t.Parallel()

	testCases := []blockIterationGateCase{
		{
			name:       "unit for_each set",
			files:      unitTree(unitForEachSet),
			command:    generateCommand,
			blockType:  "unit",
			blockLabel: "aurora",
		},
		{
			name:       "unit for_each map",
			files:      unitTree(unitForEachMap),
			command:    generateCommand,
			blockType:  "unit",
			blockLabel: "aurora",
		},
		{
			name:       "unit count",
			files:      unitTree(unitCount),
			command:    generateCommand,
			blockType:  "unit",
			blockLabel: "aurora",
		},
		{
			name:       "stack for_each set",
			files:      stackTree(stackForEachSet),
			command:    generateCommand,
			blockType:  "stack",
			blockLabel: "team",
		},
		{
			name:       "stack count",
			files:      stackTree(stackCount),
			command:    generateCommand,
			blockType:  "stack",
			blockLabel: "team",
		},
		{
			name:       "dependency for_each set",
			files:      dependencyTree(dependencyForEachSet),
			command:    renderCommand,
			blockType:  "dependency",
			blockLabel: "aurora",
		},
		{
			name:       "dependency for_each map",
			files:      dependencyTree(dependencyForEachMap),
			command:    renderCommand,
			blockType:  "dependency",
			blockLabel: "aurora",
		},
		{
			name:       "dependency count",
			files:      dependencyTree(dependencyCount),
			command:    renderCommand,
			blockType:  "dependency",
			blockLabel: "aurora",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var expansionErr config.ExpansionRequiresExperimentError
			require.ErrorAs(t, runGateCase(t, tc.files, tc.command), &expansionErr)

			assert.Equal(t, tc.blockType, expansionErr.BlockType)
			assert.Equal(t, tc.blockLabel, expansionErr.BlockLabel)
		})
	}
}

// TestBlockIterationEnabledRequiresExperiment pins the closed gate for the bare enabled
// attribute, which the experiment introduced on unit and stack blocks.
func TestBlockIterationEnabledRequiresExperiment(t *testing.T) {
	t.Parallel()

	testCases := []blockIterationGateCase{
		{
			name:       "unit",
			files:      unitTree(unitDisabled),
			command:    generateCommand,
			blockType:  "unit",
			blockLabel: "aurora",
		},
		{
			name:       "stack",
			files:      stackTree(stackDisabled),
			command:    generateCommand,
			blockType:  "stack",
			blockLabel: "team",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var enabledErr config.EnabledRequiresExperimentError
			require.ErrorAs(t, runGateCase(t, tc.files, tc.command), &enabledErr)

			assert.Equal(t, tc.blockType, enabledErr.BlockType)
			assert.Equal(t, tc.blockLabel, enabledErr.BlockLabel)
		})
	}
}

// TestBlockIterationDependencyEnabledPredatesTheExperiment pins the one cell of the gate
// matrix the experiment leaves alone. Dependency blocks accepted enabled before it existed,
// so gating that spelling would break configs that never opted in.
func TestBlockIterationDependencyEnabledPredatesTheExperiment(t *testing.T) {
	t.Parallel()

	v, _ := blockIterationVenv(t, dependencyTree(dependencyDisabled))

	assert.Equal(t, map[string]any{
		"addresses": []any{"aurora"},
		"aurora_id": "aurora-web-id",
	}, renderedInputs(t, v, gateClosed))
}

// TestBlockIterationGateGeneratesNothing pins that a closed gate stops the whole run before
// anything is generated, so a user who forgot the flag gets no half-built tree.
func TestBlockIterationGateGeneratesNothing(t *testing.T) {
	t.Parallel()

	v, fsys := blockIterationVenv(t, unitTree(unitForEachSet))

	_, err := runCLI(t, v, "stack", "generate", "--no-color", "--working-dir", liveDir)

	var expansionErr config.ExpansionRequiresExperimentError
	require.ErrorAs(t, err, &expansionErr)

	assert.False(t, vfs.Exists(fsys, filepath.Join(liveDir, generatedStackDir)))
}
