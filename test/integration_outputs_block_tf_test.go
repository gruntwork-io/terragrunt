//go:build tf

package test_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const (
	testFixtureOutputsBlock          = "fixtures/outputs-block"
	testFixtureOutputsBlockInvalid   = "fixtures/outputs-block-invalid"
	testFixtureOutputsBlockVariables = "fixtures/outputs-block-variables"
	testFixtureOutputsBlockMerge     = "fixtures/outputs-block-merge"
	testFixtureOutputsBlockDefaults  = "fixtures/outputs-block-defaults"
	testFixtureOutputsBlockNullable  = "fixtures/outputs-block-nullable"
	testFixtureOutputsBlockNull      = "fixtures/outputs-block-null"
)

func TestTFOutputsBlockDeclaredMocksStandInBeforeApply(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		unit       string
		wantVPCID  string
		wantSubnet string
	}{
		{
			name:       "the dependent reads the declared mock values",
			unit:       "app",
			wantVPCID:  "vpc-mock",
			wantSubnet: "subnet-mock-1",
		},
		{
			name:       "the dependent's mock_outputs win key by key",
			unit:       "app-override",
			wantVPCID:  "vpc-override",
			wantSubnet: "subnet-mock-1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rootPath := filepath.Join(helpers.CopyEnvironment(t, testFixtureOutputsBlock), testFixtureOutputsBlock)
			unitPath := filepath.Join(rootPath, tc.unit)

			stdout, _, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt plan --experiment outputs-block --non-interactive --working-dir "+unitPath,
			)
			require.NoError(t, err)
			assert.Contains(t, stdout, tc.wantVPCID)
			assert.Contains(t, stdout, tc.wantSubnet)
		})
	}
}

// TestTFOutputsBlockDeclaredMocksReachVariablesBlock expects declared mocks in a variables block before apply.
func TestTFOutputsBlockDeclaredMocksReachVariablesBlock(t *testing.T) {
	t.Parallel()

	rootPath := filepath.Join(
		helpers.CopyEnvironment(t, testFixtureOutputsBlockVariables),
		testFixtureOutputsBlockVariables,
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --experiment outputs-block --experiment variables-block --non-interactive --working-dir "+
			filepath.Join(rootPath, "app"),
	)
	require.NoError(t, err)
	assert.Contains(t, stdout, "vpc-mock")
	assert.Contains(t, stdout, "subnet-mock-1")
}

func TestTFOutputsBlockRequiresExperiment(t *testing.T) {
	t.Parallel()

	helpers.SkipInExperimentMode(t, experiment.OutputsBlock)

	rootPath := filepath.Join(helpers.CopyEnvironment(t, testFixtureOutputsBlock), testFixtureOutputsBlock)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --working-dir "+filepath.Join(rootPath, "vpc"),
	)
	require.ErrorContains(t, err, "--experiment outputs-block")

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --working-dir "+filepath.Join(rootPath, "app"),
	)
	require.ErrorContains(t, err, "--experiment outputs-block")
}

func TestTFOutputsBlockStateOutputsReachDependents(t *testing.T) {
	t.Parallel()

	rootPath := filepath.Join(helpers.CopyEnvironment(t, testFixtureOutputsBlock), testFixtureOutputsBlock)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all apply --experiment outputs-block --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	for _, unit := range []string{"app", "app-override"} {
		stdout, _, err := helpers.RunTerragruntCommandWithOutput(
			t,
			"terragrunt output -json --experiment outputs-block --non-interactive --working-dir "+filepath.Join(rootPath, unit),
		)
		require.NoError(t, err)

		var outputs map[string]struct {
			Value any `json:"value"`
		}

		require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))
		assert.Equal(t, "vpc-real", outputs["vpc_id"].Value, unit)
		assert.Equal(t, []any{"subnet-real-1", "subnet-real-2"}, outputs["subnet_ids"].Value, unit)
	}
}

func TestTFOutputsBlockHCLValidateChecksContracts(t *testing.T) {
	t.Parallel()

	validPath := filepath.Join(helpers.CopyEnvironment(t, testFixtureOutputsBlock), testFixtureOutputsBlock)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt hcl validate --outputs --experiment outputs-block --working-dir "+validPath,
	)
	require.NoError(t, err)

	invalidPath := filepath.Join(
		helpers.CopyEnvironment(t, testFixtureOutputsBlockInvalid),
		testFixtureOutputsBlockInvalid,
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt hcl validate --outputs --experiment outputs-block --working-dir "+invalidPath,
	)
	require.Error(t, err)
	assert.Contains(t, stdout, "Declared output not in module")
	assert.Contains(t, stdout, `"nat_id"`)
	assert.Contains(t, stdout, "Undeclared dependency output")
	assert.Contains(t, stdout, `"nope"`)
}

func TestTFOutputsBlockMergesMocksWithState(t *testing.T) {
	t.Parallel()

	rootPath := filepath.Join(helpers.CopyEnvironment(t, testFixtureOutputsBlockMerge), testFixtureOutputsBlockMerge)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --experiment outputs-block --non-interactive --working-dir "+
			filepath.Join(rootPath, "vpc"),
	)
	require.NoError(t, err)

	for _, name := range []string{"main.tf", "terragrunt.hcl"} {
		helpers.CopyFile(t, filepath.Join(rootPath, "vpc-with-nat", name), filepath.Join(rootPath, "vpc", name))
	}

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --experiment outputs-block --non-interactive --working-dir "+filepath.Join(rootPath, "app"),
	)
	require.NoError(t, err)
	assert.Contains(t, stdout, "vpc-real")
	assert.Contains(t, stdout, "nat-mock")

	_, _, err = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt apply -auto-approve --experiment outputs-block --non-interactive --working-dir "+
			filepath.Join(rootPath, "app"),
	)
	require.ErrorContains(t, err, `has no output "nat_id"`)
}

func TestTFOutputsBlockDeclaredMocksReachEveryCommandWithoutAllowedCommands(t *testing.T) {
	t.Parallel()

	rootPath := filepath.Join(helpers.CopyEnvironment(t, testFixtureOutputsBlockDefaults), testFixtureOutputsBlockDefaults)
	appPath := filepath.Join(rootPath, "app")

	for _, command := range []string{"plan", "apply -auto-approve"} {
		stdout, _, err := helpers.RunTerragruntCommandWithOutput(
			t,
			"terragrunt "+command+" --experiment outputs-block --non-interactive --working-dir "+appPath,
		)
		require.NoError(t, err)
		require.Contains(t, stdout, "vpc-mock")
	}
}

func TestTFOutputsBlockNullableOutputReadsNull(t *testing.T) {
	t.Parallel()

	rootPath := filepath.Join(helpers.CopyEnvironment(t, testFixtureOutputsBlockNullable), testFixtureOutputsBlockNullable)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all apply --experiment outputs-block --non-interactive --working-dir "+rootPath,
	)
	require.NoError(t, err)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -json --experiment outputs-block --non-interactive --working-dir "+filepath.Join(rootPath, "app"),
	)
	require.NoError(t, err)

	var outputs map[string]struct {
		Value any `json:"value"`
	}

	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))
	require.Equal(t, "vpc-real", outputs["vpc_id"].Value)
	require.Equal(t, true, outputs["nat_is_null"].Value)
}

func TestTFOutputsBlockApplyFailsOnANullOutputThatIsNotNullable(t *testing.T) {
	t.Parallel()

	rootPath := filepath.Join(helpers.CopyEnvironment(t, testFixtureOutputsBlockNull), testFixtureOutputsBlockNull)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all apply --experiment outputs-block --non-interactive --working-dir "+rootPath,
	)

	var missing run.RequiredOutputsMissingError

	require.ErrorAs(t, err, &missing)
	require.Equal(t, []string{"nat_id"}, missing.Names)
	require.Len(t, appliedStates(t, rootPath, "vpc"), 1)
	require.Empty(t, appliedStates(t, rootPath, "app"))
}

// appliedStates returns the state files that the unit in rootPath wrote.
func appliedStates(t *testing.T, rootPath, unit string) []string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(rootPath, unit, ".terragrunt-cache", "*", "*", "terraform.tfstate"))
	require.NoError(t, err)

	return matches
}
