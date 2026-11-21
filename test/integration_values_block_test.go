package test_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const (
	testFixtureValuesBlock = "fixtures/values-block"
	valuesBlockValuesFile  = "terragrunt.values.hcl"
)

func TestValuesBlockStackGenerateRejectsUndeclaredValue(t *testing.T) {
	t.Parallel()

	stackPath := filepath.Join(copyValuesBlockFixture(t), "live", "unknown-key")

	diag := requireStackGenerateDiagnostic(t, stackPath)
	assert.Equal(t, `Undeclared value "tag" for unit "app"`, diag.Summary)
	assert.Contains(t, diag.Detail, `Did you mean "tags"?`)
	assert.Equal(t, filepath.Join(stackPath, config.DefaultStackFile), diag.Subject.Filename)
	assert.Equal(t, 7, diag.Subject.Start.Line)
}

func TestValuesBlockStackGenerateRejectsMissingRequiredValue(t *testing.T) {
	t.Parallel()

	stackPath := filepath.Join(copyValuesBlockFixture(t), "live", "missing-required")

	diag := requireStackGenerateDiagnostic(t, stackPath)
	assert.Equal(t, `Missing required value "name" for unit "app"`, diag.Summary)
	assert.Equal(t, filepath.Join(stackPath, config.DefaultStackFile), diag.Subject.Filename)
	assert.Equal(t, 5, diag.Subject.Start.Line)
}

func TestValuesBlockStackGenerateRejectsMistypedValue(t *testing.T) {
	t.Parallel()

	stackPath := filepath.Join(copyValuesBlockFixture(t), "live", "mistyped")

	diag := requireStackGenerateDiagnostic(t, stackPath)
	assert.Equal(t, `Invalid value "instance_count" for unit "app"`, diag.Summary)
	assert.Contains(t, diag.Detail, "a number is required")
	assert.Equal(t, filepath.Join(stackPath, config.DefaultStackFile), diag.Subject.Filename)
	assert.Equal(t, 7, diag.Subject.Start.Line)
}

func TestValuesBlockStackGenerateFillsDefaults(t *testing.T) {
	t.Parallel()

	stackPath := filepath.Join(copyValuesBlockFixture(t), "live", "valid")

	helpers.RunTerragrunt(t, "terragrunt stack generate --experiment values-block --working-dir "+stackPath)

	inputs := renderValuesBlockInputs(t, filepath.Join(stackPath, ".terragrunt-stack", "app"))
	assert.Equal(t, map[string]any{
		"name":           "web",
		"instance_count": float64(3),
		"tags":           map[string]any{},
	}, inputs)
}

func TestValuesBlockStackGenerateChecksHandEditedValuesFile(t *testing.T) {
	t.Parallel()

	stackPath := filepath.Join(copyValuesBlockFixture(t), "live", "valid")

	helpers.RunTerragrunt(t, "terragrunt stack generate --experiment values-block --working-dir "+stackPath)

	unitPath := filepath.Join(stackPath, ".terragrunt-stack", "app")
	valuesPath := filepath.Join(unitPath, valuesBlockValuesFile)

	require.NoError(t, vfs.WriteFile(
		vfs.NewOSFS(),
		valuesPath,
		[]byte("name          = \"web\"\ninstance_cont = 2\n"),
		0o644,
	))

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt render --json --experiment values-block --working-dir "+unitPath,
	)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
	require.Len(t, diags, 1)
	assert.Equal(t, `Undeclared value "instance_cont"`, diags[0].Summary)
	assert.Contains(t, diags[0].Detail, `Did you mean "instance_count"?`)
	assert.Equal(t, valuesPath, diags[0].Subject.Filename)
	assert.Equal(t, 2, diags[0].Subject.Start.Line)
}

func TestValuesBlockStackGenerateNestedStackDefaults(t *testing.T) {
	t.Parallel()

	fixturePath := copyValuesBlockFixture(t)

	runner, err := git.NewGitRunner(venv.OSVenv())
	require.NoError(t, err)
	require.NoError(t, runner.WithWorkDir(fixturePath).Init(t.Context()))

	stackPath := filepath.Join(fixturePath, "live", "nested")

	helpers.RunTerragrunt(t, "terragrunt stack generate --experiment values-block --working-dir "+stackPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt list --no-color --experiment values-block --working-dir "+stackPath,
	)
	require.NoError(t, err)

	unitRel := filepath.Join(".terragrunt-stack", "service", ".terragrunt-stack", "app-dev")
	assert.Contains(t, strings.Fields(stdout), unitRel)

	inputs := renderValuesBlockInputs(t, filepath.Join(stackPath, unitRel))
	assert.Equal(t, map[string]any{
		"name":           "api",
		"instance_count": float64(2),
		"tags":           map[string]any{},
	}, inputs)
}

func TestHclValidateValuesBlockUndeclaredReference(t *testing.T) {
	t.Parallel()

	unitPath := filepath.Join(copyValuesBlockFixture(t), "unit")

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt hcl validate --json --experiment values-block --working-dir "+unitPath,
	)
	require.Error(t, err)

	var diags diagnostic.Diagnostics

	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(stdout)), &diags))
	require.Len(t, diags, 1)

	diag := diags[0]

	assert.Equal(t, "Undeclared value", diag.Summary)
	assert.Contains(t, diag.Detail, `Did you mean "tags"?`)

	require.NotNil(t, diag.Range)
	assert.Equal(t, filepath.Join(unitPath, config.DefaultTerragruntConfigPath), diag.Range.Filename)
	assert.Equal(t, 14, diag.Range.Start.Line)
	assert.Equal(t, 14, diag.Range.Start.Column)
	assert.Equal(t, 25, diag.Range.End.Column)

	require.NotNil(t, diag.Snippet)
	assert.Equal(t, `  tags = try(values.tgas, {})`, diag.Snippet.Code)
	assert.Equal(t, 13, diag.Snippet.HighlightStartOffset)
	assert.Equal(t, 24, diag.Snippet.HighlightEndOffset)
}

func TestValuesBlockRequiresExperiment(t *testing.T) {
	t.Parallel()

	helpers.SkipInExperimentMode(t, experiment.ValuesBlock)

	unitPath := filepath.Join(copyValuesBlockFixture(t), "unit")

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt render --json --working-dir "+unitPath)

	var experimentErr config.ValuesBlockRequiresExperimentError

	require.ErrorAs(t, err, &experimentErr)
	assert.Equal(t, filepath.Join(unitPath, config.DefaultTerragruntConfigPath), experimentErr.ConfigPath)
}

// copyValuesBlockFixture copies the values-block fixture and returns the path of the copy.
func copyValuesBlockFixture(t *testing.T) string {
	t.Helper()

	return filepath.Join(helpers.CopyEnvironment(t, testFixtureValuesBlock), testFixtureValuesBlock)
}

// requireStackGenerateDiagnostic generates the stack at stackPath, requires that it fails with one diagnostic and
// writes no values file for its app unit, and returns the diagnostic.
func requireStackGenerateDiagnostic(t *testing.T, stackPath string) *hcl.Diagnostic {
	t.Helper()

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --experiment values-block --working-dir "+stackPath,
	)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
	require.Len(t, diags, 1)
	require.NotNil(t, diags[0].Subject)
	assert.NoFileExists(t, filepath.Join(stackPath, ".terragrunt-stack", "app", valuesBlockValuesFile))

	return diags[0]
}

// renderValuesBlockInputs renders the unit at unitPath as JSON and returns its inputs.
func renderValuesBlockInputs(t *testing.T, unitPath string) map[string]any {
	t.Helper()

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt render --json --experiment values-block --working-dir "+unitPath,
	)
	require.NoError(t, err)

	var rendered struct {
		Inputs map[string]any `json:"inputs"`
	}

	require.NoError(t, json.Unmarshal([]byte(stdout), &rendered))

	return rendered.Inputs
}
