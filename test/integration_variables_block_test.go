package test_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const testFixtureVariablesBlock = "fixtures/variables-block"

func TestHclValidateInputsCountsVariablesBlocks(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVariablesBlock)
	unitPath := filepath.Join(tmpEnvPath, testFixtureVariablesBlock, "validate-inputs")

	helpers.RunTerragrunt(
		t,
		"terragrunt hcl validate --inputs --strict --experiment variables-block --working-dir "+unitPath,
	)
}

func TestHclValidateInputsReportsUndeclaredVariablesBlockInput(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVariablesBlock)
	unitPath := filepath.Join(tmpEnvPath, testFixtureVariablesBlock, "undeclared-allowed")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt hcl validate --inputs --strict --experiment variables-block --working-dir "+unitPath,
	)
	require.Error(t, err)
}

func TestRenderVariablesBlockHandlesSkippedOutputsLikeInputs(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVariablesBlock)
	unitPath := filepath.Join(tmpEnvPath, testFixtureVariablesBlock, "skip-outputs", "unit")

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt render --json --experiment variables-block --working-dir "+unitPath,
	)
	require.NoError(t, err)

	var rendered struct {
		Inputs    map[string]any `json:"inputs"`
		Variables []struct {
			Inputs map[string]any `json:"inputs"`
		} `json:"variables"`
	}

	require.NoError(t, json.Unmarshal([]byte(stdout), &rendered))
	require.Len(t, rendered.Variables, 1)

	assert.Equal(t, rendered.Inputs["from_attr"], rendered.Variables[0].Inputs["from_block"])
}

func TestHclValidateVariablesBlockDuplicateInput(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVariablesBlock)
	fixturePath := filepath.Join(tmpEnvPath, testFixtureVariablesBlock)
	unitPath := filepath.Join(fixturePath, "duplicate")

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt hcl validate --json --experiment variables-block --working-dir "+unitPath,
	)
	require.Error(t, err)

	var diags diagnostic.Diagnostics

	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(stdout)), &diags))
	require.Len(t, diags, 1)

	diag := diags[0]

	assert.Equal(t, "Duplicate input", diag.Summary)
	assert.Contains(t, diag.Detail, filepath.Join(unitPath, "terragrunt.hcl")+":3,14-18")

	require.NotNil(t, diag.Range)
	assert.Equal(t, filepath.Join(unitPath, "terragrunt.hcl"), diag.Range.Filename)
	assert.Equal(t, 9, diag.Range.Start.Line)
	assert.Equal(t, 5, diag.Range.Start.Column)
	assert.Equal(t, 9, diag.Range.End.Column)

	require.NotNil(t, diag.Snippet)
	assert.Equal(t, `    name = "second"`, diag.Snippet.Code)
	assert.Equal(t, 4, diag.Snippet.HighlightStartOffset)
	assert.Equal(t, 8, diag.Snippet.HighlightEndOffset)
}
