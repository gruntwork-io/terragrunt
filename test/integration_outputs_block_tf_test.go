//go:build tf

package test_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const (
	testFixtureOutputsBlock          = "fixtures/outputs-block"
	testFixtureOutputsBlockInvalid   = "fixtures/outputs-block-invalid"
	testFixtureOutputsBlockVariables = "fixtures/outputs-block-variables"
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

// TestTFOutputsBlockDeclaredMocksReachVariablesBlock pins that a dependent reading the outputs through a
// variables block gets the declared mock values before the unit is applied.
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
		"terragrunt hcl validate --experiment outputs-block --working-dir "+validPath,
	)
	require.NoError(t, err)

	invalidPath := filepath.Join(
		helpers.CopyEnvironment(t, testFixtureOutputsBlockInvalid),
		testFixtureOutputsBlockInvalid,
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt hcl validate --experiment outputs-block --working-dir "+invalidPath,
	)
	require.Error(t, err)
	assert.Contains(t, stdout, "Declared output not in module")
	assert.Contains(t, stdout, `"nat_id"`)
	assert.Contains(t, stdout, "Undeclared dependency output")
	assert.Contains(t, stdout, `"nope"`)
}
